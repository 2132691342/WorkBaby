// 会话 handler：列表、详情、改名、删除、换模型与思考档、导出 HTML。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListSessions 列出会话；workspace 非空时只列那个目录下的。
func (h *Handler) ListSessions(c *gin.Context) {
	list, err := h.Svc.Sessions.List(c.Query("workspace"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// GetSession 会话详情：元信息 + 投影后的消息。
func (h *Handler) GetSession(c *gin.Context) {
	detail, err := h.Svc.Sessions.Detail(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, detail)
}

// RenameSession 改名。
func (h *Handler) RenameSession(c *gin.Context) {
	var req domain.RenameSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.Rename(c.Param("id"), req.Title); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// DeleteSession 删除会话（删的是 PI 的会话文件）。
func (h *Handler) DeleteSession(c *gin.Context) {
	if err := h.Svc.Sessions.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionModel 切换模型。
func (h *Handler) SetSessionModel(c *gin.Context) {
	var req domain.SetModelREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetModel(c.Param("id"), req.Provider, req.Model); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionThinking 切换思考档。
func (h *Handler) SetSessionThinking(c *gin.Context) {
	var req domain.SetThinkingREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetThinking(c.Param("id"), req.Level); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ExportSession 把会话导成 HTML；目标路径由前端选（保存对话框）。
func (h *Handler) ExportSession(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	path, err := h.Svc.Chat.ExportHTML(c.Param("id"), req.Path)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, path)
}
