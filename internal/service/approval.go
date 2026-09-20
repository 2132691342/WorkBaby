package service

import (
	"context"
	"sync"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
)

// 审批等待用户决策的超时；超时视为拒绝（LLM 收到拒绝结果会改道，run 不会永久挂起）。
const approvalWaitTimeout = 2 * time.Minute

// pendingReopenTTL 重启后未决审批的重新武装窗口：决策等待跨进程存活（durable pause），
// 用户不必赶在重启前决策；过期后视图裁剪，记录留库作审计。
const pendingReopenTTL = 24 * time.Hour

// inputReason 补充输入请求的统一说明文案。
const inputReason = "我需要你补充一点信息才能继续"

// ApprovalService 工具审批门（durable pause）：阻塞等待用户决策，未决请求落 approval_records，
// 重启后 RearmPending 重武装决策窗口，决策经续跑钩子从检查点恢复 run。
// needs_approval 类命令批准过一次后进程内免审（可持久化为 approval_grants），
// irreversible 每次都问；续跑回放的同一调用走 FindDecided 快速裁决。
type ApprovalService struct {
	bus       *event.Bus
	emitter   *Emitter // 事件出口（与 ChatService 共享同一实例，保证 seq 连续）
	records   *repo.ApprovalRecordRepo
	grants    *repo.ApprovalGrantRepo                       // 「本会话允许」持久化（跨重启 + 可撤销 + 可回滚）；nil = 不持久化
	resume    func(ctx context.Context, runID string) error // 续跑钩子（重启后决策触发，durable pause）
	mu        sync.Mutex
	pending   map[string]*pendingApproval
	inputs    map[string]chan inputAnswer
	inflight  map[string]*pendingInput
	approved  map[string]bool                // 免审表：进程内 + 库内授权共同驱动
	runGrants map[string]map[string]struct{} // 本 run 内新增的免审命令（runID → commands）；失败/取消时回滚
}

// 放行有效期：与前端「允许一次 / 本会话允许」两档一一对应。
const (
	scopeOnce    = "once"
	scopeSession = "session"
)

// approvalDecision 一次审批决策：是否放行 + 是否记入本会话免审表。
type approvalDecision struct {
	approved bool
	remember bool
}

// canRemember 只有 needs_approval 允许「本会话免审」。
// 不可逆操作（危险正则命中）每次都问——给一次性放行是授权，给永久放行是隐患。
func canRemember(risk string) bool { return risk == tool.RiskApprovalNeeds }

// pendingApproval 单条未决审批。
type pendingApproval struct {
	ch        chan approvalDecision
	command   string
	risk      string
	runID     string
	sessionID string
	createdAt int64
	expiresAt int64
}

// pendingInput 单条未决补充输入（Pending 快照用）。
type pendingInput struct {
	question  string
	runID     string
	sessionID string
	expiresAt int64
}

// inputAnswer 补充输入的回填值；ok=false 表示用户跳过。
type inputAnswer struct {
	text string
	ok   bool
}

// NewApprovalService 构造审批服务。
func NewApprovalService(bus *event.Bus) *ApprovalService {
	return &ApprovalService{
		bus:       bus,
		emitter:   NewEmitter(bus, nil), // 默认出口可用；装配方用 WithEmitter 升级为带重放的共享实例
		pending:   map[string]*pendingApproval{},
		inputs:    map[string]chan inputAnswer{},
		inflight:  map[string]*pendingInput{},
		approved:  map[string]bool{},
		runGrants: map[string]map[string]struct{}{},
	}
}

// WithEmitter 注入事件出口：审批事件与 chat 事件共享同一序号空间，断线重放不缺帧。
func (s *ApprovalService) WithEmitter(e *Emitter) *ApprovalService {
	s.emitter = e
	return s
}

// WithRecords 启用审批持久化（暂停态跨重启可见/可决策）。
func (s *ApprovalService) WithRecords(r *repo.ApprovalRecordRepo) *ApprovalService {
	s.records = r
	return s
}

// WithResumeHook 注入续跑钩子（ChatService.ResumeRun）：重启后决策 pending 记录
// 即从检查点续跑该 run；未注入时记录只落终态、不自动续跑。
func (s *ApprovalService) WithResumeHook(fn func(ctx context.Context, runID string) error) *ApprovalService {
	s.resume = fn
	return s
}

