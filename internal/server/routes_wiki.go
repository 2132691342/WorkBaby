// Package server Wiki 导读路由：概览 + 单页。
package server

import (
	"WorkBaby/internal/api"

	"github.com/gin-gonic/gin"
)

func registerWikiRoutes(g *gin.RouterGroup, h *api.Handler) {
	// 全仓导读（session_id 定位工作区）
	g.GET("/wiki/overview", func(c *gin.Context) {
		v, err := h.WikiOverview(c.Query("session_id"))
		unwrap(c, v, err)
	})
	// 单页：目录子项清单 / 文件正文
	g.GET("/wiki/page", func(c *gin.Context) {
		v, err := h.WikiPage(c.Query("session_id"), c.Query("path"))
		unwrap(c, v, err)
	})
}
