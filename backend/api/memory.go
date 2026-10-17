// 记忆的 HTTP handler：列表 / 新增 / 改 / 删。界面上写的一律标为 manual。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListMemories 记忆列表。
func (h *Handler) ListMemories(c *gin.Context) {
	list, err := h.Svc.Memory.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// AddMemory 新增一条记忆。
func (h *Handler) AddMemory(c *gin.Context) {
	var req domain.AddMemoryREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Memory.Add(req.Content, domain.MemorySourceManual)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// UpdateMemory 改内容或开关。
func (h *Handler) UpdateMemory(c *gin.Context) {
	var req domain.UpdateMemoryREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	vo, err := h.Svc.Memory.Update(c.Param("id"), req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// DeleteMemory 删除一条记忆。
func (h *Handler) DeleteMemory(c *gin.Context) {
	if err := h.Svc.Memory.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}