// WithGrants 启用免审授权持久化：「本会话允许」升级为跨重启生效（可撤销、可回滚）。
func (s *ApprovalService) WithGrants(r *repo.ApprovalGrantRepo) *ApprovalService {
	s.grants = r
	return s
}

// LoadGrants 启动时装载库内免审授权（与进程内免审表合并）。
func (s *ApprovalService) LoadGrants(ctx context.Context) {
	if s.grants == nil {
		return
	}
	rows, err := s.grants.List(ctx)
	if err != nil {
		pkg.L.Warn("load approval grants failed", "err", err.Error())
		return
	}
	s.mu.Lock()
	for i := range rows {
		s.approved[rows[i].Command] = true
	}
	s.mu.Unlock()
	if len(rows) > 0 {
		pkg.L.Info("loaded approval grants", "count", len(rows))
	}
}

// rememberGrant 记录「本会话允许」：免审表 + 库内持久化 + 按 run 记账（供失败回滚）。
func (s *ApprovalService) rememberGrant(ctx context.Context, runID, command, risk string) {
	s.mu.Lock()
	s.approved[command] = true
	if runID != "" {
		set, ok := s.runGrants[runID]
		if !ok {
			set = map[string]struct{}{}
			s.runGrants[runID] = set
		}
		set[command] = struct{}{}
	}
	s.mu.Unlock()
	if s.grants == nil {
		return
	}
	row := &domain.ApprovalGrantDO{ID: pkg.NewID("AGR"), Command: command, Risk: risk}
	if err := s.grants.Create(ctx, row); err != nil {
		pkg.L.Warn("persist approval grant failed", "command", command, "err", err.Error())
	}
}

// RollbackRun 回滚本 run 内新增的免审授权（run 以 error/cancelled 收尾时调用）：
// 未验证的工作不留下扩权——授权跟随产生它的 run 的成败。
func (s *ApprovalService) RollbackRun(runID string) {
	s.mu.Lock()
	set, ok := s.runGrants[runID]
	if !ok {
		s.mu.Unlock()
		return
	}
	commands := make([]string, 0, len(set))
	for c := range set {
		commands = append(commands, c)
		delete(s.approved, c)
	}
	delete(s.runGrants, runID)
	s.mu.Unlock()
	if s.grants == nil {
		return
	}
	ctx := context.Background()
	for _, c := range commands {
		if err := s.grants.DeleteByCommand(ctx, c); err != nil {
			pkg.L.Warn("rollback approval grant failed", "command", c, "err", err.Error())
		}
	}
	if len(commands) > 0 {
		pkg.L.Info("rolled back run-scoped approval grants", "runID", runID, "count", len(commands))
	}
}

// ListGrants 免审授权列表（设置页撤销界面）。
func (s *ApprovalService) ListGrants(ctx context.Context) []domain.ApprovalGrantRESP {
	if s.grants == nil {
		return []domain.ApprovalGrantRESP{}
	}
	rows, err := s.grants.List(ctx)
	if err != nil {
		return []domain.ApprovalGrantRESP{}
	}
	out := make([]domain.ApprovalGrantRESP, 0, len(rows))
	for i := range rows {
		out = append(out, domain.ApprovalGrantRESP{
			ID: rows[i].ID, Command: rows[i].Command, Risk: rows[i].Risk, CreatedAt: rows[i].CreatedAt,
		})
	}
	return out
}

// RevokeGrant 撤销单条免审授权：库内删除 + 进程内免审表同步摘除。
func (s *ApprovalService) RevokeGrant(ctx context.Context, id string) error {
	if s.grants == nil {
		return pkg.New(4004, "免审授权持久化未启用", "")
	}
	rows, err := s.grants.List(ctx)
	if err != nil {
		return err
	}
	var command string
	for i := range rows {
		if rows[i].ID == id {
			command = rows[i].Command
			break
		}
	}
	if command == "" {
		return pkg.New(4003, "免审授权不存在", id)
	}
	if err := s.grants.Delete(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.approved, command)
	s.mu.Unlock()
	return nil
}

// HasGrant 报告某命令是否已获免审授权。后台任务等无人值守门（grantApprover）
// 的唯一授权来源：不弹窗、不等待，只认用户在聊天中明确批准过的命令。
func (s *ApprovalService) HasGrant(command string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.approved[command]
}

// emit 发布审批事件（归属经 ctx 提取，统一走 Emitter）。
func (s *ApprovalService) emit(ctx context.Context, name string, payload map[string]any) {
	s.emitter.EmitCtx(ctx, name, payload)
}

