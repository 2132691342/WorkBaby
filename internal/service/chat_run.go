// chat_run.go · 会话编排内核（PI Phase 3 合并 chat.go / chat_agent.go / chat_prepare.go /
// chat_finalize.go / chat_runs.go / chat_stream.go 为单文件）。
//
// 章节顺序：
//   1. ChatService 类型与依赖（ChatDeps / 结构 / NewChatService / MissingDeps / 锁）
//   2. Emitter（事件统一出口：注入归属 + seq + 重放缓冲）
//   3. run 生命周期（SendStream / runLLM / executeAgent / CancelStream / ResumeRun / ReapInterrupted）
//   4. 占位与序号（prepareRun / resolveAttachments / nextSeq / allocSeq）
//   5. 注入队列（QueueSteer / RunAgent / steerQueue）
//   6. 运行记录（ListRunRecords / startRunRecord / finishRunRecord / toRunRecordRESP）
//   7. 检查点存储（sqlCheckpointStore / NewSQLCheckpointStore）
//   8. 运行前装配（syncWorkspaceSkills / applyAgentOverrides / promptHookContext / loadRunHistory / systemWithHookContext）
//   9. 工具暴露与采样（exposedToolDefs / exposedToolNames / sampleParams / toolCallTimeout / failOutcome）
//  10. agent.Loop 装配（coreLoopSpec / newCoreLoop / guardsFor / hookGuard / pathPolicy / coreMode / allowRules / gateAllowSet / coreApprover / approvalCommand / refusal）
//  11. 收尾与计费（finalizeRun / verifyArtifactClaims / afterRun / logRunResult / persistUsage / persistUsageRow / persistUsageRowFrom / modelPricing）
//  12. 反幻觉核验（artifactClaimRe / artifactPathRe / claimsArtifact / claimedPaths / writeToolNames / evidenceForClaim / changeEvidence）
//  13. 错误与执行平面（failRun / finishExec / execState / extractTargetDir）
//  14. 策略门常量（gateInternalAllowTools）
//  15. 事件映射（coreEventMapper / newCoreEventMapper / persistBlock / flushText / toolResultContentOf / handle）
//  16. emit（service → SSE 唯一出口）
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/capability"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/modelmeta"
	"WorkBaby/internal/llm/registry"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
	"WorkBaby/internal/tool/planmode"
)

// =====================================================================
// 1. ChatService 类型与依赖
// =====================================================================

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
	Checkpoints agent.CheckpointStore
	EventLog    *event.RunEventLog
	// Emitter 事件出口；nil 时用 Bus + EventLog 现场构造。装配方传入共享实例，
	// 可保证 chat / task / approval 等域的 seq 落在同一条序列上。
	Emitter    *Emitter
	Blocks     *repo.MessageBlockRepo
	RunRecords *repo.RunRecordRepo
	Executions *agent.ExecutionRegistry

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

// ChatService 会话 + 消息编排；接入 agent.Loop 调用真实 LLM。
type ChatService struct {
	sessions    *repo.ChatSessionRepo
	messages    *repo.MessageRepo
	provRepo    *repo.AiProviderRepo
	setRepo    *repo.SystemSettingRepo
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
	checkpoints agent.CheckpointStore                // 检查点存储（SQL 默认；nil = 关闭）
	runRec      *repo.RunRecordRepo                 // 运行历史索引；nil = 不记录
	execs       *agent.ExecutionRegistry             // 执行平面
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

// =====================================================================
// 2. Emitter（事件统一出口）
// =====================================================================

// Emitter 事件统一出口：注入归属、分配 seq、写重放缓冲后广播。全应用唯一出口。
type Emitter struct {
	bus *event.Bus
	log *event.RunEventLog
}

// NewEmitter 构造事件出口；log 可为 nil（退化为无重放）。
func NewEmitter(bus *event.Bus, log *event.RunEventLog) *Emitter {
	return &Emitter{bus: bus, log: log}
}

// Emit 发布事件；payload 支持 map（高频增量，零转换）或领域结构体（JSON 归一）。
// runID 为空 = 会话级事件（目标状态等）：仍广播并带 session_id，由订阅方按会话过滤。
func (e *Emitter) Emit(runID, sessionID, name string, payload any) {
	if e == nil || e.bus == nil {
		return
	}
	m := payloadMap(payload)
	m["run_id"] = runID
	m["session_id"] = sessionID
	if runID != "" && e.log != nil {
		e.log.Append(runID, name, m)
	}
	e.bus.Publish(name, m)
}

// EmitCtx 从 ctx 提取 run/session 归属后发布（工具、审批、文件变更、工件等能力域使用）。
func (e *Emitter) EmitCtx(ctx context.Context, name string, payload any) {
	e.Emit(agent.RunIDFromCtx(ctx), agent.SessionIDFromCtx(ctx), name, payload)
}

// payloadMap 归一化为事件载荷 map：map 直接使用；结构体经 JSON 往返（低频事件可接受）。
func payloadMap(payload any) map[string]any {
	switch p := payload.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return p
	default:
		bs, err := json.Marshal(p)
		if err != nil {
			return map[string]any{}
		}
		var m map[string]any
		if json.Unmarshal(bs, &m) != nil {
			return map[string]any{}
		}
		return m
	}
}

// =====================================================================
// 3. run 生命周期
// =====================================================================

// defaultAgentName chat 主入口的默认 Agent。
const defaultAgentName = "default"

// CancelStream 中断指定 session 正在跑的 run（前端「停止」按钮）。
//
// 幂等：无 run 或 sessionID 为空直接 nil；harness 收到 ctx 取消后 emit ReasonCancelled → chat:done。
func (s *ChatService) CancelStream(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	s.runs.cancel(sessionID)
	s.steers.clear(sessionID) // run 已终止，排队中的注入消息一并作废
	return nil
}

// runIDs 一次 run 的身份（runID + user/assistant 消息 ID）。
type runIDs struct {
	RunID          string
	UserMsgID      string
	AssistantMsgID string
}

// SendStream 建一个流式 run：会话级锁 → 取会话 → 补齐 Provider/Model → 占位落库 → 异步跑 Loop。
//
// 终态事件由 executeAgent 的 mapper 延迟到 assistant 消息落库后发出，
// 前端见 chat:done 时立刻拉权威快照能看到完整过程块。
func (s *ChatService) SendStream(ctx context.Context, sessionID, content string, fileIDs []string, params agent.RequestParams) (*domain.SendStreamResult, error) {
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
	ids, err := s.prepareRun(ctx, ses, content, atts, agent.ScopeChatTurn, agentName)
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
		s.runLLM(runCtx, ses, ids.RunID, ids.AssistantMsgID, content, params, agent.Agent(agentName), false)
	}()

	return &domain.SendStreamResult{
		RunID:          ids.RunID,
		SessionID:      sessionID,
		UserMsgID:      ids.UserMsgID,
		AssistantMsgID: ids.AssistantMsgID,
	}, nil
}

