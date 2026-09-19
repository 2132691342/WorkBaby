// Package server 桌宠路由：配置 / 形象 / 状态 / 窗口形态与点击穿透。
package server

import (
	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"

	"github.com/gin-gonic/gin"
)

func registerPetRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	v1.GET("/pet/state", func(c *gin.Context) {
		v, err := h.GetPetState()
		unwrap(c, v, err)
	})
	v1.GET("/pet/config", func(c *gin.Context) {
		v, err := h.GetPetConfig()
		unwrap(c, v, err)
	})
	v1.POST("/pet/config/update", func(c *gin.Context) {
		var req domain.PetConfigREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdatePetConfig(req)
		unwrap(c, v, err)
	})
	v1.GET("/pet/sprites", func(c *gin.Context) {
		v, err := h.ListPetSprites()
		unwrap(c, v, err)
	})
	v1.POST("/pet/sprites", func(c *gin.Context) {
		var req domain.PetSpriteREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreatePetSprite(req)
		unwrap(c, v, err)
	})
	v1.POST("/pet/sprites/:id/delete", func(c *gin.Context) {
		v, err := h.DeletePetSprite(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/pet/window/:mode", func(c *gin.Context) {
		v, err := h.PetToggleMode()
		unwrap(c, v, err)
	})
	v1.POST("/pet/window/click-through", func(c *gin.Context) {
		var req domain.PetClickThroughREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetPetClickThrough(req)
		unwrap(c, v, err)
	})
}