// Approve 实现 tool.Approver：阻塞等待用户决策（批准 true / 拒绝、超时、取消 false）。
// 续跑恢复路径：库内有本 run 同命令的已决记录（approved/denied 未消费）→ 直接消费放行/拒绝，
// 不再二次询问——重启前用户已决策过，重放的同一调用问两遍是骚扰。
func (s *ApprovalService) Approve(ctx context.Context, command string, risk string) bool {
	if risk == tool.RiskApprovalNeeds {
		s.mu.Lock()
		ok := s.approved[command]
		s.mu.Unlock()
		if ok {
			return true // 免审：同命令已批准过
		}
	}
	// 已决记录快速裁决仅限续跑回放（IsResumedRun）：正常路径下同命令的第二次调用
	// 必须重新询问，一次性审批不得被残留记录静默复用。
	if core.IsResumedRun(ctx) {
		if ok, found := s.consumeDecided(ctx, domain.ApprovalKindApproval, command); found {
			return ok
		}
	}

	id := pkg.NewID("APR")
	now := time.Now()
	item := &pendingApproval{
		ch:        make(chan approvalDecision, 1),
		command:   command,
		risk:      risk,
		runID:     core.RunIDFromCtx(ctx),
		sessionID: core.SessionIDFromCtx(ctx),
		createdAt: now.UnixMilli(),
		expiresAt: now.Add(approvalWaitTimeout).UnixMilli(),
	}
	s.mu.Lock()
	s.pending[id] = item
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()
	s.persistPending(ctx, id, domain.ApprovalKindApproval, command, risk, item.expiresAt)

	s.emit(ctx, "chat:approval", map[string]any{
		"id":           id,
		"command":      command,
		"reason":       approvalReason(risk),
		"risk":         risk,
		"can_remember": canRemember(risk),
	})

	select {
	case d := <-item.ch:
		// 「本会话允许」由用户显式选择（remember）；irreversible 永不免审。
		// 授权落库（跨重启）并按 run 记账（run 失败/取消时回滚）。
		if d.approved && d.remember && canRemember(risk) {
			s.rememberGrant(ctx, item.runID, command, risk)
		}
		s.settleRecord(id, map[bool]string{true: domain.ApprovalStatusApproved, false: domain.ApprovalStatusDenied}[d.approved], "")
		s.emitDecided(ctx, id, command, map[bool]string{true: "approved", false: "denied"}[d.approved])
		return d.approved
	case <-time.After(approvalWaitTimeout):
		pkg.L.Warn("approval timed out, deny", "id", id)
		s.settleRecord(id, domain.ApprovalStatusTimeout, "")
		s.emitDecided(ctx, id, command, "timeout")
		return false
	case <-ctx.Done():
		s.settleRecord(id, domain.ApprovalStatusCancelled, "")
		s.emitDecided(ctx, id, command, "cancelled")
		return false // run 被取消/超时，工具栈随之终止
	}
}

// persistPending 未决请求落库（records 未注入时静默跳过）。
func (s *ApprovalService) persistPending(ctx context.Context, id, kind, command, risk string, expiresAt int64) {
	if s.records == nil {
		return
	}
	row := &domain.ApprovalRecordDO{
		ID: id, RunID: core.RunIDFromCtx(ctx), SessionID: core.SessionIDFromCtx(ctx),
		Kind: kind, Command: command, Risk: risk, Status: domain.ApprovalStatusPending, ExpiresAt: expiresAt,
	}
	if err := s.records.Create(ctx, row); err != nil {
		pkg.L.Warn("persist pending approval failed", "id", id, "err", err.Error())
	}
}

// settleRecord 终态更新（忽略未启用持久化）。
func (s *ApprovalService) settleRecord(id, status, answer string) {
	if s.records == nil {
		return
	}
	if err := s.records.SetStatus(context.Background(), id, status, answer); err != nil {
		pkg.L.Warn("settle approval record failed", "id", id, "err", err.Error())
	}
}

