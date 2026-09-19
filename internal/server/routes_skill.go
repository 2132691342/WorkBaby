package server

// 技能 / MCP / 工具启停路由。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
)

func registerSkillRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- skills ----
	v1.GET("/skills", func(c *gin.Context) {
		v, err := h.ListSkills()
		unwrap(c, v, err)
	})
	v1.POST("/skills", func(c *gin.Context) {
		var req domain.SkillREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.ImportSkill(req)
		unwrap(c, v, err)
	})
	// 技能包 zip 批量导入（zip_path 由前端原生对话框选取）
	v1.POST("/skills/import-zip", func(c *gin.Context) {
		var req struct {
			ZipPath string `json:"zip_path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.ImportSkillsZip(req.ZipPath)
		unwrap(c, v, err)
	})
	v1.POST("/skills/:name/enabled", func(c *gin.Context) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.SetSkillEnabled(c.Param("name"), req.Enabled))
	})
	v1.POST("/skills/:name/update", func(c *gin.Context) {
		var req domain.SkillREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateSkill(c.Param("name"), req)
		unwrap(c, v, err)
	})
	v1.POST("/skills/:name/delete", func(c *gin.Context) { Fail(c, h.DeleteSkill(c.Param("name"))) })

	// ---- mcp ----
	v1.GET("/mcp/servers", func(c *gin.Context) {
		v, err := h.ListMcpServers()
		unwrap(c, v, err)
	})
	v1.GET("/mcp/servers/raw", func(c *gin.Context) {
		v, err := h.GetMcpRaw()
		unwrap(c, v, err)
	})
	v1.POST("/mcp/servers/raw", func(c *gin.Context) {
		var req domain.McpRawREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveMcpRaw(req)
		unwrap(c, v, err)
	})
	v1.POST("/mcp/servers", func(c *gin.Context) {
		var req domain.McpServerREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.AddMcpServer(req)
		unwrap(c, v, err)
	})
	v1.POST("/mcp/servers/:name/enabled", func(c *gin.Context) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.SetMcpServerEnabled(c.Param("name"), req.Enabled))
	})
	v1.POST("/mcp/servers/:name/delete", func(c *gin.Context) { Fail(c, h.RemoveMcpServer(c.Param("name"))) })
	v1.POST("/mcp/servers/reload", func(c *gin.Context) {
		v, err := h.ReloadMcpServers()
		unwrap(c, v, err)
	})
	v1.POST("/mcp/servers/reveal", func(c *gin.Context) { Fail(c, h.RevealMcpFile()) })

	// ---- tools ----
	v1.GET("/tools", func(c *gin.Context) {
		v, err := h.ListTools()
		unwrap(c, v, err)
	})
	v1.POST("/tools/:name/enabled", func(c *gin.Context) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.SetToolEnabled(c.Param("name"), req.Enabled))
	})

}
