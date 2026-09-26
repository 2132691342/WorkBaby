package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
)

// 后台任务域：用户显式提交的持久化异步 Agent 作业。与聊天 run 的区别是它不是
// 「发消息」的同步动作：提交即返回、后台跑完出结果、跨重启可见。执行复用 agent.Loop
// 与同一条护栏链，差异只在审批门（无人值守，只吃免审授权）。

// 后台任务预算：与委派不同，后台任务是用户有预期的长作业，给足墙钟。
const (
	taskQueueCap     = 64               // 队列容量；满则拒绝提交（背压直给用户）
	taskWorkerCount  = 2                // 并发 worker 数：一个跑长任务时另一个还能接短任务
	taskWallTime     = 30 * time.Minute // 单任务墙钟上限
	taskToolTimeout  = 5 * time.Minute  // 单次工具超时（对齐聊天执行器默认）
	taskDefaultTurns = 40               // 任务默认轮次上限（Definition.Budget 可覆盖）
	taskListLimit    = 50
)

// ChatTaskService 后台任务编排：队列 + worker 池 + 持久化 + task:* 事件。
type ChatTaskService struct {
	repo    *repo.ChatTaskRepo
	bus     *event.Bus
	emitter *Emitter // 事件出口：复用 ChatService 的实例，task:* 同样带 seq 可重放
	chat    *ChatService
	grants  *ApprovalService
	session *repo.ChatSessionRepo

	queue     chan string // 待执行任务 ID
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc // taskID → 取消函数（running 态才有）
}

// NewChatTaskService 构造并启动 worker 池；启动恢复：未终态任务标记失败（进程重启中断）。
func NewChatTaskService(r *repo.ChatTaskRepo, bus *event.Bus, chat *ChatService, grants *ApprovalService, sessions *repo.ChatSessionRepo) *ChatTaskService {
	s := &ChatTaskService{
		repo: r, bus: bus, chat: chat, grants: grants, session: sessions,
		queue:   make(chan string, taskQueueCap),
		cancels: map[string]context.CancelFunc{},
	}
	if chat != nil {
		s.emitter = chat.emitter
	}
	if s.emitter == nil {
		s.emitter = NewEmitter(bus, nil)
	}
	s.recoverUnfinished()
	for i := 0; i < taskWorkerCount; i++ {
		go s.worker()
	}
	return s
}

// recoverUnfinished 启动恢复：进程重启时 running/pending 都不可能还在跑，统一标记失败。
func (s *ChatTaskService) recoverUnfinished() {
	ctx := context.Background()
	rows, err := s.repo.ListUnfinished(ctx)
	if err != nil {
		pkg.L.Warn("task recovery list failed", "err", err.Error())
		return
	}
	for _, t := range rows {
		if err := s.repo.UpdateFields(ctx, t.ID, map[string]any{
			"state": domain.TaskStateFailed, "error": "应用重启，任务中断", "finished_at": time.Now().UnixMilli(),
		}); err != nil {
			pkg.L.Warn("task recovery mark failed", "task", t.ID, "err", err.Error())
		}
	}
	if len(rows) > 0 {
		pkg.L.Info("recovered unfinished tasks", "count", len(rows))
	}
}

// worker 消费队列：取任务 ID → 载入 → 执行。
func (s *ChatTaskService) worker() {
	for id := range s.queue {
		s.runOne(id)
	}
}

// Submit 提交后台任务：立即落库（pending）→ 入队 → 发 task:created。不阻塞调用方。
func (s *ChatTaskService) Submit(ctx context.Context, sessionID, agentName, prompt string) (*domain.ChatTaskDO, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, pkg.New(5025, "任务描述不能为空", "")
	}
	if sessionID == "" {
		return nil, pkg.New(5025, "任务必须绑定一个宿主会话（工作区与模型来源）", "")
	}
	if agentName == "" {
		agentName = agent.AgentDefault
	}
	def := agent.Agent(agentName)

	row := &domain.ChatTaskDO{
		ID: pkg.NewID("TASK"), SessionID: sessionID, Agent: def.Name, Prompt: prompt, State: domain.TaskStatePending,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	s.emit("task:created", row)
	select {
	case s.queue <- row.ID:
	default:
		// 背压：队列满直接失败并回写终态，任务不静默滞留。
		_ = s.repo.UpdateFields(ctx, row.ID, map[string]any{
			"state": domain.TaskStateFailed, "error": "任务队列已满", "finished_at": time.Now().UnixMilli(),
		})
		fresh, _ := s.repo.GetByID(ctx, row.ID)
		if fresh != nil {
			s.emit("task:done", fresh)
		}
		return nil, pkg.New(5025, "任务队列已满，请稍后再试", "")
	}
	return row, nil
}