// RequestInput 模型主动向用户要信息（risk=input_required）：阻塞等待 Answer 回填文本。
// 返回 (回答, true)；超时/取消返回 ("", false)。
// 已决记录复用仅限续跑回放（IsResumedRun）：库内有本 run 同问题的已决记录
// （answered/skipped 未消费）→ 直接消费复用，一次性问答不被静默复用。
func (s *ApprovalService) RequestInput(ctx context.Context, question string) (string, bool) {
	if core.IsResumedRun(ctx) {
		if rec := s.findDecided(ctx, domain.ApprovalKindInput, question,
			[]string{domain.ApprovalStatusAnswered, domain.ApprovalStatusSkipped}); rec != nil {
			s.settleRecord(rec.ID, domain.ApprovalStatusConsumed, "")
			if rec.Status == domain.ApprovalStatusAnswered {
				return rec.Answer, true
			}
			return "", false // skipped：模型按「用户未回复」自行假设并继续
		}
	}

	id := pkg.NewID("INP")
	now := time.Now()
	ch := make(chan inputAnswer, 1)
	s.mu.Lock()
	s.inputs[id] = ch
	s.inflight[id] = &pendingInput{
		question:  question,
		runID:     core.RunIDFromCtx(ctx),
		sessionID: core.SessionIDFromCtx(ctx),
		expiresAt: now.Add(approvalWaitTimeout).UnixMilli(),
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inputs, id)
		delete(s.inflight, id)
		s.mu.Unlock()
	}()
	s.persistPending(ctx, id, domain.ApprovalKindInput, question, "input_required", now.Add(approvalWaitTimeout).UnixMilli())

	s.emit(ctx, "chat:approval", map[string]any{
		"id":      id,
		"command": question,
		"reason":  inputReason,
		"risk":    tool.RiskApprovalInput,
	})

	select {
	case ans := <-ch:
		if !ans.ok {
			s.settleRecord(id, domain.ApprovalStatusSkipped, "")
			s.emitDecided(ctx, id, question, "skipped")
			return "", false
		}
		s.settleRecord(id, domain.ApprovalStatusAnswered, ans.text)
		s.emitDecided(ctx, id, question, "answered")
		return ans.text, true
	case <-time.After(approvalWaitTimeout):
		s.settleRecord(id, domain.ApprovalStatusTimeout, "")
		s.emitDecided(ctx, id, question, "timeout")
		return "", false
	case <-ctx.Done():
		s.settleRecord(id, domain.ApprovalStatusCancelled, "")
		s.emitDecided(ctx, id, question, "cancelled")
		return "", false
	}
}

// Answer 用户回复补充输入（api 层绑定）。
// 内存无等待者（进程重启后）→ 落库 answered 并触发续跑（durable pause）。
func (s *ApprovalService) Answer(id, text string) error {
	s.mu.Lock()
	ch, ok := s.inputs[id]
	if ok {
		delete(s.inputs, id)
		delete(s.inflight, id)
	}
	s.mu.Unlock()
	if !ok {
		return s.decideFromRecord(context.Background(), id, domain.ApprovalStatusAnswered, text)
	}
	ch <- inputAnswer{text: text, ok: true}
	return nil
}

// RearmPending 启动重武装：重启后无等待 goroutine 的 pending 记录不再作废，
// 延长决策窗口（决策即续跑）；过期记录由视图按 expires_at 裁剪。
func (s *ApprovalService) RearmPending(ctx context.Context) {
	if s.records == nil {
		return
	}
	rows, err := s.records.ListPending(ctx)
	if err != nil {
		pkg.L.Warn("rearm pending approvals list failed", "err", err.Error())
		return
	}
	deadline := time.Now().Add(pendingReopenTTL).UnixMilli()
	for i := range rows {
		if err := s.records.UpdateExpiresAt(ctx, rows[i].ID, deadline); err != nil {
			pkg.L.Warn("rearm pending approval failed", "id", rows[i].ID, "err", err.Error())
		}
	}
	if len(rows) > 0 {
		pkg.L.Info("rearmed pending approvals", "count", len(rows))
	}
}

// decideFromRecord 重启恢复路径：内存无等待者的决策——记录落终态并触发续跑。
// approved/denied/answered/skipped 都续跑：拒绝/跳过后模型在续跑轮拿到 refusal 并改道。
func (s *ApprovalService) decideFromRecord(ctx context.Context, id, status, answer string) error {
	if s.records == nil {
		return pkg.New(4003, "审批请求不存在或已过期", id)
	}
	rec, err := s.records.Get(ctx, id)
	if err != nil {
		return pkg.Wrap(4003, "读取审批记录失败", err)
	}
	if rec == nil || rec.Status != domain.ApprovalStatusPending {
		return pkg.New(4003, "审批请求不存在或已过期", id)
	}
	s.settleRecord(id, status, answer)
	s.emitDecided(core.WithRunContext(ctx, rec.RunID, rec.SessionID), id, rec.Command, decisionName(status))
	s.kickResume(ctx, rec)
	return nil
}

