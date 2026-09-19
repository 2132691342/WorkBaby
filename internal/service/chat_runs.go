package service

import (
	"context"
	"strings"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// 本文件：run 生命周期操作（取消 / 插话 / 后台 run / 续跑 / 排空 / 占位落库 / seq 分配）。

func (s *ChatService) CancelStream(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	s.runs.cancel(sessionID)
	s.steers.clear(sessionID) // run 已终止，排队中的注入消息一并作废
	return nil
}

const defaultAgentName = "default"

// runIDs 一次 run 的身份（runID + user/assistant 消息 ID）。
type runIDs struct {
	RunID          string
	UserMsgID      string
	AssistantMsgID string
}

// prepareRun 落 user + assistant 占位消息并登记执行平面 run。
//
// chat / 后台任务共用：同一会话串行落 seq，run 身份在消息落库前就已确定
// （runID 需先写入消息行，供前端按 run 过滤事件）。调用方负责持会话锁。
func (s *ChatService) prepareRun(ctx context.Context, ses *domain.ChatSessionDO, content string, atts []domain.MessageAttachment, scope core.Scope, agentName string) (runIDs, error) {
	// 新 run 启动前清掉同会话遗留挂起审批（跨重启残留）：旧卡的决策对象已不存在
	if s.approval != nil {
		s.approval.CancelSession(ctx, ses.ID)
	}
	seqStart, err := s.allocSeq(ctx, ses.ID, 2)
	if err != nil {
		return runIDs{}, err
	}

	runID := pkg.NewID("RUN")
	if s.execs != nil {
		s.execs.Register(&core.ExecutionRun{
			RunID:     runID,
			SessionID: ses.ID,
			Scope:     scope,
			AgentName: agentName,
			State:     core.StateRunning,
		})
	}
	now := time.Now().UnixMilli()
	userMsg := &domain.MessageDO{
		ID:        pkg.NewID(domain.IDMessage),
		SessionID: ses.ID,
		Seq:       seqStart,
		RunID:     runID,
		Role:      domain.MessageRoleUser,
		Content:   content,
		Status:    domain.MessageStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}
	userMsg.SetAttachments(atts)
	if err := s.messages.Insert(ctx, userMsg); err != nil {
		return runIDs{}, err
	}
	assistantMsg := &domain.MessageDO{
		ID:        pkg.NewID(domain.IDMessage),
		SessionID: ses.ID,
		Seq:       seqStart + 1,
		RunID:     runID,
		Role:      domain.MessageRoleAssistant,
		Status:    domain.MessageStatusStreaming,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.messages.Insert(ctx, assistantMsg); err != nil {
		return runIDs{}, err
	}
	ses.MessageCount += 2
	ses.LastMessageAt = now
	_ = s.sessions.Update(ctx, ses)
	return runIDs{RunID: runID, UserMsgID: userMsg.ID, AssistantMsgID: assistantMsg.ID}, nil
}

// resolveAttachments 受管文件 ID → 消息附件；图片标为 image，多模态链路据此展开。
func (s *ChatService) resolveAttachments(ctx context.Context, ids []string) []domain.MessageAttachment {
	if s.files == nil || len(ids) == 0 {
		return nil
	}
	out := make([]domain.MessageAttachment, 0, len(ids))
	for _, id := range ids {
		f, err := s.files.Get(ctx, id)
		if err != nil {
			pkg.L.Warn("resolve attachment failed", "fileID", id, "err", err.Error())
			continue
		}
		name := f.OriginalName
		if name == "" {
			name = f.Name
		}
		kind := domain.AttachmentFile
		if strings.HasPrefix(f.MimeType, "image/") {
			kind = domain.AttachmentImage
		}
		out = append(out, domain.MessageAttachment{
			ID:   f.ID,
			Name: name,
			MIME: f.MimeType,
			Size: f.Size,
			Kind: kind,
			URL:  "/files/files/" + f.ID,
		})
	}
	return out
}

// nextSeq 会话下一条消息序号（数据库当前最大值 + 1）。
func (s *ChatService) nextSeq(ctx context.Context, sessionID string) (int64, error) {
	maxSeq, err := s.messages.MaxSeq(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return maxSeq + 1, nil
}

// allocSeq 分配会话内连续 n 个消息序号（进程内单调水位，首次取数据库最大值 + 1）。
//
// 运行中的工具结果消息与 steering 注入消息都会落库，必须走同一分配器：
// 序号撞号会让增量分页（after_seq 游标）漏消息。
func (s *ChatService) allocSeq(ctx context.Context, sessionID string, n int64) (int64, error) {
	s.seqMu.Lock()
	defer s.seqMu.Unlock()
	start, ok := s.seqs[sessionID]
	if !ok {
		dbSeq, err := s.nextSeq(ctx, sessionID)
		if err != nil {
			return 0, err
		}
		start = dbSeq
	}
	s.seqs[sessionID] = start + n
	return start, nil
}

// QueueSteer run 进行中插入一条用户指令：消息立即落库（前端可见），内容进注入队列由 harness
// 在 steering（轮间）或 follow-up（收尾后）缝消费。无活动 run 时返回 ErrRunNotActive。

func (s *ChatService) QueueSteer(ctx context.Context, sessionID, content string) (*domain.SteerResultRESP, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, domain.ErrMessageInvalid
	}
	runID, ok := s.runs.lookup(sessionID)
	if !ok {
		return nil, domain.ErrRunNotActive
	}
	unlock := s.lockSession(sessionID)
	defer unlock()

	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	seq, err := s.allocSeq(ctx, sessionID, 1)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	msg := &domain.MessageDO{
		ID:        pkg.NewID(domain.IDMessage),
		SessionID: sessionID,
		Seq:       seq,
		RunID:     runID,
		Role:      domain.MessageRoleUser,
		Content:   content,
		Status:    domain.MessageStatusCompleted,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.messages.Insert(ctx, msg); err != nil {
		return nil, err
	}
	ses.MessageCount++
	ses.LastMessageAt = now
	_ = s.sessions.Update(ctx, ses)

	s.steers.push(sessionID, llm.UserMessage(content))
	s.emit(runID, sessionID, "chat:steer", map[string]any{
		"message_id": msg.ID,
		"seq":        seq,
		"role":       string(domain.MessageRoleUser),
		"content":    content,
	})
	return &domain.SteerResultRESP{RunID: runID, SessionID: sessionID, MessageID: msg.ID, Queued: true}, nil
}

// RunAgent 同步跑一轮 Agent（后台任务入口； 后台任务队列）。
//
// 与 SendStream 的区别：不占用会话的活动 run 槽（后台任务与前台聊天可并行），
// 阻塞直到 run 结束并返回结果摘要。
func (s *ChatService) RunAgent(ctx context.Context, sessionID, userInput, agentName string) (*AgentRunOutcome, error) {
	unlock := s.lockSession(sessionID)
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		unlock()
		return nil, err
	}
	if ses.ProviderID == "" || ses.Model == "" {
		if pid, model, ok := s.resolveDefaultProviderModel(ctx, ses.Model); ok {
			ses.ProviderID = pid
			ses.Model = model
			_ = s.sessions.Update(ctx, ses)
		} else {
			unlock()
			return nil, pkg.Wrap(domain.ErrSessionInvalid.Code, domain.ErrSessionInvalid.Message, domain.ErrSessionInvalid)
		}
	}
	if agentName == "" {
		agentName = defaultAgentName
	}
	ids, err := s.prepareRun(ctx, ses, userInput, nil, core.ScopeTask, agentName)
	unlock()
	if err != nil {
		return nil, err
	}
	def := core.Agent(agentName)
	res := s.executeAgent(ctx, ses, ids.RunID, ids.AssistantMsgID, userInput, core.RequestParams{}, def, false)
	return &AgentRunOutcome{
		RunID:          ids.RunID,
		SessionID:      sessionID,
		AssistantMsgID: ids.AssistantMsgID,
		Content:        res.Content,
		Reason:         string(res.Reason),
		Err:            res.Err,
	}, nil
}

// AgentRunOutcome 一次 Agent 运行的成品（后台任务回执）。
type AgentRunOutcome struct {
	RunID          string
	SessionID      string
	AssistantMsgID string
	Content        string
	Reason         string
	Err            error
}

// runLLM 异步跑 LLM（多轮 ReAct）：墙钟预算 + 结果落库 + 发 chat:done / chat:error。
// resume=true 时从检查点续跑同一 runID（中断/崩溃后恢复）。

func (s *ChatService) ResumeRun(ctx context.Context, runID string) (*domain.SendStreamResult, error) {
	if s.checkpoints == nil {
		return nil, pkg.New(5007, "检查点未启用", "")
	}
	cp, err := s.checkpoints.LoadLast(runID)
	if err != nil {
		return nil, err
	}
	ses, err := s.sessions.GetByID(ctx, cp.SessionID)
	if err != nil {
		return nil, err
	}
	if cp.AssistantMsgID == "" {
		return nil, pkg.New(5007, "检查点无关联消息，无法续跑", runID)
	}
	if _, busy := s.runs.lookup(ses.ID); busy {
		return nil, pkg.New(5002, "会话忙（已有运行中的 run）", ses.ID)
	}
	// 中断终态回置 streaming，前端继续渲染同一条消息
	_ = s.messages.UpdateStatus(ctx, cp.AssistantMsgID, map[string]any{
		"status": domain.MessageStatusStreaming, "stop_reason": nil, "updated_at": time.Now().UnixMilli(),
	})
	runCtx, cancel := context.WithCancel(context.Background())
	s.runs.set(ses.ID, runID, cancel)
	go func() {
		defer s.runs.delete(ses.ID)
		s.runLLM(runCtx, ses, runID, cp.AssistantMsgID, "", core.RequestParams{}, core.Agent(defaultAgentName), true)
	}()
	return &domain.SendStreamResult{RunID: runID, SessionID: ses.ID, AssistantMsgID: cp.AssistantMsgID}, nil
}

// ReapInterrupted 启动排空：崩溃时卡在 streaming 的 assistant 消息标 failed + interrupted。
func (s *ChatService) ReapInterrupted(ctx context.Context) {
	n, err := s.messages.ReapStreaming(ctx, domain.MessageStatusFailed, "interrupted")
	if err != nil {
		pkg.L.Warn("reap interrupted messages failed", "err", err.Error())
		return
	}
	if n > 0 {
		pkg.L.Info("reaped interrupted streaming messages", "count", n)
	}
}

// executeAgent 跑一次 Agent：装配上下文 → 跑 harness → 落库/计量/记忆。
// chat 与后台任务共用同一条路径（唯一差异是调用方给的 ctx 与 Agent 定义）。
