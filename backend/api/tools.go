// 工具 handler：清单与启停（写 PI 的 defaultTools）。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
)

// ListTools 工具清单：助手当前能干什么、风险多大、是否启用。
func (h *Handler) ListTools(c *gin.Context) { ok(c, h.Svc.Tools.Catalog()) }

// ToggleTool 启停单个工具。
func (h *Handler) ToggleTool(c *gin.Context) {
	var req domain.ToggleToolREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Tools.SetEnabled(c.Param("name"), req.Enabled); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}
