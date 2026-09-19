package api

import "WorkBaby/internal/domain"

// DecideApproval 用户对工具审批请求（chat:approval 事件）的决策回填。
// 阻塞中的 exec 工具调用据此放行/拒绝；请求不存在或已超时返回 4003。
// req.Scope=session 表示「本会话允许」（仅 needs_approval 生效）。
func (h *Handler) DecideApproval(id string, req domain.DecideApprovalREQ) error {
	return h.approvalSvc.Decide(id, req.Approved, req.Scope)
}

// AnswerInput 用户对补充输入请求（request_input 工具，risk=input_required）的回复回填。
func (h *Handler) AnswerInput(id string, req domain.AnswerInputREQ) error {
	return h.approvalSvc.Answer(id, req.Answer)
}

// SkipApproval 用户跳过：审批按拒绝处理，补充输入按未回复处理（模型自行假设继续）。
func (h *Handler) SkipApproval(id string) error {
	return h.approvalSvc.Skip(id)
}

// ListPendingApprovals 返回当前未决审批（前端刷新/重连后恢复审批用）。
func (h *Handler) ListPendingApprovals() []domain.ApprovalPendingRESP {
	return h.approvalSvc.Pending()
}

// ListApprovalGrants 免审授权列表（设置页查看/撤销「本会话允许」持久化授权）。
func (h *Handler) ListApprovalGrants() []domain.ApprovalGrantRESP {
	return h.approvalSvc.ListGrants(h.ctx)
}

// RevokeApprovalGrant 撤销单条免审授权（库内删除 + 进程内免审表同步摘除）。
func (h *Handler) RevokeApprovalGrant(id string) error {
	return h.approvalSvc.RevokeGrant(h.ctx, id)
}