// runLLM 异步跑 LLM（多轮 ReAct）：墙钟预算 + 结果落库 + 发 chat:done / chat:error。
// resume=true 时从检查点续跑同一 runID（中断/崩溃后恢复）。
func (s *ChatService) runLLM(parentCtx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, userInput string, params agent.RequestParams, def agent.Definition, resume bool) {
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

// executeAgent 跑一次 Agent：装配上下文 → 跑 harness → 落库/计量/记忆。
// chat 与后台任务共用同一条路径（唯一差异是调用方给的 ctx 与 Agent 定义）。
func (s *ChatService) executeAgent(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, userInput string, params agent.RequestParams, def agent.Definition, resume bool) *agent.Outcome {
	// 运行前装配见下：技能叠加 → Agent 覆盖 → 用户钩子 → 历史 → system → 工具暴露。
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
	// 中间件与钩子里。
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
		Sink:           agent.FuncSink(mapper.handle),
		Temperature:    temperature,
		Thinking:       thinking,
		MaxRunTokens:   maxRunTokens,
		MaxInput:       budget,
		ExecTimeout:    toolCallTimeout(def),
		Hooks: agent.Hooks{
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
	ctx = tool.WithDelegator(ctx, s.newCoreDelegator(ses, runID, agent.FuncSink(mapper.handle), runModel, toolNames))

	pkg.L.Info("chat run start",
		"runID", runID, "sessionID", ses.ID, "providerID", ses.ProviderID, "model", runModel,
		"agent", def.Name, "history", len(llmMsgs), "tools", len(toolNames),
		"skill", strings.Join(runState.SkillTools, ","), "resume", resume)
	var res *agent.Outcome
	var runErr error
	if resume {
		// 续跑：前端按同一 runID 挂 SSE；Resume 不重发 run.start，这里补 stream.start
		s.emit(runID, ses.ID, "chat:stream.start", map[string]any{"model": runModel, "resumed": true})
		res, runErr = loop.Resume(ctx)
	} else {
		// 历史先清洗：空 assistant 占位与孤儿 tool 会让上游直接 400。
		res, runErr = loop.Run(ctx, agent.RebuildHistory(llmMsgs))
	}
	if res == nil {
		res = &agent.Outcome{Reason: agent.ReasonError, Err: runErr}
	}
	elapsed := time.Since(runStart).Milliseconds()

	// 用量先落明细再统计：明细是仪表盘三线图的唯一数据源，失败只告警不阻断（不因计量丢回答）。
	s.persistUsage(ctx, ses, runID, assistantMsgID, runModel, res)
	logRunResult(ses, runID, runModel, res, len(mapper.toolCalls), elapsed, resume)
	// 收尾：终态落库 → 反幻觉核验 → run 后沉淀。
	s.finalizeRun(ctx, ses, runID, assistantMsgID, runModel, res, mapper)
	if res.Err != nil {
		return res
	}
	s.verifyArtifactClaims(ctx, ses, runID, runModel, res, mapper)
	s.afterRun(ctx, ses, def, runID, runModel, userInput, llmMsgs, res)
	return res
}

// ResumeRun 从检查点续跑：中断/崩溃后以同一 runID 恢复，前端重挂 SSE 续渲染。
// 已完成工具调用经 StepRecords 复用，不重放副作用；审批/补充输入会重新发起。
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
		s.runLLM(runCtx, ses, runID, cp.AssistantMsgID, "", agent.RequestParams{}, agent.Agent(defaultAgentName), true)
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

// =====================================================================
// 4. 占位与序号
// =====================================================================

// prepareRun 落 user + assistant 占位消息并登记执行平面 run（chat 与后台任务共用）。
// run 身份在消息落库前确定：runID 需先写入消息行，供前端按 run 过滤事件。
func (s *ChatService) prepareRun(ctx context.Context, ses *domain.ChatSessionDO, content string, atts []domain.MessageAttachment, scope agent.Scope, agentName string) (runIDs, error) {
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
		s.execs.Register(&agent.ExecutionRun{
			RunID:     runID,
			SessionID: ses.ID,
			Scope:     scope,
			AgentName: agentName,
			State:     agent.StateRunning,
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

// allocSeq 分配会话内连续 n 个消息序号（进程内单调水位，首次取库内最大值 + 1）。
// 工具结果与注入消息都走同一分配器：撞号会让增量分页漏消息。
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

// =====================================================================
// 5. 注入队列与后台运行
// =====================================================================

// steerQueue 会话级注入队列：run 中进行时用户新发的消息先入队，由 harness 的
// steering（本轮工具跑完后）与 follow-up（本轮收尾后）两条注入缝消费；随 run 结束清空。
type steerQueue struct {
	mu   sync.Mutex
	msgs map[string][]*llm.Message
}

func newSteerQueue() *steerQueue {
	return &steerQueue{msgs: map[string][]*llm.Message{}}
}

// push 入队一条消息。
func (q *steerQueue) push(sessionID string, msg *llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.msgs[sessionID] = append(q.msgs[sessionID], msg)
}

// drain 取走该会话全部排队消息（注入缝消费；取空即删桶）。
func (q *steerQueue) drain(sessionID string) []*llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.msgs[sessionID]
	if len(out) == 0 {
		return nil
	}
	delete(q.msgs, sessionID)
	return out
}

// clear 丢弃该会话排队消息（run 结束 / 被取消）。
func (q *steerQueue) clear(sessionID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.msgs, sessionID)
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

// RunAgent 同步跑一轮 Agent（后台任务入口），阻塞直到 run 结束并返回结果摘要。
// 与 SendStream 的区别：不占用会话的活动 run 槽，与前台聊天可并行。
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
	ids, err := s.prepareRun(ctx, ses, userInput, nil, agent.ScopeTask, agentName)
	unlock()
	if err != nil {
		return nil, err
	}
	def := agent.Agent(agentName)
	res := s.executeAgent(ctx, ses, ids.RunID, ids.AssistantMsgID, userInput, agent.RequestParams{}, def, false)
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

// =====================================================================
// 6. 运行记录
// =====================================================================

// ListRunRecords 运行历史（倒序分页）；sessionID 为空表示全部会话。
func (s *ChatService) ListRunRecords(ctx context.Context, sessionID string, limit, offset int) (domain.RunRecordListRESP, error) {
	if s.runRec == nil {
		return domain.RunRecordListRESP{Items: []domain.RunRecordRESP{}}, nil
	}
	rows, total, err := s.runRec.List(ctx, sessionID, limit, offset)
	if err != nil {
		return domain.RunRecordListRESP{}, err
	}
	out := make([]domain.RunRecordRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toRunRecordRESP(&rows[i]))
	}
	return domain.RunRecordListRESP{Items: out, Total: total}, nil
}

// startRunRecord 记录 run 启动；失败只记日志。
func (s *ChatService) startRunRecord(ctx context.Context, runID string, ses *domain.ChatSessionDO) {
	if s.runRec == nil || runID == "" {
		return
	}
	if err := s.runRec.Start(ctx, domain.RunRecordDO{
		RunID:     runID,
		SessionID: ses.ID,
		Model:     ses.Model,
		Status:    domain.RunStatusRunning,
		StartedAt: time.Now().UnixMilli(),
	}); err != nil {
		pkg.L.Warn("save run record failed", "runID", runID, "err", err.Error())
	}
}

// finishRunRecord 回填终态与用量；错误终态按 status=error 落，便于历史页筛选失败运行。
// 用量取 Accumulated（全 run 累计）：Usage 是末轮 per-turn 口径（供上下文占用展示），
// 历史页展示「本次运行消耗」必须与 token_usages 明细 SUM 对得上。
func (s *ChatService) finishRunRecord(ctx context.Context, runID string, out *agent.Outcome) {
	if s.runRec == nil || runID == "" {
		return
	}
	status := domain.RunStatusDone
	if out.Err != nil {
		status = domain.RunStatusError
	}
	upd := map[string]any{
		"status":        status,
		"reason":        out.Reason,
		"turns":         out.Turns,
		"input_tokens":  out.Usage.InputTokens,
		"output_tokens": out.Usage.OutputTokens,
		"cache_read":    out.Usage.CacheReadTokens,
		"total_tokens":  out.Usage.TotalTokens,
		"llm_ms":        out.Timings.LLMMs,
		"tools_ms":      out.Timings.ToolsMs,
		"compress_ms":   out.Timings.CompressMs,
		"ended_at":      time.Now().UnixMilli(),
	}
	if err := s.runRec.Finish(ctx, runID, upd); err != nil {
		pkg.L.Warn("update run record failed", "runID", runID, "err", err.Error())
	}
}

func toRunRecordRESP(r *domain.RunRecordDO) domain.RunRecordRESP {
	return domain.RunRecordRESP{
		RunID: r.RunID, SessionID: r.SessionID, Model: r.Model,
		Status: r.Status, Reason: r.Reason, Turns: r.Turns,
		InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CacheRead: r.CacheRead, TotalTokens: r.TotalTokens,
		LLMMs: r.LLMMs, ToolsMs: r.ToolsMs, CompressMs: r.CompressMs,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt,
	}
}

// =====================================================================
// 7. 检查点存储
// =====================================================================

// sqlCheckpointStore 把 agent.Checkpoint 持久化到 agent_checkpoints 表。
// 每 run 只保留最新一轮快照（Resume 只认最后一轮，避免磁盘放大）；
// 序列化在 service 层收敛，repo 只做无业务语义读写。
type sqlCheckpointStore struct {
	repo *repo.AgentCheckpointRepo
}

// NewSQLCheckpointStore 构造 SQL 检查点存储（实现 agent.CheckpointStore）。
func NewSQLCheckpointStore(r *repo.AgentCheckpointRepo) agent.CheckpointStore {
	return &sqlCheckpointStore{repo: r}
}

// Append 保存一轮检查点（(run_id, turn) upsert），随后清掉同 run 的更早轮次。
func (s *sqlCheckpointStore) Append(cp *agent.Checkpoint) error {
	ctx := context.Background()
	messages, err := json.Marshal(cp.Messages)
	if err != nil {
		return pkg.Wrap(5005, "checkpoint messages marshal failed", err)
	}
	usage, err := json.Marshal(cp.Usage)
	if err != nil {
		return pkg.Wrap(5005, "checkpoint usage marshal failed", err)
	}
	stepsJSON := ""
	if len(cp.Steps) > 0 {
		bs, serr := json.Marshal(cp.Steps)
		if serr != nil {
			return pkg.Wrap(5005, "checkpoint steps marshal failed", serr)
		}
		stepsJSON = string(bs)
	}
	row := &domain.AgentCheckpointDO{
		ID:                 pkg.NewID("CP"),
		SessionID:          cp.SessionID,
		RunID:              cp.RunID,
		Turn:               cp.Turn,
		AssistantMessageID: cp.AssistantMsgID,
		MessagesJSON:       string(messages),
		// StateJSON 是旧内核的停滞计数载体；新内核的守卫自带计数，
		// 写空对象保持列兼容，不做数据迁移。
		StateJSON: "{}",
		UsageJSON: string(usage),
		StepsJSON: stepsJSON,
		Content:   cp.Content,
		Thinking:  cp.Thinking,
	}
	if err := s.repo.Save(ctx, row); err != nil {
		return err
	}
	return s.repo.DeleteBeforeTurn(ctx, cp.RunID, cp.Turn)
}

// LoadLast 取最新一轮检查点。
func (s *sqlCheckpointStore) LoadLast(runID string) (*agent.Checkpoint, error) {
	row, err := s.repo.LoadLast(context.Background(), runID)
	if err != nil {
		return nil, err
	}
	cp := &agent.Checkpoint{
		RunID:          row.RunID,
		SessionID:      row.SessionID,
		Turn:           row.Turn,
		AssistantMsgID: row.AssistantMessageID,
		Content:        row.Content,
		Thinking:       row.Thinking,
		CreatedAt:      row.CreatedAt,
	}
	if err := json.Unmarshal([]byte(row.MessagesJSON), &cp.Messages); err != nil {
		return nil, pkg.Wrap(5005, "checkpoint messages parse failed", err)
	}
	if err := json.Unmarshal([]byte(row.UsageJSON), &cp.Usage); err != nil {
		return nil, pkg.Wrap(5005, "checkpoint usage parse failed", err)
	}
	if row.StepsJSON != "" {
		if err := json.Unmarshal([]byte(row.StepsJSON), &cp.Steps); err != nil {
			return nil, pkg.Wrap(5005, "checkpoint steps parse failed", err)
		}
	}
	return cp, nil
}

// Cleanup 保留最近 keep 个 run 的检查点。
func (s *sqlCheckpointStore) Cleanup(sessionID string, keep int) error {
	return s.repo.CleanupRuns(context.Background(), sessionID, keep)
}

// =====================================================================
// 8. 运行前装配
// =====================================================================

// syncWorkspaceSkills 按会话工作区叠加技能目录（未变化时零开销）；失败不阻断 run。
func (s *ChatService) syncWorkspaceSkills(ctx context.Context, ses *domain.ChatSessionDO) {
	if s.skillSync == nil {
		return
	}
	if err := s.skillSync(ctx, ses.WorkspacePath); err != nil {
		pkg.L.Warn("sync workspace skills failed", "sessionID", ses.ID, "err", err.Error())
	}
}

// applyAgentOverrides 应用 Agent 定义的运行期覆盖，返回实际使用的模型与采样参数。
// 模型：Agent 定义优先于会话模型（优先级差必须让用户看见）；推理强度：请求级优先。
func (s *ChatService) applyAgentOverrides(ctx context.Context, ses *domain.ChatSessionDO, runID string, def agent.Definition, params agent.RequestParams) (string, agent.RequestParams) {
	runModel := def.EffectiveModel(ses.Model)
	if runModel != ses.Model {
		pkg.L.Info("agent overrides session model",
			"sessionID", ses.ID, "agent", def.Name, "sessionModel", ses.Model, "model", runModel)
		s.emit(runID, ses.ID, "chat:warn", domain.ChatWarnEvent{
			Kind:         "agent_model_override",
			Agent:        def.Name,
			SessionModel: ses.Model,
			Model:        runModel,
			Message:      "本次运行由子智能体定义指定了模型 " + runModel + "（会话模型 " + ses.Model + " 已被覆盖）",
		})
	}
	if params.Thinking == nil && def.Thinking != "" {
		params.Thinking = llm.ThinkingFromEffort(def.Thinking)
	}
	return runModel, params
}

// promptHookContext 触发 UserPromptSubmit 用户钩子：返回可并入 system 的补充上下文；
// 被钩子阻断时返回错误。续跑不重复触发——该事件属于「用户提交」这一次动作。
func (s *ChatService) promptHookContext(ctx context.Context, ses *domain.ChatSessionDO, runID, userInput string, resume bool) (string, error) {
	if s.hookRunner == nil || resume {
		return "", nil
	}
	blocked, reason, extra := s.hookRunner.UserPromptSubmit(ctx, ses.ID, runID, userInput, ses.WorkspacePath, ses.PermissionMode)
	if blocked {
		return "", pkg.New(8610, "请求被用户钩子阻断", reason)
	}
	return extra, nil
}

// loadRunHistory 拉会话历史并转成 LLM 消息（含工具调用上下文与图片附件）。
// 辅助对话（侧链路）把主会话历史按预算前置拼接，追问不必重复交代背景。
func (s *ChatService) loadRunHistory(ctx context.Context, ses *domain.ChatSessionDO) ([]*llm.Message, error) {
	hists, err := s.messages.ListBySession(ctx, ses.ID, 0, 0)
	if err != nil {
		return nil, err
	}
	vision := s.providerVision(ctx, ses.ProviderID)
	llmMsgs, err := s.toLLMMessagesWithVision(ctx, hists, vision)
	if err != nil {
		return nil, err
	}
	parentMsgs, err := s.sideParentMessages(ctx, ses, vision)
	if err != nil {
		return nil, err
	}
	if len(parentMsgs) > 0 {
		llmMsgs = append(parentMsgs, llmMsgs...)
		pkg.L.Debug("side parent prefix", "sessionID", ses.ID, "prefixMsgs", len(parentMsgs))
	}
	return llmMsgs, nil
}

// systemWithHookContext 把用户钩子的补充段并入 system 尾部（SessionStart 仅首轮 + UserPromptSubmit），
// 返回最终 system 正文。追加而非新开 system 消息：多 system 段在部分上游被拒。
func (s *ChatService) systemWithHookContext(ctx context.Context, ses *domain.ChatSessionDO, runID string, sys *llm.Message, promptCtx string, resume bool) string {
	if s.hookRunner != nil {
		var parts []string
		if !resume {
			if txt := s.hookRunner.SessionStart(ctx, ses.ID, runID, "startup", ses.WorkspacePath, ses.PermissionMode); txt != "" {
				parts = append(parts, txt)
			}
		}
		if promptCtx != "" {
			parts = append(parts, promptCtx)
		}
		if len(parts) > 0 {
			extra := "## 用户钩子上下文\n\n" + strings.Join(parts, "\n\n")
			if sys != nil {
				sys.Content = sys.Content + "\n\n" + extra
			} else {
				sys = &llm.Message{Role: llm.RoleSystem, Content: extra}
			}
		}
	}
	if sys == nil {
		return ""
	}
	return sys.Content
}

// =====================================================================
// 9. 工具暴露与采样
// =====================================================================

// exposedToolDefs 工具暴露的唯一入口：启用工具 → Skill 白名单（命中技能时）→ Agent 工具策略。
// 上下文占用透视走同一入口（skillTools 传 nil），保证「看到的工具」等于「实发的工具」。
func (s *ChatService) exposedToolDefs(ctx context.Context, def agent.Definition, skillTools []string) []llm.ToolDefinition {
	return def.FilterTools(s.tools.LLMDefinitionsFiltered(ctx, skillTools))
}

// exposedToolNames 暴露工具名清单（内核 Expose 用；空清单表示不限制）。
func (s *ChatService) exposedToolNames(ctx context.Context, def agent.Definition, skillTools []string) []string {
	defs := s.exposedToolDefs(ctx, def, skillTools)
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

// sampleParams 采样参数两层合并：请求级 > 全局默认（Provider 级不参与）。
func (s *ChatService) sampleParams(ctx context.Context, params agent.RequestParams) (*float64, *llm.ThinkingConfig) {
	dl := s.defaults(ctx)
	temperature := params.Temperature
	if temperature == nil {
		t := dl.Temperature
		temperature = &t
	}
	thinking := params.Thinking
	if thinking == nil {
		thinking = dl.Thinking
	}
	return temperature, thinking
}

// toolCallTimeout 单次工具执行超时：Agent 预算优先，否则内核默认 5 分钟。
func toolCallTimeout(def agent.Definition) time.Duration {
	if def.Budget.ToolCallTimeout > 0 {
		return def.Budget.ToolCallTimeout
	}
	return 5 * time.Minute
}

// failOutcome run 启动期失败的统一出口：落库 + 发 chat:error + 返回错误终态。
func (s *ChatService) failOutcome(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID string, err error) *agent.Outcome {
	s.failRun(ctx, runID, ses.ID, assistantMsgID, err)
	return &agent.Outcome{Reason: agent.ReasonError, Err: err}
}

// =====================================================================
// 10. agent.Loop 装配
// =====================================================================

// coreLoopSpec 一次 run 的装配输入。
type coreLoopSpec struct {
	Ses            *domain.ChatSessionDO
	RunID          string
	AssistantMsgID string
	Def            agent.Definition
	Provider       llm.Provider
	Model          string
	System         string
	ToolNames      []string
	Sink           agent.Sink
	Temperature    *float64
	Thinking       *llm.ThinkingConfig
	MaxTokens      *int
	MaxInput       int
	MaxRunTokens   int
	ExecTimeout    time.Duration
	Hooks          agent.Hooks
	// Approver 覆盖默认审批门。默认 coreApprover 阻塞等用户决策（聊天 run）；
	// 后台任务等无人值守 run 注入 taskApprover（只吃免审授权，不弹窗）。
	Approver agent.Approver
}

// newCoreLoop 构造一次 run 的内核实例。
func (s *ChatService) newCoreLoop(spec coreLoopSpec) *agent.Loop {
	cfg := agent.Config{
		Model:        spec.Model,
		System:       spec.System,
		MaxInput:     spec.MaxInput,
		MaxRunTokens: spec.MaxRunTokens,
		Temperature:  spec.Temperature,
		MaxTokens:    spec.MaxTokens,
		Thinking:     spec.Thinking,
		Exec:         agent.ExecOptions{Timeout: spec.ExecTimeout},
	}
	spec.Def.Budget.Apply(&cfg)

	loop := agent.New(spec.Provider, s.tools.Registry(), cfg).
		WithSink(spec.Sink).
		WithMeta(agent.Meta{RunID: spec.RunID, SessionID: spec.Ses.ID, Agent: spec.Def.Name}).
		WithAssistantMessage(spec.AssistantMsgID).
		WithCompressor(agent.MicroCompressor{}).
		WithHooks(spec.Hooks).
		Expose(spec.ToolNames)
	// 护栏链在 Loop 构造后挂入：RepeatGuard 经 StepsStore 消费检查点恢复的步骤记忆
	// （续跑命中直接复用结果，不重放副作用；适配器解引用，Resume 重建后仍生效）。
	loop.WithGuard(s.guardsFor(spec, loop.StepsStore())...).
		// 建流瞬时错误重试：退避前进度透出为 agent.retry 事件，
		// 编排层（service/chat_run）桥接为前端 chat:retry 横幅。
		WithRetry(llm.DefaultRetryPolicy(), func(attempt int, delay time.Duration) {
			spec.Sink.Emit(agent.Event{
				Kind: agent.EventRetry, RunID: spec.RunID, SessionID: spec.Ses.ID, Agent: spec.Def.Name,
				Payload: agent.RetryPayload{Attempt: attempt, DelayMs: delay.Milliseconds()},
			})
		})

	if s.checkpoints != nil {
		loop.WithCheckpoints(s.checkpoints)
	}
	return loop
}

// guardsFor 组装护栏中间件链（顺序即语义，外到内，任一环拒绝即短路）：
// ExposeGuard → SchemaGuard → PolicyGuard → hookGuard → AdaptiveLoopGuard → RepeatGuard。
// RepeatGuard 的 store 来自 Loop 步骤记忆：检查点恢复的键在续跑时直接复用结果。
func (s *ChatService) guardsFor(spec coreLoopSpec, steps agent.StepStore) []agent.Middleware {
	var exposed map[string]bool
	if len(spec.ToolNames) > 0 {
		exposed = make(map[string]bool, len(spec.ToolNames))
		for _, n := range spec.ToolNames {
			exposed[n] = true
		}
	}

	// 审批门：spec.Approver 优先（后台任务等无人值守 run），否则阻塞等用户决策。
	approver := agent.Approver(coreApprover{svc: s.approval})
	if spec.Approver != nil {
		approver = spec.Approver
	}

	ms := []agent.Middleware{
		agent.ExposeGuard(exposed),
		agent.SchemaGuard(),
		// PolicyGuard 现在统一收口安全门：注入/路径/计划/审批四件事。
		// 注入检测无条件生效；路径预检与审批门按需挂入。
		agent.PolicyGuard(
			s.coreMode(context.Background(), spec.Ses.PermissionMode),
			allowRules{},
			approver,
			s.pathPolicy(),
		),
	}
	// 用户钩子在策略门之后：内置护栏先裁决，钩子只处理「本机护栏已放行、但用户另有规矩」的场景。
	ms = append(ms, s.hookGuard(spec.Ses, spec.RunID))
	// 失败改道闸门：在 RepeatGuard 之前计数失败次数，达标后给模型注入改道提示；
	// 不影响 RepeatGuard 的同参熔断（两者维度互补：tool-name 维度 vs 同参 key 维度）。
	ms = append(ms, agent.AdaptiveLoopGuard(3))
	// 循环与停滞熔断放最后：前面的拒绝（未暴露/越权）不算「重复调用」。
	return append(ms, agent.RepeatGuard(3, steps))
}

// hookGuard 用户钩子闸门（子进程协议）：PreToolUse 可放行 / 升级确认 / 拦截，
// PostToolUse 的附加上下文并入回执。钩子自身故障不阻断（内部已兜底放行）。
func (s *ChatService) hookGuard(ses *domain.ChatSessionDO, runID string) agent.Middleware {
	if s.hookRunner == nil {
		return func(next agent.Handler) agent.Handler { return next }
	}
	return func(next agent.Handler) agent.Handler {
		return func(ctx context.Context, call agent.Call) tool.ToolResult {
			pre := s.hookRunner.PreToolUse(ctx, ses.ID, runID, call.Name, call.ID,
			ses.WorkspacePath, ses.PermissionMode, call.Args)
			switch pre.Decision {
			case "deny":
				reason := strings.TrimSpace(pre.Reason)
				if reason == "" {
					reason = "用户钩子拦截了本次调用"
				}
				return refusal(agent.RefusePolicy, reason)
			case "ask":
				if s.approval == nil || !s.approval.Approve(ctx, approvalCommand(call), tool.RiskApprovalNeeds) {
					return refusal(agent.RefuseApproval, "用户钩子要求人工确认，已被拒绝")
				}
			}

			out := next(ctx, call)
			base := []string{out.Content}
			if post := s.hookRunner.PostToolUse(ctx, ses.ID, runID, call.Name, call.ID,
			ses.WorkspacePath, ses.PermissionMode, call.Args, out.Content); strings.TrimSpace(post) != "" {
				base = append(base, "[hook] "+post)
			}
			if out.Err != nil {
				if extra := s.hookRunner.PostToolUseFailure(ctx, ses.ID, runID, call.Name, call.ID,
					ses.WorkspacePath, ses.PermissionMode, call.Args, out.Err.Error()); strings.TrimSpace(extra) != "" {
					base = append(base, "[hook] "+extra)
				}
			}
			out.Content = strings.Join(base, "\n\n")
			return out
		}
	}
}

// pathPolicy 路径/计划模式预检，返回 (ok, reason)，由 PolicyGuard 在审批前判定。
// 只对目标目录类工具（exec 的 cwd）生效：file_* 已被工作区沙箱约束，再问一次只会噪声。
func (s *ChatService) pathPolicy() agent.PathPolicy {
	return func(ctx context.Context, call agent.Call) (bool, string) {
		if s.planStore != nil {
			if reason := s.planStore.Guard(agent.SessionIDFromCtx(ctx), call.Name); reason != "" {
				return false, "计划模式拒绝: " + reason
			}
		}
		if s.trust == nil {
			return true, ""
		}
		dir, related := extractTargetDir(call.Name, call.Args)
		if !related || strings.TrimSpace(dir) == "" {
			return true, ""
		}
		if allowed, reason := s.trust.Ensure(ctx, dir); !allowed {
			return false, "路径不受信任: " + dir + "（" + reason + "）"
		}
		return true, ""
	}
}

// coreMode 会话权限模式映射。无 restricted 档：它与 default 的差异只在「ask 还是直接 deny」，
// 而 deny 会让模型无法改道，收益低于代价。
func (s *ChatService) coreMode(ctx context.Context, sessionMode string) agent.Mode {
	mode := tool.SessionModeDefault
	if v := strings.TrimSpace(sessionMode); v != "" {
		mode = tool.ParseSessionMode(v)
	} else if s.setRepo != nil {
		if row, err := s.setRepo.Get(ctx, domain.SettingKeyAgentSessionMode); err == nil && row != nil {
			mode = tool.ParseSessionMode(row.V)
		}
	}
	switch mode {
	case tool.SessionModeAutoEdit:
		return agent.ModeAutoEdit
	case tool.SessionModeYolo:
		return agent.ModeYolo
	default:
		return agent.ModeDefault
	}
}

// allowRules 显式放行清单（default 模式下本会走 ask 的那些）。
// 放行不等于无护栏：命令级裁决仍在工具内部生效（exec 的白名单与危险正则）。
type allowRules map[string]bool

// Match 实现 agent.Rules。
func (r allowRules) Match(name string) (agent.Decision, bool) {
	if r[name] || gateAllowSet[name] {
		return agent.DecisionAllow, true
	}
	return "", false
}

// gateAllowSet 由 gateInternalAllowTools 派生的集合（含通配写法）。
var gateAllowSet = func() map[string]bool {
	out := make(map[string]bool, len(gateInternalAllowTools))
	for _, n := range gateInternalAllowTools {
		out[n] = true
	}
	return out
}()

// coreApprover 把人工审批门适配到内核接口。
type coreApprover struct{ svc *ApprovalService }

// Approve 阻塞等待用户决策；命令描述用于审批卡片展示与「本会话允许」的免审记忆。
func (a coreApprover) Approve(ctx context.Context, call agent.Call, risk string) bool {
	return a.svc.Approve(ctx, approvalCommand(call), risk)
}

// approvalCommand 生成一次调用的可读描述。
// 优先用工具自述（RiskClassifier），退化为「工具名 + 参数」。
func approvalCommand(call agent.Call) string {
	if call.Tool != nil {
		if rc, ok := call.Tool.(tool.RiskClassifier); ok {
			if desc, _ := rc.ClassifyArgs(call.Args); desc != "" {
				return desc
			}
		}
	}
	if args := strings.TrimSpace(string(call.Args)); args != "" && args != "{}" {
		return call.Name + " " + args
	}
	return call.Name
}

// refusal 构造结构化拒绝回执；拒绝不是故障，模型可据此改道续跑。
func refusal(reason, hint string) tool.ToolResult {
	if strings.TrimSpace(hint) == "" {
		hint = "已被护栏拒绝"
	}
	return tool.ToolResult{Content: hint, Refused: true, Meta: map[string]string{"refused_reason": reason}}
}

// =====================================================================
// 11. 收尾与计费
// =====================================================================

// finalizeRun 写终态：用量落库 → 运行历史 → 授权回滚 → assistant 消息状态与费用。
func (s *ChatService) finalizeRun(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, runModel string, res *agent.Outcome, mapper *coreEventMapper) {
	s.finishRunRecord(ctx, runID, res)

	nowMs := time.Now().UnixMilli()
	// 终止原因统一口径：内核枚举 → 领域 stop_reason，
	// max_turns / budget_exceeded 不再被硬写成 completed，前端可差异化收尾。
	stopReason := domain.MapHarnessReason(res.Reason)
	// 权限来源回滚：以 error/cancelled 收尾的 run 不留下本次扩出的免审授权
	//（未验证的工作不保留「本会话允许」），成功/主动收尾的 run 授权保留。
	if s.approval != nil && (res.Reason == agent.ReasonCancelled || res.Reason == agent.ReasonError) {
		s.approval.RollbackRun(runID)
	}
	toolCallsJSON := ""
	if len(mapper.toolCalls) > 0 {
		if bs, err := json.Marshal(mapper.toolCalls); err == nil {
			toolCallsJSON = string(bs)
		}
	}
	// 消息级费用估算：按累计用量 × 单价（未配置单价为空串，前端不显示费用）。
	costUSD := s.modelPricing(ctx, runModel).
		CostUSD(int64(res.Usage.InputTokens), int64(res.Usage.OutputTokens), int64(res.Usage.CacheReadTokens))
	costStr := ""
	if costUSD > 0 {
		costStr = fmt.Sprintf("$%.4f", costUSD)
	}

	if res.Err != nil {
		s.finishExec(runID, agent.StateFailed)
		_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
			"content":       res.Content,
			"thinking":      res.Thinking,
			"tool_calls":    toolCallsJSON,
			"status":        domain.MessageStatusFailed,
			"stop_reason":   stopReason,
			"output_tokens": res.Usage.OutputTokens,
			"cost":          costStr,
			"updated_at":    nowMs,
		})
		return
	}
	s.finishExec(runID, execState(res.Reason))
	_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
		"content":       res.Content,
		"thinking":      res.Thinking,
		"tool_calls":    toolCallsJSON,
		"status":        domain.MessageStatusCompleted,
		"stop_reason":   stopReason,
		"input_tokens":  res.Usage.InputTokens,
		"output_tokens": res.Usage.OutputTokens,
		"cache_read":    res.Usage.CacheReadTokens,
		"total_tokens":  res.Usage.TotalTokens,
		"cost":          costStr,
		"latency_ms":    nowMs - ses.LastMessageAt,
		"updated_at":    nowMs,
	})
}

