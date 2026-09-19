// Package server 后台任务路由：提交 / 列表 / 取消。实时变化走 task:* SSE 通道。
package server

import (
	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"

	"github.com/gin-gonic/gin"
)

func registerTaskRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	v1.GET("/tasks", func(c *gin.Context) {
		v, err := h.ListTasks(atoi(c.DefaultQuery("limit", "50"), 50))
		unwrap(c, v, err)
	})
	v1.POST("/tasks", func(c *gin.Context) {
		var req domain.ChatTaskREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SubmitTask(req)
		unwrap(c, v, err)
	})
	v1.POST("/tasks/:id/cancel", func(c *gin.Context) {
		Fail(c, h.CancelTask(c.Param("id")))
	})
}
