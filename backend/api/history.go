// 通用会话历史 HTTP handler：薄层。
package api

import (
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ScanHistory 扫描全部来源的历史会话。
func (h *Handler) ScanHistory(c *gin.Context) {
	res, err := h.Svc.History.Scan()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, res)
}

// ReadHistory 一段会话的完整消息（只读）。id 走 query：它带盘符与斜杠，
// 做成路径参数会被 gin 按 / 切开。
func (h *Handler) ReadHistory(c *gin.Context) {
	e, err := h.Svc.History.Read(c.Query("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, e)
}

// HistoryFolders 已注册的自定义文件夹。
func (h *Handler) HistoryFolders(c *gin.Context) {
	m, err := h.Svc.History.Folders()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, m)
}

// AddHistoryFolder 注册自定义文件夹。
func (h *Handler) AddHistoryFolder(c *gin.Context) {
	var req struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.History.AddFolder(req.Source, req.Path); err != nil {
		fail(c, err)
		return
	}
	ok(c, nil)
}

// RemoveHistoryFolder 删除注册。
func (h *Handler) RemoveHistoryFolder(c *gin.Context) {
	var req struct {
		Source string `json:"source"`
	}
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if err := h.Svc.History.RemoveFolder(req.Source); err != nil {
		fail(c, err)
		return
	}
	ok(c, nil)
}

// ContinueHistory 接着聊：新建会话并注入上下文。
func (h *Handler) ContinueHistory(c *gin.Context) {
	var req struct {
		EntryID string `json:"entry_id"`
		Next    string `json:"next"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.History.Continue(req.EntryID, req.Next)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}
