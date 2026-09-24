package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/registry"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
	"WorkBaby/internal/tool/planmode"
)

// FileStore 附件读取能力（ChatService 只用于把图片附件转成 LLM 多模态 part）。
type FileStore interface {
	Get(ctx context.Context, id string) (domain.FileRESP, error)
	ReadDataURL(ctx context.Context, id string) (string, error)
}

// ChatDeps ChatService 的依赖集合：构造期一次性注入（不提供后置 setter，
// 漏接会让检查点/重放/落块等能力静默失效；由 MissingDeps 自检兜底）。
type ChatDeps struct {
	// 基础
	Sessions  *repo.ChatSessionRepo
	Messages  *repo.MessageRepo
	Providers *repo.AiProviderRepo
	Settings  *repo.SystemSettingRepo
	Usages    *repo.TokenUsageRepo
	Bus       *event.Bus
	Registry  *registry.Registry
	Tools     *ToolService
	Memory    *memory.Service

	// 路径解析（与工具沙箱 / 文件变更快照共用同一实例）
	Session *SessionContext

	// 持久化与运行时设施
	Checkpoints core.CheckpointStore
	EventLog    *event.RunEventLog
	// Emitter 事件出口；nil 时用 Bus + EventLog 现场构造。装配方传入共享实例，
	// 可保证 chat / task / approval 等域的 seq 落在同一条序列上。
	Emitter    *Emitter
	Blocks     *repo.MessageBlockRepo
	RunRecords *repo.RunRecordRepo
	Executions *core.ExecutionRegistry

	// 协作服务
	Approvals    *ApprovalService
	Trust        *TrustService
	Changes      *FileChangeService
	PlanStore    *planmode.Store
	Hooks        *UserHookService
	Files        FileStore
	Capabilities *capability.Registry

	// SkillSync run 前按会话工作区叠加技能；nil = 不启用
	SkillSync func(context.Context, string) error
}

// ChatService 会话 + 消息编排；接入 core.Loop 调用真实 LLM。
type ChatService struct {
	sessions    *repo.ChatSessionRepo
	messages    *repo.MessageRepo
	provRepo    *repo.AiProviderRepo
	setRepo     *repo.SystemSettingRepo
	usages      *repo.TokenUsageRepo // token 明细（仪表盘三线图的唯一数据源）
	bus         *event.Bus
	emitter     *Emitter // 事件统一出口（注入归属 + seq + 广播）
	reg         *registry.Registry
	tools       *ToolService
	mem         *memory.Service                     // 上下文占用统计（/context 分段）读取长期记忆
	sctx        *SessionContext                     // 工作区根与过程数据目录解析（唯一数据源）
	trust       *TrustService                       // 目录信任；nil = 关闭
	planStore   *planmode.Store                     // 计划模式状态；nil = 不启用
	locksMu     sync.Mutex                          // 保护 locks
	locks       map[string]*sync.Mutex              // 会话级互斥：同会话的建流 / 插话 / 后台 run 串行，跨会话并行
	runs        *runRegistry                        // 活动 run 注册中心（按 sessionID → cancel）；用于前端「停止」按钮
	checkpoints core.CheckpointStore                // 检查点存储（SQL 默认；nil = 关闭）
	runRec      *repo.RunRecordRepo                 // 运行历史索引；nil = 不记录
	execs       *core.ExecutionRegistry             // 执行平面
	blocks      *repo.MessageBlockRepo              // 消息块持久化；nil = 不落块
	files       FileStore                           // 受管文件读取（消息附件 → 多模态 part）；nil = 附件降级为文本
	changeSvc   *FileChangeService                  // 本 run 文件变更查询（完成度证据核对）
	approval    *ApprovalService                    // 工具策略门 ask 决策的人工审批；nil = 策略门不启用
	steers      *steerQueue                         // run 中用户新消息的注入队列（steering / follow-up）
	seqMu       sync.Mutex                          // 保护 seqs
	seqs        map[string]int64                    // 会话消息序号分配水位（工具消息与注入消息统一分配，防撞号）
	caps        *capability.Registry                // 能力注册表：上下文装配 / 工具暴露 / run 后沉淀三条通道
	skillSync   func(context.Context, string) error // 技能目录同步钩子（run 前按会话工作区叠加）；nil = 不启用
	hookRunner  *UserHookService                    // 用户钩子执行器（run_start/before_tool/after_tool/run_end）；nil = 不启用
}