// verifyArtifactClaims 反幻觉核验：声称已产出文件，但本 run 没有对应的 file_changes
//（拒绝/失败的不算）→ 强提示揭露。证据级别是「声明路径与本 run 产物的精确比对」——
// 只跑 exec / 搜索后泛指「已生成 report」也算幻觉。
func (s *ChatService) verifyArtifactClaims(ctx context.Context, ses *domain.ChatSessionDO, runID, runModel string, res *agent.Outcome, mapper *coreEventMapper) {
	if !claimsArtifact(res.Content) || s.changeSvc == nil {
		return
	}
	changeRows, _ := s.changeSvc.ListByRun(ctx, runID, 200)
	changes := make([]changeEvidence, 0, len(changeRows))
	for _, r := range changeRows {
		changes = append(changes, changeEvidence{Path: r.RelPath})
	}
	if evidenceForClaim(claimedPaths(res.Content), mapper.toolCalls, changes) {
		return
	}
	pkg.L.Warn("unbacked artifact claim (no matching file_changes in run)",
		"runID", runID, "sessionID", ses.ID, "model", runModel,
		"claimed", fmt.Sprint(claimedPaths(res.Content)), "changes", len(changeRows))
	s.emit(runID, ses.ID, "chat:warn", domain.ChatWarnEvent{
		Kind:    "unbacked_claim",
		Message: "本条回复声称已产出文件，但本 run 没有任何对应的写文件变更记录——相关文件并不存在，请让模型实际写入后再确认。",
	})
}

