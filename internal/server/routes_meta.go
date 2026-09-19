package server

// 元信息与运维路由：meta / admin / 内置文档 / 仪表盘 / SSE / 受管文件静态服务。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

func registerMetaRoutes(s *Server, v1 *gin.RouterGroup, h *api.Handler) {
	// ---- meta ----
	v1.GET("/meta/version", func(c *gin.Context) { OK(c, h.GetVersion()) })
	v1.GET("/meta/health", func(c *gin.Context) { OK(c, h.GetHealth()) })
	v1.GET("/meta/contract", func(c *gin.Context) { OK(c, h.GetContractVersion()) })
	v1.GET("/meta/runtime", func(c *gin.Context) { OK(c, h.GetRuntimeStatus()) })

	// ---- admin ----
	v1.GET("/admin/overview", func(c *gin.Context) {
		v, err := h.GetAdminOverview()
		unwrap(c, v, err)
	})
	v1.POST("/admin/cleanup-token-usages", func(c *gin.Context) {
		v, err := h.CleanupMisreportedTokenUsage()
		unwrap(c, v, err)
	})

	// ---- docs (built-in) ----
	v1.GET("/docs", func(c *gin.Context) {
		v, err := h.ListDocs()
		unwrap(c, v, err)
	})
	v1.GET("/docs/:name", func(c *gin.Context) {
		v, err := h.GetDoc(c.Param("name"))
		unwrap(c, v, err)
	})

	// ---- dashboard ----
	v1.GET("/dashboard/stats", func(c *gin.Context) {
		v, err := h.GetDashboardStats()
		unwrap(c, v, err)
	})
	v1.GET("/dashboard/trend", func(c *gin.Context) {
		days := atoi(c.DefaultQuery("range", "7"), 7)
		v, err := h.GetDashboardTrend(days)
		unwrap(c, v, err)
	})
	v1.GET("/dashboard/token-trend", func(c *gin.Context) {
		req := domain.TokenTrendREQ{
			Scope:   domain.TokenTrendScope(c.DefaultQuery("scope", string(domain.TrendScopeToday))),
			StartAt: atoi64(c.Query("start_at"), 0),
			EndAt:   atoi64(c.Query("end_at"), 0),
		}
		v, err := h.GetTokenTrend(req)
		unwrap(c, v, err)
	})

	// ---- events (SSE) ----
	v1.GET("/events", s.hub.Serve)

	// ---- files (本地受管文件：media/sprites/files/workspace) ----
	// 与业务 API 同 origin，前端 <img>/fetch 在 dev 与生产均可直接加载；
	// 生产环境 Wails AssetServer 同样把 /files/** 转发到同一 FileServer，双通道行为一致。
	s.engine.GET("/files/*filepath", func(c *gin.Context) {
		if h.Files == nil {
			Fail(c, pkg.New(1404, "file service not ready", ""))
			return
		}
		h.Files.ServeHTTP(c.Writer, c.Request)
	})

}