// memoryCaptureTimeout run 后沉淀（记忆形成等）的独立超时。
const memoryCaptureTimeout = 30 * time.Second

// lockSession 取会话级互斥锁；返回解锁函数。
// 关键区只覆盖「占位消息落库 + 活动 run 登记」：同一会话的并发发送 / 插话在此串行，
// 不同会话互不阻塞（全局单锁会让多会话同时对话排队）。
func (s *ChatService) lockSession(sessionID string) func() {
	s.locksMu.Lock()
	if s.locks == nil {
		s.locks = make(map[string]*sync.Mutex)
	}
	l, ok := s.locks[sessionID]
	if !ok {
		l = &sync.Mutex{}
		s.locks[sessionID] = l
	}
	s.locksMu.Unlock()
	l.Lock()
	return l.Unlock
}

// NewChatService 构造会话编排服务；依赖一次性注入（见 ChatDeps）。
func NewChatService(deps ChatDeps) *ChatService {
	emitter := deps.Emitter
	if emitter == nil {
		emitter = NewEmitter(deps.Bus, deps.EventLog)
	}
	return &ChatService{
		sessions: deps.Sessions, messages: deps.Messages, provRepo: deps.Providers,
		setRepo: deps.Settings, usages: deps.Usages, bus: deps.Bus, reg: deps.Registry,
		emitter: emitter,
		tools:   deps.Tools, mem: deps.Memory, sctx: deps.Session,
		caps: deps.Capabilities, checkpoints: deps.Checkpoints,
		blocks: deps.Blocks, runRec: deps.RunRecords, execs: deps.Executions,
		approval: deps.Approvals, trust: deps.Trust, changeSvc: deps.Changes,
		planStore: deps.PlanStore, hookRunner: deps.Hooks, files: deps.Files,
		skillSync: deps.SkillSync,
		runs:      newRunRegistry(),
		steers:    newSteerQueue(), seqs: map[string]int64{},
	}
}

// MissingDeps 未注入的核心依赖清单（启动自检）；空 = 装配完整。
// 核心链路依赖缺失时的表现是「功能静默不生效」而非报错，装配方必须显式校验。
func (s *ChatService) MissingDeps() []string {
	var missing []string
	add := func(cond bool, name string) {
		if cond {
			missing = append(missing, name)
		}
	}
	add(s.sessions == nil, "Sessions")
	add(s.messages == nil, "Messages")
	add(s.reg == nil, "Registry")
	add(s.tools == nil, "Tools")
	add(s.caps == nil, "Capabilities")
	add(s.checkpoints == nil, "Checkpoints")
	add(s.emitter == nil || s.emitter.log == nil, "EventLog")
	add(s.blocks == nil, "Blocks")
	add(s.runRec == nil, "RunRecords")
	add(s.execs == nil, "Executions")
	add(s.approval == nil, "Approvals")
	add(s.sctx == nil, "Session")
	return missing
}

// CancelStream 中断指定 session 正在跑的 run（前端「停止」按钮）。
//
// 幂等：无 run 或 sessionID 为空直接 nil；harness 收到 ctx 取消后 emit ReasonCancelled → chat:done。