// afterRun 收尾后的沉淀与续跑：记忆形成等（异步、独立超时、不阻塞响应）+ 目标模式续跑判定。
func (s *ChatService) afterRun(ctx context.Context, ses *domain.ChatSessionDO, def agent.Definition, runID, runModel, userInput string, sent []*llm.Message, res *agent.Outcome) {
	// 各能力按自身策略决定是否沉淀（如 Agent 定义关闭 Formation 时记忆能力直接跳过）
	s.caps.CaptureAll(&capability.CaptureCtx{
		SessionID:  ses.ID,
		RunID:      runID,
		UserInput:  userInput,
		Reply:      res.Content,
		Transcript: captureTranscript(sent, userInput, res.Content),
		Def:        def,
		ProviderID: ses.ProviderID,
		Model:      runModel,
	}, memoryCaptureTimeout)
	// 目标模式：活动目标在 run 正常收尾后自动校验，未达标携带下一步动作续跑
	s.maybeContinueGoal(ctx, ses, runID, res)
}

// logRunResult run 结果日志（成功与失败两条口径；字段是排障时唯一稳定的入口）。
func logRunResult(ses *domain.ChatSessionDO, runID, runModel string, res *agent.Outcome, toolCalls int, elapsed int64, resume bool) {
	if res.Err != nil {
		pkg.L.Error("chat run failed",
			"runID", runID, "sessionID", ses.ID, "model", runModel, "reason", res.Reason,
			"latencyMs", elapsed, "err", res.Err.Error())
		return
	}
	pkg.L.Info("chat run done",
		"runID", runID, "sessionID", ses.ID, "model", runModel, "reason", res.Reason,
		"stopReason", res.StopReason, "turns", res.Turns, "toolCalls", toolCalls,
		"latencyMs", elapsed, "resume", resume,
		"input", res.Usage.InputTokens, "output", res.Usage.OutputTokens,
		"cacheRead", res.Usage.CacheReadTokens, "total", res.Usage.TotalTokens)
}

