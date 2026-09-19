// Package server 用户钩子路由：CRUD + 试跑。
package server

import (
	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"

	"github.com/gin-gonic/gin"
)

func registerHookRoutes(g *gin.RouterGroup, h *api.Handler) {
	g.GET("/hooks", func(c *gin.Context) {
		v, err := h.ListHooks()
		unwrap(c, v, err)
	})
	g.POST("/hooks", func(c *gin.Context) {
		var req domain.UserHookREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpsertHook(req)
		unwrap(c, v, err)
	})
	g.POST("/hooks/:id/delete", func(c *gin.Context) {
		Fail(c, h.DeleteHook(c.Param("id")))
	})
	// 试跑：以样例载荷执行一次，返回决策与耗时
	g.POST("/hooks/:id/test", func(c *gin.Context) {
		v, err := h.TestHook(c.Param("id"))
		unwrap(c, v, err)
	})
}
