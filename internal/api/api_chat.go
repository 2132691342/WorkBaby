package api

import (
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/resource"
	"WorkBaby/internal/service"
)

// ListMessages 增量分页（afterSeq=0 取该会话全部）。
func (h *Handler) ListMessages(sessionID string, afterSeq int64, limit int) (domain.MessageListRESP, error) {
	r, err := h.chatSvc.ListMessages(h.ctx, sessionID, afterSeq, limit)
	if err != nil {
		return domain.MessageListRESP{}, err
	}
	return *r, nil
}

// SendStream 立即返回 runID/消息 ID；流式事件经 harness → event.Bus → api 层 → chat:* 推前端。
func (h *Handler) SendStream(req domain.SendStreamREQ) (domain.SendStreamResult, error) {
	if req.SessionID == "" {
		return domain.SendStreamResult{}, domain.ErrSessionInvalid
	}
	params := agent.RequestParams{Temperature: req.Temperature, Thinking: llm.ThinkingFromEffort(req.ThinkingEffort)}
	r, err := h.chatSvc.SendStream(h.ctx, req.SessionID, req.Content, req.FileIDs, params)
	if err != nil {
		return domain.SendStreamResult{}, err
	}
	return *r, nil
}

// ResumeChat 从检查点续跑：同一 runID 恢复，前端以返回的 run_id 重挂 SSE。
func (h *Handler) ResumeChat(runID string) (domain.SendStreamResult, error) {
	if runID == "" {
		return domain.SendStreamResult{}, domain.ErrSessionInvalid
	}
	r, err := h.chatSvc.ResumeRun(h.ctx, runID)
	if err != nil {
		return domain.SendStreamResult{}, err
	}
	return *r, nil
}

// ExportRunEvents run 事件 JSONL 导出（回放/测试/无头消费）。
func (h *Handler) ExportRunEvents(runID string) (string, error) {
	bs, err := h.eventLog.Export(runID)
	if err != nil {
		return "", pkg.New(5007, "run 事件导出不可用", runID)
	}
	return string(bs), nil
}

// ListRuns 运行历史索引（倒序分页）；单 run 的事件明细回放走 /chat/runs/:id/events。
func (h *Handler) ListRuns(req domain.RunRecordListREQ) (domain.RunRecordListRESP, error) {
	return h.chatSvc.ListRunRecords(h.ctx, req.SessionID, req.Limit, req.Offset)
}

// SteerSession run 进行中插入一条用户指令（steering / follow-up 注入缝）。
// 会话当前没有活动 run 时返回 5010，前端退回普通发送。
func (h *Handler) SteerSession(sessionID string, req domain.SteerREQ) (domain.SteerResultRESP, error) {
	r, err := h.chatSvc.QueueSteer(h.ctx, sessionID, req.Content)
	if err != nil {
		return domain.SteerResultRESP{}, err
	}
	return *r, nil
}

// CancelStream 中断会话正在跑的 run（前端「停止」按钮调用）。幂等。
func (h *Handler) CancelStream(sessionID string) error {
	return h.chatSvc.CancelStream(sessionID)
}

// ListChatCommands 斜杠命令元数据：内置 + 自定义命令合并（自定义命令 client_only 且带 prompt 模板）。
// sessionID 非空时一并加载该会话工作区下的命令文件；同名按就近优先：工作区 > 用户级 > 设置页。
func (h *Handler) ListChatCommands(sessionID string) (domain.CommandListRESP, error) {
	items := service.BuiltinCommands()
	byName := make(map[string]int, len(items))
	for i, it := range items {
		byName[it.Name] = i
	}

	// upsertCustom 同名覆盖已存在的条目（含内置名），否则追加。
	upsertCustom := func(c domain.SlashCommand) {
		if c.Desc == "" {
			c.Desc = firstLine(c.Prompt)
		}
		if i, dup := byName[c.Name]; dup {
			items[i] = c
			return
		}
		byName[c.Name] = len(items)
		items = append(items, c)
	}

	// ① 设置页记录（最先铺底，会被下面的文件覆盖）
	if customs, err := h.commandSvc.List(h.ctx); err == nil {
		for _, c := range customs {
			upsertCustom(domain.SlashCommand{
				Name:       c.Name,
				Desc:       c.Description,
				Group:      "custom",
				ClientOnly: true,
				Prompt:     c.Prompt,
				Source:     domain.CommandSourceUser,
			})
		}
	}
	// ② + ③ 用户级 / 工作区级文件：工作区同名优先于用户级（与工作区技能同约定）。
	// 一次性合并加载，避免两次循环 upsert。
	var wsPath string
	if sessionID != "" {
		wsPath = h.chatSvc.WorkspaceRoot(h.ctx, sessionID, "")
	}
	for _, c := range resource.LoadCommands(h.paths.Home, wsPath) {
		upsertCustom(c)
	}
	return domain.CommandListRESP{Items: items, Total: len(items)}, nil
}

// firstLine 取模板首行作描述兜底（rune 截断 80）。
func firstLine(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	r := []rune(s)
	if len(r) > 80 {
		s = string(r[:80]) + "…"
	}
	return s
}

// UpsertCustomCommand 创建/更新自定义斜杠命令（按 name upsert）。
func (h *Handler) UpsertCustomCommand(req domain.UserCommandREQ) (domain.UserCommandRESP, error) {
	p, err := h.commandSvc.Upsert(h.ctx, &req)
	if err != nil {
		return domain.UserCommandRESP{}, err
	}
	return *p, nil
}

