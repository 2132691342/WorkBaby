// 工作区文件 handler：给 @ 引用面板逐级浏览目录。
package api

import (
	"github.com/gin-gonic/gin"
)

// ListFiles 列举工作区目录，供 @ 引用面板逐级浏览。
func (h *Handler) ListFiles(c *gin.Context) {
	vo, err := h.Svc.Files.List(c.Query("path"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}