// persistUsage 落 run 的按轮明细。model 是**本次实际调用**的模型（可能被 Agent 定义覆盖），
// 用它而不是会话模型，否则仪表盘的模型用量与费用会归错账。
func (s *ChatService) persistUsage(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, model string, out *agent.Outcome) {
	if s.usages == nil || len(out.PerTurn) == 0 {
		return
	}
	pricing := s.modelPricing(ctx, model)
	nowMs := time.Now().UnixMilli()
	rows := make([]domain.TokenUsageDO, 0, len(out.PerTurn))
	for _, t := range out.PerTurn {
		rows = append(rows, domain.TokenUsageDO{
			ID:               pkg.NewID("USAGE"),
			SessionID:        ses.ID,
			RunID:            runID,
			MessageID:        assistantMsgID,
			ProviderID:       ses.ProviderID,
			Model:            model,
			Source:           domain.UsageSourceChat,
			Turn:             t.Turn,
			InputTokens:      t.Usage.InputTokens,
			OutputTokens:     t.Usage.OutputTokens,
			CacheReadTokens:  t.Usage.CacheReadTokens,
			CacheWriteTokens: t.Usage.CacheWriteTokens,
			TotalTokens:      t.Usage.TotalTokens,
			CostUSD:          pricing.CostUSD(int64(t.Usage.InputTokens), int64(t.Usage.OutputTokens), int64(t.Usage.CacheReadTokens)),
			LatencyMs:        int(t.LatencyMs),
			CreatedAt:        nowMs,
		})
	}
	if err := s.usages.BatchCreate(ctx, rows); err != nil {
		pkg.L.Warn("persist token usage failed", "runID", runID, "turns", len(rows), "err", err.Error())
	}
}