// consumeDecided 护栏链快速裁决：消费本 run 同命令的已决记录（approved→true / denied→false），
// 消费后置 consumed。found=false 表示无记录可消费，走正常询问。
func (s *ApprovalService) consumeDecided(ctx context.Context, kind, command string) (ok, found bool) {
	rec := s.findDecided(ctx, kind, command,
		[]string{domain.ApprovalStatusApproved, domain.ApprovalStatusDenied})
	if rec == nil {
		return false, false
	}
	s.settleRecord(rec.ID, domain.ApprovalStatusConsumed, "")
	return rec.Status == domain.ApprovalStatusApproved, true
}

// findDecided 查未消费的已决记录（records 未启用时恒 nil）。
func (s *ApprovalService) findDecided(ctx context.Context, kind, command string, statuses []string) *domain.ApprovalRecordDO {
	if s.records == nil {
		return nil
	}
	rec, err := s.records.FindDecided(ctx, core.RunIDFromCtx(ctx), kind, command, statuses)
	if err != nil {
		pkg.L.Warn("find decided approval failed", "err", err.Error())
		return nil
	}
	return rec
}

// kickResume 异步触发续跑：决策路径不能阻塞在 run 重启上。
func (s *ApprovalService) kickResume(ctx context.Context, rec *domain.ApprovalRecordDO) {
	if s.resume == nil || rec.RunID == "" {
		return
	}
	go func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.resume(cctx, rec.RunID); err != nil {
			pkg.L.Warn("approval resume failed", "runID", rec.RunID, "err", err.Error())
		}
	}()
}

// decisionName 决策终态 → 前端事件文案。
func decisionName(status string) string {
	switch status {
	case domain.ApprovalStatusApproved:
		return "approved"
	case domain.ApprovalStatusAnswered:
		return "answered"
	case domain.ApprovalStatusSkipped:
		return "skipped"
	default:
		return "denied"
	}
}

// CancelSession 清空同会话遗留挂起（新 run 启动时调用）：旧 run 的审批卡不悬挂到新对话。
// 内存等待者立即按拒绝/未回复醒来（旧 run 的决策对象已不存在）；库内记录统一落 cancelled
// （被唤醒者随后落的 denied/skipped 同为终态，相互覆盖无碍）。
func (s *ApprovalService) CancelSession(ctx context.Context, sessionID string) {
	s.mu.Lock()
	for id, it := range s.pending {
		if it.sessionID == sessionID {
			delete(s.pending, id)
			select {
			case it.ch <- approvalDecision{approved: false}:
			default:
			}
		}
	}
	for id, it := range s.inflight {
		if it.sessionID == sessionID {
			delete(s.inflight, id)
			if ch, ok := s.inputs[id]; ok {
				delete(s.inputs, id)
				close(ch) // 等待者立即醒来并按「未回复」处理
			}
		}
	}
	s.mu.Unlock()
	if s.records == nil {
		return
	}
	rows, err := s.records.ListPendingBySession(ctx, sessionID)
	if err != nil {
		pkg.L.Warn("cancel session approvals list failed", "err", err.Error())
		return
	}
	for i := range rows {
		s.settleRecord(rows[i].ID, domain.ApprovalStatusCancelled, "")
	}
}

// Decide 用户决策回填（api 层 DecideApproval 绑定调用）。
// scope=session 表示「本会话允许」——只对 needs_approval 生效，其余按一次性放行处理。
// 内存无等待者（进程重启后）→ 记录落终态并触发续跑（durable pause）。
func (s *ApprovalService) Decide(id string, approved bool, scope string) error {
	s.mu.Lock()
	item, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if !ok {
		if _, isInput := s.inputs[id]; isInput {
			return pkg.New(4003, "这是补充输入请求，请用 skip 或 answer 处理", id)
		}
		status := domain.ApprovalStatusDenied
		if approved {
			status = domain.ApprovalStatusApproved
		}
		return s.decideFromRecord(context.Background(), id, status, "")
	}
	item.ch <- approvalDecision{approved: approved, remember: scope == scopeSession}
	return nil
}

