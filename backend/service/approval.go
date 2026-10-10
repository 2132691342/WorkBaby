package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/tool"
)

// 等待确认的时限：无人值守时按拒绝处理，绝不自动放行危险操作。
// 给 30 分钟，覆盖开会 / 午休离座——5 分钟会把「人不在工位」变成一串静默拒绝。
const approvalTimeout = 30 * time.Minute

// ApprovalService 是审批门：内核执行工具前必经，落表以便审计与界面展示。
// 等待中的决策只存在于内存，重启即失效——由 ExpireStale 在启动时收口。
type ApprovalService struct {
	env    *Env
	mu     sync.Mutex
	wait   map[string]waitEntry
	grants map[string]map[string]bool
}

// waitEntry 记录一次等待中的审批：会话随条目一起存，
// 取消整会话时按内存字段过滤，不必逐个回库查（那会在持锁期间做 N 次 SQL）。
type waitEntry struct {
	ch        chan domain.ApprovalDecisionDTO
	sessionID string
}

// NewApprovalService 构造审批服务。
func NewApprovalService(env *Env) *ApprovalService {
	return &ApprovalService{
		env:    env,
		wait:   map[string]waitEntry{},
		grants: map[string]map[string]bool{},
	}
}

// Gate 拦下需要审批的工具调用：落记录 → 推事件 → 等决策。
func (a *ApprovalService) Gate(ctx context.Context, sessionID string, perm string, t tool.Tool, call domain.ApprovalCallDTO) (bool, string) {
	if !t.RequiresApproval() {
		return false, ""
	}
	if perm == domain.PermissionYolo {
		return false, ""
	}
	if perm == domain.PermissionAutoEdit && (t.Name() == "write" || t.Name() == "edit") {
		return false, ""
	}
	a.mu.Lock()
	if a.grants[sessionID] != nil && a.grants[sessionID][t.Name()] {
		a.mu.Unlock()
		return false, ""
	}
	a.mu.Unlock()

	argsJSON, _ := json.Marshal(call.Args)
	rec := &domain.ApprovalDO{
		ID:         pkg.NewID(domain.PrefixApproval),
		SessionID:  sessionID,
		ToolCallID: call.ID,
		Tool:       t.Name(),
		Label:      t.Label(),
		ArgsJSON:   string(argsJSON),
		Risk:       call.Risk,
		Reason:     call.Reason,
		Status:     domain.ApprovalPending,
	}
	if err := a.env.Repo.CreateApproval(rec); err != nil {
		return true, "无法记录这次确认，已按拒绝处理"
	}

	ch := make(chan domain.ApprovalDecisionDTO, 1)
	a.mu.Lock()
	a.wait[rec.ID] = waitEntry{ch: ch, sessionID: sessionID}
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.wait, rec.ID)
		a.mu.Unlock()
	}()

	a.env.Emitter.Emit(sessionID, domain.EventChatApproval, domain.ApprovalData{
		ApprovalID: rec.ID, ToolCallID: call.ID, Tool: t.Name(), Label: t.Label(),
		Args: call.Args, Risk: call.Risk, Reason: call.Reason,
	})

	// 决策必须连 scope 一起回传：只回传布尔值会让「本会话内都放行」永远失效。
	decision, timedOut := a.await(ctx, ch)
	status := domain.ApprovalApproved
	reason := "用户拒绝了这个操作"
	if !decision.Approved {
		status = domain.ApprovalDenied
		if timedOut {
			reason = fmt.Sprintf("等待确认超过 %d 分钟，已按拒绝处理；让助手重新发起即可", int(approvalTimeout.Minutes()))
		}
	} else if decision.Scope == domain.ApprovalScopeSession {
		a.mu.Lock()
		if a.grants[sessionID] == nil {
			a.grants[sessionID] = map[string]bool{}
		}
		a.grants[sessionID][t.Name()] = true
		a.mu.Unlock()
	}
	_ = a.env.Repo.SettleApproval(rec.ID, status, nowMillis())
	return !decision.Approved, reason
}