func (s *ChatService) SendStream(ctx context.Context, sessionID, content string, fileIDs []string, params core.RequestParams) (*domain.SendStreamResult, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	// 兜底：历史会话可能缺 provider/model（首次启动升级 / 旧数据），运行时补一个默认 Provider
	if ses.ProviderID == "" || ses.Model == "" {
		if pid, model, ok := s.resolveDefaultProviderModel(ctx, ses.Model); ok {
			ses.ProviderID = pid
			ses.Model = model
			_ = s.sessions.Update(ctx, ses)
		} else {
			return nil, pkg.Wrap(domain.ErrSessionInvalid.Code, domain.ErrSessionInvalid.Message, domain.ErrSessionInvalid)
		}
	}

	// 首条用户消息自动命名（无历史消息可推导时），让会话在列表里可辨认。
	firstTurn := ses.MessageCount == 0
	atts := s.resolveAttachments(ctx, fileIDs)
	agentName := SessionAgent(ses)
	if agentName == "" {
		agentName = defaultAgentName
	}
	ids, err := s.prepareRun(ctx, ses, content, atts, core.ScopeChatTurn, agentName)
	if err != nil {
		return nil, err
	}
	if firstTurn {
		// 只有附件没有文字时（纯图片提问）用附件名兜底，避免会话标题为空
		titleSrc := content
		if strings.TrimSpace(titleSrc) == "" && len(atts) > 0 {
			titleSrc = atts[0].Name
		}
		s.autoTitleSession(ctx, ses, titleSrc)
	}

	// 异步跑；用可取消 ctx（前端「停止」→ CancelStream 触发）
	runCtx, cancel := context.WithCancel(context.Background())
	s.runs.set(ses.ID, ids.RunID, cancel)
	go func() {
		// deleteIf：目标模式自动续跑会在旧 run 尾部拉起新 run，无条件 delete 会误删新注册
		defer s.runs.deleteIf(ses.ID, ids.RunID)
		s.runLLM(runCtx, ses, ids.RunID, ids.AssistantMsgID, content, params, core.Agent(agentName), false)
	}()

	return &domain.SendStreamResult{
		RunID:          ids.RunID,
		SessionID:      sessionID,
		UserMsgID:      ids.UserMsgID,
		AssistantMsgID: ids.AssistantMsgID,
	}, nil
}

// defaultAgentName chat 主入口的默认 Agent。

func (s *ChatService) runLLM(parentCtx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, userInput string, params core.RequestParams, def core.Definition, resume bool) {
	s.startRunRecord(parentCtx, runID, ses)
	wall := 10 * time.Minute
	if def.Budget.MaxWallTime > 0 {
		wall = def.Budget.MaxWallTime
	}
	ctx, cancel := context.WithTimeout(parentCtx, wall)
	defer cancel()
	// run 结束（含取消/失败）后队列作废：注入消息只属于本次 run
	defer s.steers.clear(ses.ID)
	s.executeAgent(ctx, ses, runID, assistantMsgID, userInput, params, def, resume)
}

// ResumeRun 从检查点续跑：中断/崩溃后以同一 runID 恢复，前端重挂 SSE 续渲染。
// 已完成工具调用经 StepRecords 复用，不重放副作用；审批/补充输入会重新发起。

