// 工作区版本视图的 HTTP handler：status 与 diff 两个只读端点。
package api

// 工作区版本视图 handler：状态与差异只读，编排全部在 service 层。

import "github.com/gin-gonic/gin"

// GitStatus 版本状态。带 session_id 时看该会话的工作目录，否则看全局工作目录。
func (h *Handler) GitStatus(c *gin.Context) {
	vo, err := h.Svc.Git.Status(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitDiff 取 path 指定文件的差异；不带 path 表示整个工作区。
func (h *Handler) GitDiff(c *gin.Context) {
	vo, err := h.Svc.Git.Diff(c.Query("session_id"), c.Query("path"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}