// persistUsageRow 落一条附属 LLM 调用的用量明细（上下文压缩摘要等）。
// turn<0 表示非主循环调用：进总消耗统计，不进按轮次的图表。
func (s *ChatService) persistUsageRow(ctx context.Context, ses *domain.ChatSessionDO, runID, messageID string, turn int, model string, u llm.TokenUsage) {
	s.persistUsageRowFrom(ctx, ses, runID, messageID, turn, domain.UsageSourceChat, "", model, u)
}

// persistUsageRowFrom 带来源与 Agent 归属的明细落库；来源是仪表盘按场景拆分的唯一依据。
// model 为本次实际调用的模型（委派场景是子 Agent 的模型，而非父会话模型）。
func (s *ChatService) persistUsageRowFrom(ctx context.Context, ses *domain.ChatSessionDO, runID, messageID string, turn int, source domain.TokenUsageSource, agent, model string, u llm.TokenUsage) {
	if s.usages == nil {
		return
	}
	if model == "" {
		model = ses.Model
	}
	pricing := s.modelPricing(ctx, model)
	row := domain.TokenUsageDO{
		ID:               pkg.NewID("USAGE"),
		SessionID:        ses.ID,
		RunID:            runID,
		MessageID:        messageID,
		ProviderID:       ses.ProviderID,
		Model:            model,
		Source:           source,
		Agent:            agent,
		Turn:             turn,
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		TotalTokens:      u.TotalTokens,
		CostUSD:          pricing.CostUSD(int64(u.InputTokens), int64(u.OutputTokens), int64(u.CacheReadTokens)),
		CreatedAt:        time.Now().UnixMilli(),
	}
	if err := s.usages.BatchCreate(ctx, []domain.TokenUsageDO{row}); err != nil {
		pkg.L.Warn("persist usage row failed", "runID", runID, "err", err.Error())
	}
}

// modelPricing 读取模型单价（pricing.<model> KV）；未配置 / 解析失败返回零值（不计费）。
func (s *ChatService) modelPricing(ctx context.Context, model string) domain.ModelPricing {
	if s.setRepo == nil || model == "" {
		return domain.ModelPricing{}
	}
	row, err := s.setRepo.Get(ctx, domain.SettingKeyPricingPrefix+model)
	if err != nil || row == nil || row.V == "" {
		// 设置未配 → 内置模型目录兜底（近似公开单价；目录未收录为零值 = 不计费）
		mp := modelmeta.PricingFor(model)
		return domain.ModelPricing{InputPerM: mp.InputPerM, OutputPerM: mp.OutputPerM, CacheReadPerM: mp.CacheReadPerM}
	}
	var p domain.ModelPricing
	if json.Unmarshal([]byte(row.V), &p) != nil {
		return domain.ModelPricing{}
	}
	return p
}

// =====================================================================
// 12. 反幻觉核验辅助
// =====================================================================

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

// artifactClaimRe 匹配「已产出文件」类声明：完成标记 + 文件扩展名同句相邻出现。
// 保留为更宽的版本（向后兼容），路径提取走 artifactPathRe。
var artifactClaimRe = regexp.MustCompile(
	`(?i)((已|已经|成功)[^。\n]{0,12}(创建|生成|保存|写入|写出|导出|输出|制作)|(created|generated|saved|wrote|exported))` +
		`[^。\n]{0,80}\.(pptx|docx|xlsx|pdf|md|txt|csv|html|htm|zip|png|jpe?g|json|mp4|mp3)`)

// artifactPathRe 提取声明中的具体文件名（含后缀）。优先精确匹配文件名本身而非
// 扩展名——这是证据核对的关键：声明 .pdf 不算证据，声明 report.pdf 才是。
var artifactPathRe = regexp.MustCompile(
	`(?i)([\w\-\.\(\)一-龥]+\.(?:pptx|docx|xlsx|pdf|md|txt|csv|html|htm|zip|png|jpe?g|json|mp4|mp3))`)

// claimsArtifact 判断回复是否包含「已产出文件」类声明。
func claimsArtifact(content string) bool {
	return content != "" && artifactClaimRe.MatchString(content)
}

// claimedPaths 从声明里抽取所有提到的文件名（含后缀），小写归一去前后缀空白。
func claimedPaths(content string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range artifactPathRe.FindAllStringSubmatch(content, -1) {
		p := strings.ToLower(strings.TrimSpace(m[1]))
		if p != "" {
			out[p] = struct{}{}
		}
	}
	return out
}

// writeToolNames 写文件类工具（兜底：声明里没抽到路径，但整轮有写工具且产生过变更时也算证据）。
var writeToolNames = map[string]struct{}{
	"file_write":      {},
	"file_edit":       {},
	"archive_manager": {},
}

// evidenceForClaim 核对声明与产物清单的覆盖关系：claimed 为空直接通过（调用方已先筛过含声明）；
// 至少一条声明路径在本 run 的 file_changes 命中 → 证据齐备；路径未命中但有写入工具且工作区
// 有变更（声明泛指）→ 接受；其余判为无证据。
func evidenceForClaim(claimed map[string]struct{}, calls []llm.ToolCall, changes []changeEvidence) bool {
	if len(claimed) == 0 {
		return true
	}
	hit := 0
	for p := range claimed {
		for _, c := range changes {
			if !c.Refused && strings.EqualFold(c.Path, p) {
				hit++
				break
			}
		}
	}
	if hit > 0 {
		return true
	}
	if len(changes) == 0 {
		return false
	}
	hasWriteTool := false
	for _, c := range calls {
		if _, ok := writeToolNames[c.Function.Name]; ok {
			hasWriteTool = true
			break
		}
	}
	if !hasWriteTool {
		return false
	}
	// 写工具存在且产物落库（即便路径未在声明里精确匹配）：接受「泛指」证据
	realChange := false
	for _, c := range changes {
		if !c.Refused {
			realChange = true
			break
		}
	}
	return realChange
}