func (s *ChatService) executeAgent(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, userInput string, params core.RequestParams, def core.Definition, resume bool) *core.Outcome {
	// 运行前装配见 chat_prepare.go：技能叠加 → Agent 覆盖 → 用户钩子 → 历史 → system → 工具暴露。
	s.syncWorkspaceSkills(ctx, ses)
	runModel, params := s.applyAgentOverrides(ctx, ses, runID, def, params)

	promptHookCtx, err := s.promptHookContext(ctx, ses, runID, userInput, resume)
	if err != nil {
		return s.failOutcome(ctx, ses, runID, assistantMsgID, err)
	}
	prov, err := s.reg.Get(ses.ProviderID)
	if err != nil {
		return s.failOutcome(ctx, ses, runID, assistantMsgID, err)
	}
	llmMsgs, err := s.loadRunHistory(ctx, ses)
	if err != nil {
		return s.failOutcome(ctx, ses, runID, assistantMsgID, err)
	}

	// system 装配（真实请求口径，与 ContextUsage 透视同源——见 buildSystem）：
	// 能力注入段 + 服务侧附加段 + 用户钩子补充段，最终交给内核 Config.System。
	// 历史里不再前置 system 消息——两处都放会让上游收到两条 system，部分厂商直接 400。
	sys, runState := s.buildSystem(ctx, ses, runID, userInput, def)
	sysText := s.systemWithHookContext(ctx, ses, runID, sys, promptHookCtx, resume)

	// 终态事件延迟到 assistant 消息落库后再发：前端收 chat:done 会立刻拉权威快照，
	// 早于落库会让「空 content」覆盖已渲染内容，过程块一并丢失。
	// 事件 → chat:* / 消息块 / tool 消息的映射收敛在 coreEventMapper（双通道汇聚点）。
	runStart := time.Now()
	mapper := newCoreEventMapper(s, ctx, ses, runID, assistantMsgID, runState)
	defer func() {
		if mapper.doneEvent != nil {
			s.emit(runID, ses.ID, "chat:done", mapper.doneEvent)
		}
	}()

	toolNames := s.exposedToolNames(ctx, def, runState.SkillTools)

	// 上下文预算按「上下文窗口 × 压缩比例」重算：Agent 内置值是静态的，
	// 小窗口模型会撑爆、大窗口模型又过早压缩，交给 provider 与全局设置决定。
	window := s.modelContextWindow(ctx, ses.ProviderID, runModel)
	budget := s.contextBudget(ctx, window, s.providerDO(ctx, ses.ProviderID))
	// 单次 run 的 token 花费上限（全局设置；0 = 不限）。
	maxRunTokens := int(s.settingFloat(ctx, domain.SettingKeyChatMaxRunTokens, 0))
	// 采样参数两层合并：请求级 > 全局默认（Provider 级已被 §3.4 下线）。
	temperature, thinking := s.sampleParams(ctx, params)

	// 装配内核：主循环只做「请求 → 执行 → 回填」，权限 / 审批 / 信任 / 续接全在
	// 中间件与钩子里（见 chat_agent.go）。
	stopContinues := 0
	loop := s.newCoreLoop(coreLoopSpec{
		Ses:            ses,
		RunID:          runID,
		AssistantMsgID: assistantMsgID,
		Def:            def,
		Provider:       prov,
		Model:          runModel,
		System:         sysText,
		ToolNames:      toolNames,
		Sink:           core.FuncSink(mapper.handle),
		Temperature:    temperature,
		Thinking:       thinking,
		MaxRunTokens:   maxRunTokens,
		MaxInput:       budget,
		ExecTimeout:    toolCallTimeout(def),
		Hooks: core.Hooks{
			// 轮间插话：跑过工具后允许用户纠偏，下一轮生效。
			Steering: func(context.Context) []*llm.Message { return s.steers.drain(ses.ID) },
			// 收尾续接：待发送消息优先；没有则问 Stop 钩子——返回 block 即带着反馈再跑一轮。
			// 连续续跑上限 3 次（与协议一致），防止「每次都要求再来一轮」把 run 拖成无限循环。
			FollowUp: func(fctx context.Context) []*llm.Message {
				if msgs := s.steers.drain(ses.ID); len(msgs) > 0 {
					return msgs
				}
				if s.hookRunner == nil || stopContinues >= stopHookMaxContinues {
					return nil
				}
				res := s.hookRunner.Stop(fctx, ses.ID, runID, "", stopContinues > 0)
				if !res.Block {
					return nil
				}
				stopContinues++
				// 以 user 消息回注：模型据此补齐后才允许收尾
				return []*llm.Message{{Role: llm.RoleUser, Content: res.Reason}}
			},
		},
	})

	// 补充输入能力注入 ctx：request_input 工具暂停 run 问用户
	if s.approval != nil {
		ctx = tool.WithInputRequester(ctx, s.approval)
	}
	// 委派能力注入 ctx：delegate_task 经此发起子 Agent（独立上下文与预算，只回传摘要）。
	ctx = tool.WithDelegator(ctx, s.newCoreDelegator(ses, runID, core.FuncSink(mapper.handle), runModel, toolNames))

	pkg.L.Info("chat run start",
		"runID", runID, "sessionID", ses.ID, "providerID", ses.ProviderID, "model", runModel,
		"agent", def.Name, "history", len(llmMsgs), "tools", len(toolNames),
		"skill", strings.Join(runState.SkillTools, ","), "resume", resume)
	var res *core.Outcome
	var runErr error
	if resume {
		// 续跑：前端按同一 runID 挂 SSE；Resume 不重发 run.start，这里补 stream.start
		s.emit(runID, ses.ID, "chat:stream.start", map[string]any{"model": runModel, "resumed": true})
		res, runErr = loop.Resume(ctx)
	} else {
		// 历史先清洗：空 assistant 占位与孤儿 tool 会让上游直接 400。
		res, runErr = loop.Run(ctx, core.RebuildHistory(llmMsgs))
	}
	if res == nil {
		res = &core.Outcome{Reason: core.ReasonError, Err: runErr}
	}
	elapsed := time.Since(runStart).Milliseconds()

	// 用量先落明细再统计：明细是仪表盘三线图的唯一数据源，失败只告警不阻断（不因计量丢回答）。
	s.persistUsage(ctx, ses, runID, assistantMsgID, runModel, res)
	logRunResult(ses, runID, runModel, res, len(mapper.toolCalls), elapsed, resume)
	// 收尾见 chat_finalize.go：终态落库 → 反幻觉核验 → run 后沉淀。
	s.finalizeRun(ctx, ses, runID, assistantMsgID, runModel, res, mapper)
	if res.Err != nil {
		return res
	}
	s.verifyArtifactClaims(ctx, ses, runID, runModel, res, mapper)
	s.afterRun(ctx, ses, def, runID, runModel, userInput, llmMsgs, res)
	return res
}

