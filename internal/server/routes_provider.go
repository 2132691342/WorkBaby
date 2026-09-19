package server

// 模型 Provider 与全局设置路由。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
)

func registerProviderRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- ai-provider ----
	v1.GET("/ai-provider", func(c *gin.Context) {
		v, err := h.ListProviders()
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider", func(c *gin.Context) {
		var req domain.AiProviderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreateProvider(req)
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/available", func(c *gin.Context) {
		v, err := h.ListAvailableModels()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/kinds", func(c *gin.Context) {
		v, err := h.ListProviderKinds()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/tiers", func(c *gin.Context) {
		v, err := h.ListProviderTiers()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/presets", func(c *gin.Context) {
		v, err := h.ListProviderPresets()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/:id", func(c *gin.Context) {
		v, err := h.GetProvider(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/update", func(c *gin.Context) {
		var req domain.AiProviderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateProvider(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/delete", func(c *gin.Context) { Fail(c, h.DeleteProvider(c.Param("id"))) })
	v1.POST("/ai-provider/:id/test", func(c *gin.Context) { Fail(c, h.TestProviderConnect(c.Param("id"))) })
	v1.POST("/ai-provider/reload", func(c *gin.Context) {
		// model.json 热重载：重新读文件 → 同步 DB → 重建 Registry
		v, err := h.ReloadProvidersFromFile()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/circuit-status", func(c *gin.Context) {
		v, err := h.GetCircuitStatus()
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/reset-circuit", func(c *gin.Context) {
		v, err := h.ResetCircuit(c.Param("id"))
		unwrap(c, v, err)
	})

	// ---- settings ----
	v1.GET("/settings", func(c *gin.Context) {
		v, err := h.ListSettings()
		unwrap(c, v, err)
	})
	v1.GET("/settings/general", func(c *gin.Context) {
		v, err := h.GetGeneralSettings()
		unwrap(c, v, err)
	})
	v1.POST("/settings/general", func(c *gin.Context) {
		var req map[string]any
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveGeneralSettings(req)
		unwrap(c, v, err)
	})
	v1.GET("/settings/websearch", func(c *gin.Context) {
		v, err := h.GetWebSearchConfig()
		unwrap(c, v, err)
	})
	v1.POST("/settings/websearch", func(c *gin.Context) {
		var req domain.WebSearchConfigRESP
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveWebSearchConfig(req)
		unwrap(c, v, err)
	})
	v1.GET("/settings/exec/agent", func(c *gin.Context) {
		v, err := h.GetExecWhitelist()
		unwrap(c, v, err)
	})
	v1.POST("/settings/exec/agent", func(c *gin.Context) {
		var req []string
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveExecWhitelist(req)
		unwrap(c, v, err)
	})
	v1.GET("/kv/:key", func(c *gin.Context) {
		v, err := h.GetSetting(c.Param("key"))
		unwrap(c, v, err)
	})
	v1.POST("/kv/:key", func(c *gin.Context) {
		var req struct {
			Value string `json:"value"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSetting(c.Param("key"), req.Value)
		unwrap(c, v, err)
	})

}
