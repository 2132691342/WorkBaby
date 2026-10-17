// 模型 handler：能力画像与逐模型配置的读写。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
)

// ModelCapability 查模型能力：上下文窗口、是否支持思考、是否识图。
func (h *Handler) ModelCapability(c *gin.Context) {
	model := c.Query("model")
	if model == "" {
		fail(c, pkg.New("3111", "请指定要查询的模型", ""))
		return
	}
	ok(c, h.Svc.Chat.ModelCapability(c.Query("provider_id"), model))
}

// ModelCapabilities 批量查模型能力：换模型下拉一次列几十上百个模型，
// 逐个请求等于把 N+1 打到本机服务上。
func (h *Handler) ModelCapabilities(c *gin.Context) {
	var req domain.ModelCapabilitiesREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	ok(c, h.Svc.Chat.ModelCapabilities(req.ProviderID, req.Models))
}

// GetModelConfig 查单个模型配置（目录 + 覆写合并后的最终值）。
func (h *Handler) GetModelConfig(c *gin.Context) {
	model := c.Query("model")
	if model == "" {
		fail(c, pkg.New("3111", "请指定要查询的模型", ""))
		return
	}
	vo, err := h.Svc.Providers.ModelConfig(c.Query("provider_id"), model)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ListModelConfigs 列出一个服务下的全部模型配置。
func (h *Handler) ListModelConfigs(c *gin.Context) {
	list, err := h.Svc.Providers.ListModelConfigs(c.Query("provider_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// UpsertModelConfig 保存模型配置。
func (h *Handler) UpsertModelConfig(c *gin.Context) {
	var req domain.UpsertModelConfigREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Providers.UpsertModelConfig(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// FetchModels 用未保存的连接信息拉上游模型列表（新增服务场景）。
func (h *Handler) FetchModels(c *gin.Context) {
	var req domain.FetchModelsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if req.API == "" {
		req.API = domain.APIOpenAICompletions
	}
	models, err := h.Svc.Providers.FetchModels(c.Request.Context(), req.API, req.BaseURL, req.APIKey)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, models)
}