// captureTranscript 组装沉淀用的对话副本：本轮发送给 LLM 的消息 + 用户输入 + 最终回答。
//
//nolint:unused
func captureTranscript(sent []*llm.Message, userInput, reply string) []llm.Message {
	transcript := make([]llm.Message, 0, len(sent)+2)
	for _, m := range sent {
		if m == nil {
			continue
		}
		transcript = append(transcript, *m)
	}
	if userInput != "" {
		transcript = append(transcript, *llm.UserMessage(userInput))
	}
	if reply != "" {
		transcript = append(transcript, *llm.AssistantMessage(reply, nil))
	}
	return transcript
}

// failRun 错误落库：标 failed + 发 chat:error。
func (s *ChatService) failRun(ctx context.Context, runID, sessionID, assistantMsgID string, err error) {
	s.finishExec(runID, core.StateFailed)
	now := time.Now().UnixMilli()
	_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
		"status":      domain.MessageStatusFailed,
		"stop_reason": domain.StopReasonError,
		"updated_at":  now,
	})
	s.emit(runID, sessionID, "chat:error", map[string]any{"code": 5000, "message": err.Error()})
	// 与正常收尾共用 domain.ChatDoneEvent 形状：前端只认一种终态载荷。
	s.emit(runID, sessionID, "chat:done", domain.ChatDoneEvent{
		Status:    "failed",
		Reason:    string(domain.StopReasonError),
		MessageID: assistantMsgID,
	})
}

// providerParams 拉 Provider DO → *llm.ProviderParams；DO 缺/取失败返回 nil（走全局默认）。

// finishExec 执行平面终态收束（registry 未注入时空操作）。
func (s *ChatService) finishExec(runID string, state core.ExecutionState) {
	if s.execs != nil {
		s.execs.Finish(runID, state)
	}
}

// execState run 终止原因 → 执行平面终态：取消优先，错误落 failed，其余按完成收束。
func execState(reason string) core.ExecutionState {
	switch reason {
	case core.ReasonCancelled:
		return core.StateCancelled
	case core.ReasonError:
		return core.StateFailed
	default:
		return core.StateCompleted
	}
}
