// 模型服务 handler：models.json 的增删改查、连通测试、拉模型列表与默认模型。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListProviders 列出服务商（密钥只给掩码）。
func (h *Handler) ListProviders(c *gin.Context) {
	vo, err := h.Svc.Providers.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// UpsertProvider 新增或更新一个服务商。
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

// DeleteProvider 删除一个服务商。
func (h *Handler) DeleteProvider(c *gin.Context) {
	var req domain.ProviderIDREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	if err := h.Svc.Providers.Delete(req.ID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// RevealProviderKey 显式查看密钥明文。
func (h *Handler) RevealProviderKey(c *gin.Context) {
	var req domain.ProviderIDREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	key, err := h.Svc.Providers.Reveal(req.ID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.RevealProviderKeyVO{APIKey: key})
}

// TestProvider 连通测试。
func (h *Handler) TestProvider(c *gin.Context) {
	var req domain.ProviderIDREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.ID == "" {
		req.ID = c.Param("id")
	}
	vo, err := h.Svc.Providers.Test(c.Request.Context(), req.ID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// SetDefaultProvider 设默认模型（写 PI 的 settings.json）。
func (h *Handler) SetDefaultProvider(c *gin.Context) {
	var req domain.SetDefaultREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Providers.SetDefault(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// FetchModels 用给定连接信息向上游要模型列表。
func (h *Handler) FetchModels(c *gin.Context) {
	var req domain.FetchModelsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if req.API == "" {
		req.API = domain.APIOpenAICompletions
	}
	vo, err := h.Svc.Providers.FetchModels(c.Request.Context(), req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// AvailableModels 向 PI 进程要它眼下的可用模型（含内置服务商与登录态）。
func (h *Handler) AvailableModels(c *gin.Context) {
	list, err := h.Svc.Providers.Available(c.Request.Context(), c.Query("workspace"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}
