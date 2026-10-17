// 审批编排：把 PI 扩展发出的 confirm/select 请求变成界面上的审批卡，并把决策回传。
// 等待表是内存态：PI 那边阻塞等响应，所以决策必须原样回一条 extension_ui_response。
package service

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// ApprovalService 是审批桥：PI 的 confirm 请求 ↔ 界面的审批卡。
type ApprovalService struct {
	env *Env
	mu  sync.Mutex
	// wait 是「等待决策中」的审批：id（PI 的请求 id）→ 条目。
	wait map[string]*pendingApproval
	// grants 记本会话内已放行的工具（scope=session）。
	grants map[string]map[string]bool
}

// pendingApproval 是一次等待中的审批。
type pendingApproval struct {
	sessionID string
	method    string
	tool      string
	// client 是发出这条请求的 PI 进程：响应必须回到同一个进程。
	workspace string
}

// NewApprovalService 构造审批服务。
func NewApprovalService(env *Env) *ApprovalService {
	return &ApprovalService{
		env:    env,
		wait:   map[string]*pendingApproval{},
		grants: map[string]map[string]bool{},
	}
}

// Open 登记一条来自 PI 的审批请求并广播审批卡。
// 它不阻塞：PI 那边在等 extension_ui_response，我们在这里只负责把卡片推出去。
func (a *ApprovalService) Open(sessionID, requestID, method, title, body string, options []string) {
	tool := toolNameFromTitle(title)
	a.mu.Lock()
	a.wait[requestID] = &pendingApproval{
		sessionID: sessionID, method: method, tool: tool,
	}
	a.mu.Unlock()

	args := map[string]any{}
	if len(options) > 0 {
		// select 的选项带出去，界面按选项渲染而不是「允许 / 拒绝」两键。
		args["options"] = options
	}
	reason := body
	if reason == "" {
		reason = title
	}
	a.env.Emitter.Emit(sessionID, domain.EventChatApproval, domain.ApprovalData{
		ApprovalID: requestID,
		Tool:       tool,
		Label:      title,
		Args:       args,
		Risk:       domain.RiskMedium,
		Reason:     reason,
	})
}

// Decide 把用户决策回给 PI 并发事件。
// 摘不到条目说明等待已结束（超时 / 取消 / 重复点击）：返回 4102 且不改任何状态。
func (a *ApprovalService) Decide(sessionID, approvalID string, approved bool, scope string) error {
	a.mu.Lock()
	entry, ok := a.wait[approvalID]
	if ok {
		delete(a.wait, approvalID)
	}
	a.mu.Unlock()
	if !ok {
		return domain.ErrApprovalSettled
	}
	if scope == domain.ApprovalScopeSession && approved {
		a.mu.Lock()
		if a.grants[sessionID] == nil {
			a.grants[sessionID] = map[string]bool{}
		}
		a.grants[sessionID][entry.tool] = true
		a.mu.Unlock()
	}

	// 响应必须回到发出请求的那个 PI 进程；进程换过就没人接了。
	client := a.env.PiSupervisor().Get(entry.workspace)
	if client == nil || !client.Alive() {
		return pkg.New("9701", "助手进程已经退出，这条确认没有送达", approvalID)
	}
	resp := map[string]any{
		"type": "extension_ui_response",
		"id":   approvalID,
	}
	// confirm 用 confirmed，select 用 value；用错字段 PI 会当成取消。
	if entry.method == "select" {
		if approved {
			resp["value"] = "Allow"
		} else {
			resp["cancelled"] = true
		}
	} else {
		resp["confirmed"] = approved
	}
	if err := client.WriteResponse(resp); err != nil {
		return err
	}
	return nil
}

// Pending 列出会话内待决审批（内存态，供前端跨重连恢复决策卡）。
func (a *ApprovalService) Pending(sessionID string) []domain.ApprovalVO {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []domain.ApprovalVO{}
	for id, entry := range a.wait {
		if sessionID != "" && entry.sessionID != sessionID {
			continue
		}
		out = append(out, domain.ApprovalVO{
			ID: id, SessionID: entry.sessionID, Tool: entry.tool,
			Label: entry.tool, Status: domain.ApprovalPending,
		})
	}
	return out
}

// CancelSession 取消会话内剩余待决审批：回 cancelled，让 PI 那边按拒绝收口。
// 不回的话 PI 会一直阻塞在那个 confirm 上。
func (a *ApprovalService) CancelSession(sessionID string) {
	a.mu.Lock()
	victims := map[string]*pendingApproval{}
	for id, entry := range a.wait {
		if entry.sessionID == sessionID {
			victims[id] = entry
			delete(a.wait, id)
		}
	}
	a.mu.Unlock()

	for id, entry := range victims {
		client := a.env.PiSupervisor().Get(entry.workspace)
		if client == nil || !client.Alive() {
			continue
		}
		_ = client.WriteResponse(map[string]any{
			"type": "extension_ui_response", "id": id, "cancelled": true,
		})
	}
}

// ForgetSession 清掉会话的放行记忆。Stop 不能清（用户「本会话内放行」的意愿还在），
// 只有删会话时才清——grants 只增不减会随会话数缓慢泄漏。
func (a *ApprovalService) ForgetSession(sessionID string) {
	a.mu.Lock()
	delete(a.grants, sessionID)
	a.mu.Unlock()
}

// AllowedTools 返回本会话已放行的工具名，供写进 policy 文件。
func (a *ApprovalService) AllowedTools(sessionID string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	set := a.grants[sessionID]
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	return out
}

// toolNameFromTitle 从审批标题里取工具名。
// 标题由 WorkBaby 的扩展生成，格式固定为「允许 <工具名>?」，解析失败就用整个标题。
func toolNameFromTitle(title string) string {
	const prefix = "允许 "
	if len(title) > len(prefix) && title[:len(prefix)] == prefix {
		rest := title[len(prefix):]
		for i, r := range rest {
			if r == '?' || r == '？' {
				return rest[:i]
			}
		}
		return rest
	}
	return title
}

// ExpireStale 启动期收口：等待表是内存态，进程重启后上次的待决审批全部失效。
// 返回空列表即可——本次没有残留需要报告（上次的等待随进程一起没了）。
func (a *ApprovalService) ExpireStale() ([]domain.ApprovalVO, error) {
	a.mu.Lock()
	stale := len(a.wait)
	a.wait = map[string]*pendingApproval{}
	a.mu.Unlock()
	if stale > 0 {
		pkg.Warnf("approval: 清掉 %d 条上次残留的等待中审批", stale)
	}
	return nil, nil
}