// changeEvidence 完成度核对用的最小变更视图（service 装配：file_changes 行 + refused 标记）。
type changeEvidence struct {
	Path    string
	Refused bool
}

// =====================================================================
// 13. 错误与执行平面
// =====================================================================

// failRun 错误落库：标 failed + 发 chat:error。
func (s *ChatService) failRun(ctx context.Context, runID, sessionID, assistantMsgID string, err error) {
	s.finishExec(runID, agent.StateFailed)
	now := time.Now().UnixMilli()
	_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
		"status":      domain.MessageStatusFailed,
		"stop_reason": domain.StopReasonError,
		"updated_at":  now,
	})
	s.emit(runID, sessionID, "chat:error", domain.ChatErrorEvent{Code: 5000, Message: err.Error()})
	// 与正常收尾共用 domain.ChatDoneEvent 形状：前端只认一种终态载荷。
	s.emit(runID, sessionID, "chat:done", domain.ChatDoneEvent{
		Status:    "failed",
		Reason:    string(domain.StopReasonError),
		MessageID: assistantMsgID,
	})
}

// finishExec 执行平面终态收束（registry 未注入时空操作）。
func (s *ChatService) finishExec(runID string, state agent.ExecutionState) {
	if s.execs != nil {
		s.execs.Finish(runID, state)
	}
}

// execState run 终止原因 → 执行平面终态：取消优先，错误落 failed，其余按完成收束。
func execState(reason string) agent.ExecutionState {
	switch reason {
	case agent.ReasonCancelled:
		return agent.StateCancelled
	case agent.ReasonError:
		return agent.StateFailed
	default:
		return agent.StateCompleted
	}
}

// =====================================================================
// 14. 策略门常量
// =====================================================================

// gateInternalAllowTools 工具策略门显式放行清单（default 模式下本会走 ask 的那些）。
// 放行不等于无护栏：实现 RiskClassifier 的工具仍由 runner 做命令级裁决。
var gateInternalAllowTools = []string{
	"exec", "run_skill_script", "delegate_task", // 命令级裁决走 RiskClassifier
	"websearch", "webfetch", "http", // 信息型网络读取
	"knowledge_search", "doc_reader", "todo", // 只读 / 会话内计划
	"file_write", "archive_manager", "memory_write", // 工作区沙箱内的本地写
}

// extractTargetDir 从工具入参抽取目标目录。
//
// 返回 (dir, ok)；ok=false 表示该工具与目录无关，调用方应跳过信任检查。
func extractTargetDir(toolName string, args json.RawMessage) (string, bool) {
	switch toolName {
	case "exec":
		// cwd 优先；为空 → 走进程默认目录（app home），跳过信任检查
		var v struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(args, &v); err != nil {
			return "", true
		}
		return v.Cwd, true
	case "file_read", "file_write", "file_list", "doc_reader":
		// 这些工具已被工作区根沙箱约束，目录信任在工具内部处理
		return "", false
	case "archive_manager":
		// archive_manager 的 source/target 是工作区相对路径，不涉及外部目录
		return "", false
	default:
		return "", false
	}
}

// todoToolName 计划工具名；其结构化产出单独发 chat:todo 事件（前端进度卡）。
const todoToolName = "todo"

// =====================================================================
// 15. 事件映射（agent.Event → chat:* / 消息块 / tool 消息）
// =====================================================================

// coreEventMapper 一次 run 的事件映射器。
type coreEventMapper struct {
	svc            *ChatService
	ctx            context.Context
	ses            *domain.ChatSessionDO
	runID          string
	assistantMsgID string
	runState       *capability.RunState // 能力装配态：技能命中详情在 RunStart 时落事件/块
	runStart       time.Time

	blockSeq  int64          // message_blocks 单调序号（同一事件对应同一 seq）
	toolCalls []llm.ToolCall // 父 run 收集的工具调用（run 结束写 assistant.tool_calls）
	doneEvent any            // 终态事件载荷（domain.ChatDoneEvent）：延迟到 assistant 落库后由调用方发出

	// textSeg 自上次落块以来累积的正文增量：在工具调用之前与轮次/run 结束时落块，
	// 使块的 seq 与模型真实输出顺序一致（叙述 → 工具 → 叙述）。
	textSeg strings.Builder
}

// newCoreEventMapper 构造一次 run 的事件映射器。
func newCoreEventMapper(svc *ChatService, ctx context.Context, ses *domain.ChatSessionDO,
	runID, assistantMsgID string, runState *capability.RunState) *coreEventMapper {
	return &coreEventMapper{
		svc: svc, ctx: ctx, ses: ses, runID: runID, assistantMsgID: assistantMsgID,
		runState: runState, runStart: time.Now(),
	}
}

// persistBlock 过程块落库（thinking / tool_call / tool_result / artifact / skill），
// 仅父 run 落块；刷新后历史消息据此完整回放执行过程。
func (m *coreEventMapper) persistBlock(e agent.Event, kind domain.MessageBlockKind, payload map[string]any) {
	if m.svc.blocks == nil || m.assistantMsgID == "" || e.RunID != m.runID {
		return
	}
	m.blockSeq++
	bs, err := json.Marshal(payload)
	if err != nil {
		return
	}
	row := domain.MessageBlockDO{
		ID:        pkg.NewID(domain.IDMessageBlock),
		MessageID: m.assistantMsgID,
		SessionID: m.ses.ID,
		Seq:       m.blockSeq,
		Kind:      kind,
		Payload:   string(bs),
	}
	if err := m.svc.blocks.Create(m.ctx, &row); err != nil {
		pkg.L.Warn("persist message block failed", "runID", m.runID, "err", err.Error())
	}
}

// flushText 把累积的正文片段落成一个 text 块（空/纯空白不落）。
// 落块时机 = 工具调用之前 与 轮次/run 结束：这样块序列里的正文与工具调用
// 就保持了模型真实的输出顺序，而不是「过程全在前、正文全在后」。
func (m *coreEventMapper) flushText(e agent.Event) {
	if m.textSeg.Len() == 0 {
		return
	}
	text := m.textSeg.String()
	m.textSeg.Reset()
	if strings.TrimSpace(text) == "" {
		return
	}
	m.persistBlock(e, domain.BlockText, map[string]any{"text": text})
}

// toolResultContentOf 组装落库的 tool 消息正文：错误信息并入正文，
// 下次 run 重建上下文时模型能看到「上次为什么失败」。
func toolResultContentOf(p agent.ToolResultPayload) string {
	if p.Err != "" {
		return "error: " + p.Err + "\n" + p.Content
	}
	return p.Content
}

