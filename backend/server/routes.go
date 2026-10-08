package server

import (
	"net/http"
	"strconv"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// registerRoutes 是路由的唯一注册入口；路径与前端 api 层一一对应。
func registerRoutes(e *gin.Engine, h *api.Handler, hub *Hub) {
	v1 := e.Group("/api/v1")
	{
		v1.GET("/health", h.Health)
		v1.GET("/bootstrap", h.Bootstrap)

		sessions := v1.Group("/sessions")
		{
			sessions.GET("", h.ListSessions)
			sessions.POST("", h.CreateSession)
			sessions.GET("/:id", h.GetSession)
			sessions.POST("/:id/rename", h.RenameSession)
			sessions.POST("/:id/workspace", h.SetSessionWorkspace)
			sessions.POST("/:id/delete", h.DeleteSession)
			sessions.POST("/:id/model", h.SetSessionModel)
			sessions.POST("/:id/permission", h.SetSessionPermission)
			sessions.POST("/:id/branch", h.BranchSession)
		}

		chat := v1.Group("/chat")
		{
			chat.POST("/send", h.SendMessage)
			chat.POST("/stop", h.StopRun)
			chat.POST("/steer", h.SteerMessage)
			chat.POST("/followup", h.FollowUpMessage)
		}

		approvals := v1.Group("/approvals")
		{
			approvals.GET("", h.ListApprovals)
			approvals.POST("/:id/:action", h.DecideApproval)
		}

		providers := v1.Group("/providers")
		{
			providers.GET("", h.ListProviders)
			providers.POST("", h.UpsertProvider)
			providers.POST("/:id/update", h.UpdateProvider)
			providers.POST("/:id/delete", h.DeleteProvider)
			providers.POST("/:id/test", h.TestProvider)
			providers.POST("/:id/reveal", h.RevealProviderKey)
			providers.POST("/:id/default", h.SetDefaultProvider)
			providers.GET("/models", h.ListModels)
			providers.POST("/models/fetch", h.FetchModels)
		}

		skills := v1.Group("/skills")
		{
			skills.GET("", h.ListSkills)
			skills.POST("", h.CreateSkill)
			skills.POST("/import", h.ImportSkills)
			skills.POST("/:id/toggle", h.ToggleSkill)
			skills.POST("/:id/delete", h.DeleteSkill)
			skills.GET("/:id/content", h.SkillContent)
		}

		knowledge := v1.Group("/knowledge")
		{
			knowledge.GET("/docs", h.ListDocs)
			knowledge.POST("/docs/add", h.AddDocs)
			knowledge.POST("/docs/:id/delete", h.DeleteDoc)
			knowledge.POST("/reindex", h.ReindexDocs)
			knowledge.POST("/search", h.SearchKnowledge)
		}

		v1.GET("/tools", h.ListTools)
		v1.POST("/tools/:name/toggle", h.ToggleTool)
		v1.GET("/models/capability", h.ModelCapability)
		v1.GET("/models/config", h.GetModelConfig)
		v1.GET("/models/configs", h.ListModelConfigs)
		v1.POST("/models/config", h.UpsertModelConfig)
		v1.GET("/runtime", h.RuntimeStatus)
		v1.POST("/runtime/redetect", h.RedetectRuntime)

		v1.GET("/settings", h.AllSettings)
		v1.POST("/settings", h.SetSetting)
		v1.GET("/stats", h.Stats)
	}

	e.GET("/api/v1/events", sseHandler(hub))
}

// sseHandler 是事件流端点：订阅维度是会话，支持 Last-Event-ID 重放。
// seq 优先取 query（前端手动重建 EventSource 时拿不到自动附加的 header），header 兜底。
func sseHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Query("session_id")
		if sessionID == "" {
			c.JSON(http.StatusOK, domain.Resp{Code: 1101, Message: "缺少 session_id"})
			return
		}
		client := hub.Subscribe(sessionID)
		defer hub.Unsubscribe(client)

		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")

		after := parseSeq(c.Query("last_event_id"))
		if after == 0 {
			after = parseSeq(c.GetHeader("Last-Event-ID"))
		}
		if after > 0 {
			replay := hub.Replay(sessionID, after)
			if len(replay) == 0 {
				// 重放窗口溢出或 seq 不认识：提示前端拉快照对账，避免丢事件却毫不知情。
				gap, _ := encodeEvent(domain.Envelope{Seq: after, Event: domain.EventChatGap, Data: domain.GapData{Reason: "replay_overflow"}})
				_, _ = c.Writer.Write(gap)
			}
			for _, env := range replay {
				frame, err := encodeEvent(env)
				if err != nil {
					continue
				}
				if _, err := c.Writer.Write(frame); err != nil {
					return
				}
			}
			c.Writer.Flush()
		}

		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()
		pkg.Infof("sse: 会话 %s 已接入（%s）", sessionID, c.ClientIP())

		defer func() { pkg.Infof("sse: 会话 %s 断开", sessionID) }()
		// 心跳：WebView2 与中间件都会掐掉长时间静默的连接，
		// 空闲时每 20 秒发一行注释帧，事件流因此不会莫名其妙断掉。
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-client.Done():
				return
			case <-ticker.C:
				if _, err := c.Writer.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				c.Writer.Flush()
			case env := <-client.Events():
				frame, err := encodeEvent(env)
				if err != nil {
					continue
				}
				if _, err := c.Writer.Write(frame); err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

func parseSeq(raw string) int64 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
