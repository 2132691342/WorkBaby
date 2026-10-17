// 对话与审批 handler：发送、停止、插话、排队，以及审批清单与决策。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
)

// SendMessage 发消息：立即返回 runID，后续经 SSE 推送。
func (h *Handler) SendMessage(c *gin.Context) {
	var req domain.SendMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Chat.Send(req.SessionID, req.Content, req.Attachments)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// StopRun 停止当前 run。
func (h *Handler) StopRun(c *gin.Context) {
	var req domain.StopRunREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.SessionID == "" {
		req.SessionID = c.Query("session_id")
	}
	if err := h.Svc.Chat.Stop(req.SessionID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SteerMessage 插话。
func (h *Handler) SteerMessage(c *gin.Context) {
	var req domain.QueueMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.Steer(req.SessionID, req.Content); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// FollowUpMessage 排队。
func (h *Handler) FollowUpMessage(c *gin.Context) {
	var req domain.QueueMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.FollowUp(req.SessionID, req.Content); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListApprovals 待决审批，供前端渲染决策卡。
func (h *Handler) ListApprovals(c *gin.Context) {
	ok(c, h.Svc.Approvals.Pending(c.Query("session_id")))
}

// DecideApproval 审批决策；approve / deny 共用，动作来自请求体。
func (h *Handler) DecideApproval(c *gin.Context) {
	var req domain.DecideApprovalREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.Action != "allow" && req.Action != "deny" {
		fail(c, pkg.New("1107", "审批决策只有 allow 与 deny 两种", req.Action))
		return
	}
	if req.Scope == "" {
		req.Scope = domain.ApprovalScopeOnce
	}
	if err := h.Svc.Approvals.Decide(req.SessionID, c.Param("id"), req.Action == "allow", req.Scope); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}