// handle 实现 agent.Sink：单事件映射入口。
//
// 子 Agent 委派的生命周期事件走独立 chat:subagent-* 通道（复用父 run 的 chat:done 会让
// 前端提前关闭 SSE）；子工具事件推 chat:tool*（带 agent 标签），但不写父 run 的块与 tool 历史。
func (m *coreEventMapper) handle(e agent.Event) {
	s := m.svc
	runID, ses := m.runID, m.ses
	isChild := e.RunID != m.runID
	switch e.Kind {
	case agent.EventRunStart:
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-start", domain.ChatSubagentStartEvent{SubRunID: e.RunID, Agent: e.Agent})
			return
		}
		// 技能命中先于首帧正文：时间线叙事顺序为「技能命中 → 工具 → 回答」
		if st := m.runState; st != nil && st.SkillName != "" {
			s.emit(runID, ses.ID, "chat:skill", skillBlockPayload(st))
			m.persistBlock(e, domain.BlockSkill, skillBlockPayload(st))
		}
		s.emit(runID, ses.ID, "chat:stream.start", map[string]any{"model": ses.Model})
	case agent.EventTurnStart:
		// 长任务的轮次推进需要用户可见：前端据此标注「第 N 轮」并按轮分段时间线。
		if isChild {
			return
		}
		s.emit(runID, ses.ID, "chat:turn-start", map[string]any{"turn": e.Turn})
	case agent.EventCheckpoint:
		// 检查点位点：崩溃/中断后能续跑到哪，续跑提示据此说明从哪一轮接着做。
		if isChild {
			return
		}
		s.emit(runID, ses.ID, "chat:checkpoint", map[string]any{"turn": e.Turn})
	case agent.EventTurnDelta:
		if isChild {
			return
		}
		if p, ok := e.Payload.(agent.DeltaPayload); ok && p.Kind == "content" {
			s.emit(runID, ses.ID, "chat:stream", map[string]any{"delta": p.Text})
			m.textSeg.WriteString(p.Text)
		}
	case agent.EventTurnThinking:
		if isChild {
			return
		}
		if p, ok := e.Payload.(agent.DeltaPayload); ok && p.Kind == "thinking" {
			s.emit(runID, ses.ID, "chat:thinking", map[string]any{"delta": p.Text})
		}
	case agent.EventTurnEnd:
		if isChild {
			return
		}
		// 轮次结束推本轮用量：前端 streamingStats 实时驱动上下文进度与消息用量行。
		if p, ok := e.Payload.(agent.TurnEndPayload); ok {
			s.emit(runID, ses.ID, "chat:stats", domain.ChatStatsEvent{
				Turn:                e.Turn,
				InputTokens:         p.Usage.Input,
				OutputTokens:        p.Usage.Output,
				CacheReadTokens:     p.Usage.CacheRead,
				CacheCreationTokens: p.Usage.CacheWrite,
				TotalTokens:         p.Usage.Total,
				LatencyMs:           p.LatencyMs,
			})
		}
		// 本轮叙述收尾：这一轮若以正文结束（没有后续工具调用），在这里落块。
		m.flushText(e)
	case agent.EventToolCall:
		if p, ok := e.Payload.(agent.ToolCallPayload); ok {
			if !isChild {
				// 先落正文再落工具调用：块的 seq 顺序即用户看到的执行顺序。
				m.flushText(e)
				m.toolCalls = append(m.toolCalls, llm.ToolCall{
					ID:   p.ID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      p.Name,
						Arguments: p.Arguments,
					},
				})
			}
			s.emit(runID, ses.ID, "chat:tool", domain.ChatToolCallEvent{
				ID: p.ID, Name: p.Name, Arguments: p.Arguments,
				Activity: p.Activity, Agent: e.Agent,
			})
			if !isChild {
				m.persistBlock(e, domain.BlockToolCall, map[string]any{
					"id": p.ID, "name": p.Name, "arguments": p.Arguments, "activity": p.Activity,
				})
			}
		}
	case agent.EventToolStart:
		if _, ok := e.Payload.(agent.ToolCallPayload); ok {
			s.emit(runID, ses.ID, "chat:tool-start", map[string]any{"agent": e.Agent, "turn": e.Turn})
		}
	case agent.EventToolResult:
		p, ok := e.Payload.(agent.ToolResultPayload)
		if !ok {
			return
		}
		s.emit(runID, ses.ID, "chat:tool-result", domain.ChatToolResultEvent{
			ID: p.ToolCallID, Name: p.Name,
			Content: p.Content, Error: p.Err, DurationMs: p.DurationMs,
			Agent: e.Agent, UIHint: p.UIHint, Data: p.Data,
			Refused: p.Refused, RefusedReason: p.RefusedReason,
			Meta: p.Meta,
		})
		if isChild {
			return
		}
		// 计划快照独立成事件：前端进度卡直接消费，无需解析工具文本。
		if p.Name == todoToolName {
			if st, ok := p.Data["session_todo"].(domain.TodoStateRESP); ok {
				s.emit(runID, ses.ID, "chat:todo", map[string]any{"state": st})
			}
		}
		// 结果 + 产物落块：审批拒绝同样落块，历史可复现完整过程。
		// meta 透传：cwd / same_failure_count / adaptive_hint 等元数据落块，
		// 历史消息刷新后仍能展示「这条命令落在哪个目录」「失败改道提示是否生效」。
		m.persistBlock(e, domain.BlockToolResult, map[string]any{
			"tool_call_id": p.ToolCallID, "name": p.Name,
			"content": p.Content, "error": p.Err,
			"duration_ms": p.DurationMs, "refused": p.Refused,
			"ui_hint": p.UIHint, "data": p.Data,
			"meta": p.Meta,
		})
		if len(p.Data) > 0 {
			m.persistBlock(e, domain.BlockArtifact, map[string]any{"name": p.Name, "data": p.Data})
		}
		// 工具结果落库（role=tool），供后续轮次/下次 run 重建上下文。
		// 序号统一走分配器：与插话注入消息共享水位，避免撞号。
		seq, serr := s.allocSeq(m.ctx, ses.ID, 1)
		if serr != nil {
			pkg.L.Warn("alloc tool message seq failed", "err", serr)
		}
		now := time.Now().UnixMilli()
		toolMsg := &domain.MessageDO{
			ID:         pkg.NewID(domain.IDMessage),
			SessionID:  ses.ID,
			Seq:        seq,
			RunID:      m.runID,
			Role:       domain.MessageRoleTool,
			Content:    toolResultContentOf(p),
			ToolCallID: p.ToolCallID,
			Status:     domain.MessageStatusCompleted,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		_ = s.messages.Insert(m.ctx, toolMsg)
	case agent.EventCompressed:
		// 压缩证据：用户只会看到「前面的聊天不见了」，不提示就等于静默吞上下文。
		p, ok := e.Payload.(agent.CompressedPayload)
		if !ok || isChild {
			return
		}
		pkg.L.Info("context compressed", "runID", runID, "removed", p.Removed, "truncated", p.Truncated)
		s.persistCompressBoundary(m.ctx, ses, p)
		s.emit(runID, ses.ID, "chat:compressed", domain.ChatCompressedEvent{
			RemovedMessages: p.Removed,
			Summary:         p.Summary,
			Truncated:       p.Truncated,
			// 压缩器标识：内核只保留确定性折叠一种，供前端展示折叠来源。
			FilterKey: compressFilterMicro,
			CutoffAt:  p.CutoffAt,
		})
	case agent.EventError:
		p, ok := e.Payload.(agent.ErrorPayload)
		if !ok {
			return
		}
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-error", domain.ChatSubagentErrorEvent{
				SubRunID: e.RunID, Agent: e.Agent, Message: p.Message,
			})
			return
		}
		s.emit(runID, ses.ID, "chat:error", domain.ChatErrorEvent{Code: p.Code, Message: p.Message})
	case agent.EventRetry:
		// 建流瞬时错误退避重试：用户可见进度，模型不可见。
		if isChild {
			return
		}
		if p, ok := e.Payload.(agent.RetryPayload); ok {
			s.emit(runID, ses.ID, "chat:retry", domain.ChatRetryEvent{
				Attempt: p.Attempt, DelayMs: p.DelayMs,
			})
		}
	case agent.EventQueueDrained:
		// 队列取出可见性：前端展示「用户消息已入队并被消费」
		if isChild {
			return
		}
		if p, ok := e.Payload.(agent.QueueDrainedPayload); ok {
			s.emit(runID, ses.ID, "chat:queue-drained", domain.ChatQueueDrainedEvent{
				Queue: p.Queue, Count: p.Count, Mode: p.Mode, Turn: p.Turn,
			})
		}
	case agent.EventRunDone:
		p, ok := e.Payload.(agent.RunDonePayload)
		if !ok {
			return
		}
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-done", domain.ChatSubagentDoneEvent{
				SubRunID: e.RunID, Agent: e.Agent,
				Reason: string(domain.MapHarnessReason(p.Reason)),
			})
			return
		}
		// 收尾前把最后一段正文落块（正常路径已在 TurnEnd 落过，这里是兜底）。
		m.flushText(e)
		m.doneEvent = domain.ChatDoneEvent{
			Status:     "completed",
			Reason:     string(domain.MapHarnessReason(p.Reason)),
			StopReason: p.StopReason,
			MessageID:  m.assistantMsgID,
			Usage: &domain.ChatDoneUsage{
				InputTokens:  p.Usage.Input,
				OutputTokens: p.Usage.Output,
				CacheRead:    p.Usage.CacheRead,
				CacheWrite:   p.Usage.CacheWrite,
				Total:        p.Usage.Total,
			},
		}
	}
}

// =====================================================================
// 16. emit（service → SSE 唯一出口）
// =====================================================================

// emit 发布 run 事件：统一走 Emitter（注入归属、分配 seq、广播）。
// 载荷支持 map[string]any（高频增量）或领域事件结构体（如 domain.ChatDoneEvent）；
// 字段一律 snake_case（AGENTS.md）。
func (s *ChatService) emit(runID, sessionID, name string, payload any) {
	s.emitter.Emit(runID, sessionID, name, payload)
}