// 模型服务 handler：增删改、连通测试、取回密钥、设为默认与拉模型列表。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
)

// ListProviders 模型服务列表。
func (h *Handler) ListProviders(c *gin.Context) {
	list, err := h.Svc.Providers.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// UpsertProvider 新增或更新模型服务。
func (h *Handler) UpsertProvider(c *gin.Context) {
	var req domain.UpsertProviderREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Providers.Upsert(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// UpdateProvider 更新指定模型服务。
func (h *Handler) UpdateProvider(c *gin.Context) {
	var req domain.UpsertProviderREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	req.ID = c.Param("id")
	vo, err := h.Svc.Providers.Upsert(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// DeleteProvider 删除模型服务。
func (h *Handler) DeleteProvider(c *gin.Context) {
	if err := h.Svc.Providers.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// TestProvider 连通测试。
func (h *Handler) TestProvider(c *gin.Context) {
	var req domain.TestProviderREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	id := req.ID
	if id == "" {
		id = c.Param("id")
	}
	resp, err := h.Svc.Providers.Test(c.Request.Context(), id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// RevealProviderKey 返回解密后的 API Key：只在用户显式点「显示」时调用。
func (h *Handler) RevealProviderKey(c *gin.Context) {
	key, err := h.Svc.Providers.Reveal(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.RevealProviderKeyVO{APIKey: key})
}

// SetDefaultProvider 设为默认。
func (h *Handler) SetDefaultProvider(c *gin.Context) {
	if err := h.Svc.Providers.SetDefault(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListModels 拉取上游模型列表。
func (h *Handler) ListModels(c *gin.Context) {
	models, err := h.Svc.Providers.Models(c.Request.Context(), c.Query("provider_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, models)
}