// DeleteCustomCommand 删除自定义斜杠命令。
func (h *Handler) DeleteCustomCommand(name string) error {
	return h.commandSvc.Delete(h.ctx, name)
}

// ListCustomCommands 自定义命令全量（设置页管理用；/ 面板走 ListChatCommands 合并）。
func (h *Handler) ListCustomCommands() ([]domain.UserCommandRESP, error) {
	return h.commandSvc.List(h.ctx)
}

// SetSessionAgent 切换会话 Agent（写元数据；下次 run 起生效）。Agent 合法值：
// "" / "default" / "coding" / "research" / "writer"；空字符串 = 清除覆盖（回到内置 default）。
func (h *Handler) SetSessionAgent(sessionID string, req domain.SetSessionAgentREQ) error {
	return h.chatSvc.SetSessionAgent(h.ctx, sessionID, req.Agent)
}

// CompactSession 压缩会话历史上下文（/compact 的后端实现）。Instructions 是「保留指示」：
// 非空会钉进会话元数据并以 system 段常驻注入（不受历史折叠影响）；KeepRecent<=0 用后端默认。
func (h *Handler) CompactSession(sessionID string, req domain.CompactREQ) (domain.CompactResultRESP, error) {
	return h.chatSvc.CompactSession(h.ctx, sessionID, req)
}

// GetSessionGoal 查询会话目标（目标模式状态卡数据源）。
func (h *Handler) GetSessionGoal(sessionID string) (domain.GoalRESP, error) {
	return h.chatSvc.Goal(h.ctx, sessionID)
}

// SetSessionGoal 设置/替换/暂停/恢复/清除会话目标（/goal 命令后端实现）。
func (h *Handler) SetSessionGoal(sessionID string, req domain.GoalREQ) (domain.GoalRESP, error) {
	return h.chatSvc.SetGoal(h.ctx, sessionID, req)
}

// GetSideConversation 返回主会话已有的辅助会话；没有返回 null（面板显示空态）。
func (h *Handler) GetSideConversation(sessionID string) (*domain.ChatSessionRESP, error) {
	return h.chatSvc.GetSideConversation(h.ctx, sessionID)
}

// EnsureSideConversation 返回主会话的辅助会话（不存在则创建；模型/工作区/权限随主会话）。
func (h *Handler) EnsureSideConversation(sessionID string) (*domain.ChatSessionRESP, error) {
	return h.chatSvc.EnsureSideConversation(h.ctx, sessionID)
}

// SearchSessions 跨会话快速检索（/resume）。
//
// req.Scope：all / title / content（空 = all）；req.Limit 上限 50。
func (h *Handler) SearchSessions(req domain.SessionSearchREQ) (*domain.SessionListRESP, error) {
	return h.chatSvc.SearchSessions(h.ctx, req)
}

// ContextUsage 会话上下文占用分段快照（/context 侧栏环）。
func (h *Handler) ContextUsage(sessionID string) (domain.ContextUsageRESP, error) {
	return h.chatSvc.ContextUsage(h.ctx, sessionID)
}

// GetSessionTodos 返回会话计划快照（前端进度卡首次渲染 / 刷新后恢复用）。
// 运行中的增量由 chat:todo 事件推送，与此处同源（同一 SessionTodoStore）。
func (h *Handler) GetSessionTodos(sessionID string) domain.TodoStateRESP {
	if h.todoStore == nil {
		return domain.TodoStateRESP{SessionID: sessionID}
	}
	state, err := h.todoStore.State(h.ctx, sessionID)
	if err != nil {
		// 读取失败返回空快照而非抛错：进度卡属增强信息，不该让会话页整体报错
		pkg.L.Warn("load session todos failed", "sessionID", sessionID, "err", err.Error())
		return domain.TodoStateRESP{SessionID: sessionID}
	}
	return state
}

// ToggleSessionTodo 用户手动勾选/取消计划项：与模型写的同一份状态，下一轮注入即生效。
func (h *Handler) ToggleSessionTodo(sessionID, itemID string) (domain.TodoStateRESP, error) {
	if h.todoStore == nil {
		return domain.TodoStateRESP{SessionID: sessionID}, nil
	}
	return h.todoStore.Toggle(h.ctx, sessionID, itemID)
}

// ClearMessages 清空某会话全部消息。
func (h *Handler) ClearMessages(sessionID string) error {
	return h.chatSvc.ClearMessages(h.ctx, sessionID)
}

// DeleteMessage 删除单条消息。
func (h *Handler) DeleteMessage(sessionID string, messageID string) error {
	return h.chatSvc.DeleteMessage(h.ctx, sessionID, messageID)
}

// TruncateMessages 从指定消息起（含）截断，用于「从此处重新生成」。
func (h *Handler) TruncateMessages(sessionID string, req domain.TruncateMessagesREQ) error {
	return h.chatSvc.TruncateMessages(h.ctx, sessionID, req.MessageID)
}

// ForkSession 从指定消息处分叉为新会话。
func (h *Handler) ForkSession(sessionID string, req domain.ForkSessionREQ) (domain.ChatSessionRESP, error) {
	r, err := h.chatSvc.ForkSession(h.ctx, sessionID, req.MessageID, req.Name)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *r, nil
}