// Cancel 取消任务：pending 直接标记；running 经 ctx 取消，由执行流程落终态。
func (s *ChatTaskService) Cancel(ctx context.Context, id string) error {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if domain.TaskTerminal(row.State) {
		return nil // 幂等：已终态无事发生
	}
	if row.State == domain.TaskStatePending {
		if err := s.repo.UpdateFields(ctx, id, map[string]any{
			"state": domain.TaskStateCancelled, "error": "提交后取消", "finished_at": time.Now().UnixMilli(),
		}); err != nil {
			return err
		}
		fresh, _ := s.repo.GetByID(ctx, id)
		if fresh != nil {
			s.emit("task:done", fresh)
		}
		return nil
	}
	// running：触发 ctx 取消；worker 收尾时按 cancelled 落库发事件
	s.cancelsMu.Lock()
	cancel, ok := s.cancels[id]
	s.cancelsMu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

// List 任务中心列表（最新在前）。
func (s *ChatTaskService) List(ctx context.Context, limit int) (*domain.ChatTaskListRESP, error) {
	if limit <= 0 {
		limit = taskListLimit
	}
	rows, total, err := s.repo.ListRecent(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &domain.ChatTaskListRESP{Items: rows, Total: int(total)}, nil
}

// runOne 执行单个任务：pending → running → 终态，全程落库 + 发事件。
func (s *ChatTaskService) runOne(id string) {
	ctx := context.Background()
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		pkg.L.Warn("task load failed", "task", id, "err", err.Error())
		return
	}
	// 队列里等到执行时可能已被取消（pending → cancelled）：终态直接跳过。
	if domain.TaskTerminal(row.State) {
		return
	}

	ses, err := s.session.GetByID(ctx, row.SessionID)
	if err != nil || ses == nil {
		s.finishFailed(ctx, row.ID, "宿主会话不可用（可能已删除）")
		return
	}

	now := time.Now().UnixMilli()
	runID := pkg.NewID("RUN")
	if err := s.repo.UpdateFields(ctx, id, map[string]any{
		"state": domain.TaskStateRunning, "run_id": runID, "started_at": now,
	}); err != nil {
		pkg.L.Warn("task mark running failed", "task", id, "err", err.Error())
		return
	}
	row, _ = s.repo.GetByID(ctx, id)
	if row != nil {
		s.emit("task:started", row)
	}
	if s.chat.execs != nil {
		s.chat.execs.Register(&agent.ExecutionRun{
			RunID: runID, SessionID: ses.ID, AgentName: row.Agent, Scope: agent.ScopeTask, State: agent.StateRunning,
		})
	}

	// 可取消 ctx：Cancel 写 cancels 表 → 这里收到取消 → loop 返回 → 落 cancelled。
	runCtx, cancel := context.WithTimeout(agent.WithRunContext(ctx, runID, ses.ID), taskWallTime)
	s.cancelsMu.Lock()
	s.cancels[id] = cancel
	s.cancelsMu.Unlock()
	defer func() {
		s.cancelsMu.Lock()
		delete(s.cancels, id)
		s.cancelsMu.Unlock()
		cancel()
	}()

	outcome := s.execute(runCtx, ses, row, runID)
	s.settle(ctx, id, runID, outcome)
}

// taskOutcome 执行结果归一。
type taskOutcome struct {
	state  string
	result string
	err    string
}

// execute 装配并运行任务 Loop；返回归一结果（不落库）。
func (s *ChatTaskService) execute(ctx context.Context, ses *domain.ChatSessionDO, row *domain.ChatTaskDO, runID string) taskOutcome {
	def := agent.Agent(row.Agent)
	prov, err := s.chat.reg.Get(ses.ProviderID)
	if err != nil {
		return taskOutcome{state: domain.TaskStateFailed, err: "Provider 不可用: " + err.Error()}
	}

	// 工具暴露：全注册表 ∩ Agent 策略（与聊天 run 同源，受同一护栏链约束）。
	all := s.chat.tools.Registry().List()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name())
	}
	defs := make([]llm.ToolDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, llm.ToolDefinition{Name: n})
	}
	toolNames := make([]string, 0)
	for _, d := range def.FilterTools(defs) {
		toolNames = append(toolNames, d.Name)
	}

	// 轮次预算：Definition 未声明时取任务默认值（经 Budget.Apply 进入 Loop 配置）。
	if def.Budget.MaxTurns <= 0 {
		def.Budget.MaxTurns = taskDefaultTurns
	}

	// 无人值守：审批门换成 grantApprover（只吃免审授权，不弹窗）。
	model := def.EffectiveModel(ses.Model)
	loop := s.chat.newCoreLoop(coreLoopSpec{
		Ses:         ses,
		RunID:       runID,
		Def:         def,
		Provider:    prov,
		Model:       model,
		System:      def.Persona,
		ToolNames:   toolNames,
		Sink:        agent.NopSink{},
		ExecTimeout: taskToolTimeout,
		Approver:    grantApprover{svc: s.grants},
	})

	out, runErr := loop.Run(ctx, []*llm.Message{llm.UserMessage(row.Prompt)})
	if runErr == nil && s.chat.usages != nil {
		// 任务消耗单独归因（UsageSourceTask），仪表盘可按场景拆分。
		bgCtx := context.Background()
		for _, t := range out.PerTurn {
			s.chat.persistUsageRowFrom(bgCtx, ses, runID, "", -1, domain.UsageSourceTask, def.Name, model, t.Usage)
		}
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return taskOutcome{state: domain.TaskStateCancelled, err: "用户取消"}
		}
		return taskOutcome{state: domain.TaskStateFailed, err: runErr.Error()}
	}
	switch out.Reason {
	case agent.ReasonCancelled:
		return taskOutcome{state: domain.TaskStateCancelled, err: "用户取消"}
	case agent.ReasonMaxTurns, agent.ReasonBudget:
		// 预算耗尽不算失败：产出的部分结论仍是结果，附说明。
		return taskOutcome{state: domain.TaskStateCompleted,
			result: pkg.TruncateRunes(out.Content, taskResultMax) + "\n\n（已达轮次/预算上限，任务提前收束）"}
	case agent.ReasonError:
		msg := ""
		if out.Err != nil {
			msg = out.Err.Error()
		}
		return taskOutcome{state: domain.TaskStateFailed, err: msg}
	}
	return taskOutcome{state: domain.TaskStateCompleted, result: pkg.TruncateRunes(out.Content, taskResultMax)}
}