// Skip 用户跳过：审批按拒绝处理，补充输入按「未回复」处理（模型据此自行假设并继续）。
// 两类请求共用同一入口，前端无需按 risk 分流打不同端点——分流正是 4003 的根因。
// 内存无等待者（进程重启后）→ 记录落终态并触发续跑。
func (s *ApprovalService) Skip(id string) error {
	s.mu.Lock()
	item, isApproval := s.pending[id]
	if isApproval {
		delete(s.pending, id)
	}
	ch, isInput := s.inputs[id]
	if isInput {
		delete(s.inputs, id)
		delete(s.inflight, id)
	}
	s.mu.Unlock()
	switch {
	case isApproval:
		item.ch <- approvalDecision{approved: false}
		return nil
	case isInput:
		ch <- inputAnswer{ok: false}
		return nil
	default:
		if s.records != nil {
			ctx := context.Background()
			if rec, err := s.records.Get(ctx, id); err == nil && rec != nil && rec.Status == domain.ApprovalStatusPending {
				status := domain.ApprovalStatusDenied
				if rec.Kind == domain.ApprovalKindInput {
					status = domain.ApprovalStatusSkipped
				}
				return s.decideFromRecord(ctx, id, status, "")
			}
		}
		return pkg.New(4003, "审批请求不存在或已过期", id)
	}
}

// Pending 返回未决审批快照（供前端刷新/重启后恢复；审批恢复）。
// 内存项之外合并库内 pending 记录（重启遗留，durable pause）——决策卡跨进程存活；
// 顺带清理已过期的审批（Approve 侧超时路径会拒绝，这里只做视图裁剪）。
func (s *ApprovalService) Pending() []domain.ApprovalPendingRESP {
	now := time.Now().UnixMilli()
	s.mu.Lock()
	out := make([]domain.ApprovalPendingRESP, 0, len(s.pending)+len(s.inflight))
	seen := make(map[string]struct{}, len(s.pending)+len(s.inflight))
	for id, it := range s.pending {
		if now >= it.expiresAt {
			delete(s.pending, id)
			continue
		}
		seen[id] = struct{}{}
		out = append(out, domain.ApprovalPendingRESP{
			ID:          id,
			RunID:       it.runID,
			SessionID:   it.sessionID,
			Command:     it.command,
			Reason:      approvalReason(it.risk),
			Risk:        it.risk,
			ExpiresAt:   it.expiresAt,
			CanRemember: canRemember(it.risk),
		})
	}
	// 补充输入与审批同属「暂停等用户」，必须一并回传，否则刷新后提问卡片永久丢失
	for id, it := range s.inflight {
		if now >= it.expiresAt {
			delete(s.inputs, id)
			delete(s.inflight, id)
			continue
		}
		seen[id] = struct{}{}
		out = append(out, domain.ApprovalPendingRESP{
			ID:        id,
			RunID:     it.runID,
			SessionID: it.sessionID,
			Command:   it.question,
			Reason:    inputReason,
			Risk:      tool.RiskApprovalInput,
			ExpiresAt: it.expiresAt,
		})
	}
	s.mu.Unlock()
	// 重启遗留：库内 pending（含已重武装窗口）合并进视图，内存已有的按 id 去重
	if s.records != nil {
		rows, err := s.records.ListPending(context.Background())
		if err != nil {
			pkg.L.Warn("list pending approvals for view failed", "err", err.Error())
		}
		for i := range rows {
			r := &rows[i]
			if now >= r.ExpiresAt {
				continue
			}
			if _, dup := seen[r.ID]; dup {
				continue
			}
			reason, risk := approvalReason(r.Risk), r.Risk
			if r.Kind == domain.ApprovalKindInput {
				reason, risk = inputReason, tool.RiskApprovalInput
			}
			out = append(out, domain.ApprovalPendingRESP{
				ID:          r.ID,
				RunID:       r.RunID,
				SessionID:   r.SessionID,
				Command:     r.Command,
				Reason:      reason,
				Risk:        risk,
				ExpiresAt:   r.ExpiresAt,
				CanRemember: canRemember(risk),
			})
		}
	}
	return out
}

// emitDecided 发反向事件，让前端清 pendingApproval。
// ctx 可能已取消（Approve 的 ctx.Done 分支），从 ctx 读不到 runID 时回退为空字符串。
func (s *ApprovalService) emitDecided(ctx context.Context, id, command, decision string) {
	s.emit(ctx, "chat:approval-decided", map[string]any{
		"id":       id,
		"command":  command,
		"decision": decision,
	})
}

func approvalReason(risk string) string {
	if risk == tool.RiskApprovalIrrev {
		return "命令命中危险操作模式（不可逆），需要你确认"
	}
	return "命令不在白名单内，需要你确认后执行"
}
