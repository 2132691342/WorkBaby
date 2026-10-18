// 工具 handler：清单来自 PI（会话里的工具声明 + MCP 在线工具），启停写 settings.json。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListTools 列出 PI 当前声明的工具。
func (h *Handler) ListTools(c *gin.Context) { ok(c, h.Svc.Tools.Catalog()) }

// ToggleTool 启停一个工具。
func (h *Handler) ToggleTool(c *gin.Context) {
	var req domain.ToggleToolREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if req.Name == "" {
		req.Name = c.Param("name")
	}
	if err := h.Svc.Tools.SetEnabled(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}