// settle 终态落库 + 发事件 + 用量落账 + 执行平面收尾。
func (s *ChatTaskService) settle(ctx context.Context, id, runID string, out taskOutcome) {
	fields := map[string]any{"state": string(out.state), "finished_at": time.Now().UnixMilli()}
	if out.err != "" {
		fields["error"] = out.err
	}
	if out.result != "" {
		fields["result"] = out.result
	}
	if err := s.repo.UpdateFields(ctx, id, fields); err != nil {
		pkg.L.Warn("task settle failed", "task", id, "err", err.Error())
	}
	if row, err := s.repo.GetByID(ctx, id); err == nil {
		s.emit("task:done", row)
	}
	if s.chat.execs != nil {
		state := agent.StateCompleted
		if out.state == domain.TaskStateFailed {
			state = agent.StateFailed
		} else if out.state == domain.TaskStateCancelled {
			state = agent.StateCancelled
		}
		s.chat.execs.Finish(runID, state)
	}
}

// finishFailed 快速失败路径（会话缺失等，无 run 可言）。
func (s *ChatTaskService) finishFailed(ctx context.Context, id, reason string) {
	now := time.Now().UnixMilli()
	if err := s.repo.UpdateFields(ctx, id, map[string]any{
		"state": domain.TaskStateFailed, "error": reason, "finished_at": now,
	}); err != nil {
		pkg.L.Warn("task finishFailed failed", "task", id, "err", err.Error())
		return
	}
	if row, err := s.repo.GetByID(ctx, id); err == nil {
		s.emit("task:done", row)
	}
}

// emit 发布 task:* 事件：整条任务 DO 作为载荷（前端整对象 upsert）。
// 归属与 seq 由 Emitter 统一注入——任务事件与 chat:* 走同一套可靠性机制。
func (s *ChatTaskService) emit(name string, t *domain.ChatTaskDO) {
	if t == nil {
		return
	}
	s.emitter.Emit(t.RunID, t.SessionID, name, map[string]any{
		"task":    t,
		"task_id": t.ID,
	})
}

// grantApprover 无人值守审批门：免审授权命中即放行，其余一律拒绝（不弹窗、不等待）。
// 后台任务继承用户在聊天里批准过的命令；未授权的拒绝是可改道回执，模型会换路径或收尾说明。
type grantApprover struct{ svc *ApprovalService }

// Approve 实现 agent.Approver。
func (g grantApprover) Approve(_ context.Context, call agent.Call, risk string) bool {
	if g.svc == nil {
		return false
	}
	if risk == tool.RiskApprovalIrrev {
		return false // 不可逆操作永不免审（与聊天同一铁律）
	}
	return g.svc.HasGrant(approvalCommand(call))
}

// DenyReason 实现 agent.ApprovalReasoner：拒绝回执可解释，模型能据此改道。
func (g grantApprover) DenyReason() string {
	return "后台任务不弹审批窗，该命令未在免审授权中（请先在聊天中批准过，或调高会话权限模式）"
}

// taskResultMax 结果字段截断（rune）：任务结果只是摘要，全文留在会话产物/工作区里。
const taskResultMax = 8000
