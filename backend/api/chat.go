// 对话 handler：发送、停止、插话、排队、取消重试，以及回答引擎抛回来的问题。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// SendMessage 发消息：立即返回 runID 与 session_id，后续经 SSE 推送。
// session_id 为空表示新对话，由本次发送现场创建。
func (h *Handler) SendMessage(c *gin.Context) {
	var req domain.SendMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Chat.Send(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// StopRun 停止当前 run。
func (h *Handler) StopRun(c *gin.Context) {
	var req domain.SteerREQ
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
	var req domain.SteerREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.Steer(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// FollowUpMessage 排队。
func (h *Handler) FollowUpMessage(c *gin.Context) {
	var req domain.SteerREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.FollowUp(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// CancelRetry 叫停 PI 正在进行的自动重试。
func (h *Handler) CancelRetry(c *gin.Context) {
	var req domain.SteerREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.SessionID == "" {
		req.SessionID = c.Query("session_id")
	}
	if err := h.Svc.Chat.AbortRetry(req.SessionID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// Answer 回答引擎抛回来的问题（confirm / select / input）。
func (h *Handler) Answer(c *gin.Context) {
	var req domain.AnswerREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.Answer(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListCommands 列出当前会话能跑的斜杠命令（引擎里注册的那些）。
func (h *Handler) ListCommands(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		ok(c, []domain.CommandVO{})
		return
	}
	list, err := h.Svc.Chat.Commands(sessionID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}