// await 等决策；ctx 取消与超时都记为不批准。
func (a *ApprovalService) await(ctx context.Context, ch chan domain.ApprovalDecisionDTO) (domain.ApprovalDecisionDTO, bool) {
	timer := time.NewTimer(approvalTimeout)
	defer timer.Stop()
	select {
	case d := <-ch:
		return d, false
	case <-ctx.Done():
		return domain.ApprovalDecisionDTO{}, false
	case <-timer.C:
		return domain.ApprovalDecisionDTO{}, true
	}
}

// Decide 落用户决策并唤醒等待中的工具调用。
func (a *ApprovalService) Decide(id string, approved bool, scope string) error {
	if err := a.env.Repo.SettleApproval(id, boolStatus(approved), nowMillis()); err != nil {
		return err
	}
	a.mu.Lock()
	entry, ok := a.wait[id]
	a.mu.Unlock()
	if ok {
		select {
		case entry.ch <- domain.ApprovalDecisionDTO{Approved: approved, Scope: scope}:
		default:
		}
	}
	return nil
}

// ExpireStale 启动期把上次运行残留的待决审批按拒绝收口。
// 等待通道是内存态，留着它们前端会渲染一批点了没反应的审批卡。
func (a *ApprovalService) ExpireStale() error {
	return a.env.Repo.SettleAllPending(domain.ApprovalDenied, nowMillis())
}

// Pending 列出会话内待决审批，供前端跨重启恢复决策卡。
func (a *ApprovalService) Pending(sessionID string) ([]domain.ApprovalVO, error) {
	recs, err := a.env.Repo.ListPendingApprovals(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ApprovalVO, 0, len(recs))
	for i := range recs {
		out = append(out, approvalVO(&recs[i]))
	}
	return out, nil
}

// CancelSession 取消会话内剩余待决审批（run 被停止时调用）。
// 唤醒在锁内只做非阻塞发信号，DB 收口放锁外——持锁做 SQL 会连带卡住所有 Gate/Decide。
func (a *ApprovalService) CancelSession(sessionID string) {
	a.mu.Lock()
	for _, entry := range a.wait {
		if entry.sessionID == sessionID {
			select {
			case entry.ch <- domain.ApprovalDecisionDTO{}:
			default:
			}
		}
	}
	a.mu.Unlock()
	_ = a.env.Repo.CancelSessionApprovals(sessionID, nowMillis())
}

// ForgetSession 清掉会话的放行记忆。Stop 不能清（用户「本会话内放行」的意愿还在），
// 只有删会话时才清——grants 只增不减会随会话数缓慢泄漏。
func (a *ApprovalService) ForgetSession(sessionID string) {
	a.mu.Lock()
	delete(a.grants, sessionID)
	a.mu.Unlock()
}

func boolStatus(ok bool) string {
	if ok {
		return domain.ApprovalApproved
	}
	return domain.ApprovalDenied
}

func approvalVO(r *domain.ApprovalDO) domain.ApprovalVO {
	var args map[string]any
	_ = json.Unmarshal([]byte(r.ArgsJSON), &args)
	return domain.ApprovalVO{
		ID: r.ID, SessionID: r.SessionID, ToolCallID: r.ToolCallID,
		Tool: r.Tool, Label: r.Label, Args: args, Risk: r.Risk,
		Reason: r.Reason, Status: r.Status, CreatedAt: r.CreatedAt, DecidedAt: r.DecidedAt,
	}
}

// reasonOf 生成审批理由：把工具与参数说成人话。
func reasonOf(t tool.Tool, args map[string]any) string {
	switch t.Name() {
	case "write":
		return fmt.Sprintf("要写入文件 %v", args["path"])
	case "edit":
		return fmt.Sprintf("要修改文件 %v", args["path"])
	case "powershell":
		return fmt.Sprintf("要执行一条命令：%v", args["command"])
	case "python":
		return "要执行一段 Python 脚本"
	default:
		return fmt.Sprintf("要执行 %s", t.Label())
	}
}
