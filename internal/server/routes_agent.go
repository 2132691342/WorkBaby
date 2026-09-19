package server

// 自定义子智能体路由（delegate_task 委派目标的用户自定义层）。

import (
	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
	"github.com/gin-gonic/gin"
)

func registerAgentRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- agent profiles ----
	v1.GET("/agent-profiles", func(c *gin.Context) {
		v, err := h.ListAgentProfiles()
		unwrap(c, v, err)
	})
	v1.POST("/agent-profiles", func(c *gin.Context) {
		var req domain.AgentProfileREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpsertAgentProfile(req)
		unwrap(c, v, err)
	})
	v1.POST("/agent-profiles/:name/enabled", func(c *gin.Context) {
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.SetAgentProfileEnabled(c.Param("name"), req.Enabled))
	})
	v1.POST("/agent-profiles/:name/delete", func(c *gin.Context) {
		Fail(c, h.DeleteAgentProfile(c.Param("name")))
	})
}
