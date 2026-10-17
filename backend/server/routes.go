// HTTP 路由表：/api/v1 下每个资源的动作路径，与前端 api 层一一对应。
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
			sessions.POST("/rename/:id", h.RenameSession)
			sessions.POST("/workspace/:id", h.SetSessionWorkspace)
			sessions.POST("/delete/:id", h.DeleteSession)
			sessions.POST("/model/:id", h.SetSessionModel)
			sessions.POST("/permission/:id", h.SetSessionPermission)
			sessions.POST("/thinking/:id", h.SetSessionThinking)
			sessions.POST("/fork/:id", h.ForkSession)
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
			approvals.POST("/decide/:id", h.DecideApproval)
		}

		providers := v1.Group("/providers")
		{
			providers.GET("", h.ListProviders)
			providers.POST("", h.UpsertProvider)
			providers.POST("/update/:id", h.UpdateProvider)
			providers.POST("/delete/:id", h.DeleteProvider)
			providers.POST("/test/:id", h.TestProvider)
			providers.POST("/reveal/:id", h.RevealProviderKey)
			providers.POST("/default/:id", h.SetDefaultProvider)
			providers.GET("/models", h.ListModels)
			providers.POST("/models/fetch", h.FetchModels)
		}

		skills := v1.Group("/skills")
		{
			skills.GET("", h.ListSkills)
			skills.POST("", h.CreateSkill)
			skills.POST("/import", h.ImportSkills)
			skills.POST("/import-zip", h.ImportSkillZip)
			skills.POST("/toggle/:id", h.ToggleSkill)
			skills.POST("/delete/:id", h.DeleteSkill)
			skills.GET("/content/:id", h.SkillContent)
		}

		knowledge := v1.Group("/knowledge")
		{
			knowledge.GET("/docs", h.ListDocs)
			knowledge.POST("/docs/add", h.AddDocs)
			knowledge.POST("/docs/delete/:id", h.DeleteDoc)
			knowledge.POST("/reindex", h.StartReindex)
			knowledge.GET("/reindex", h.ReindexStatus)
			knowledge.POST("/reindex/cancel", h.CancelReindex)
			knowledge.POST("/search", h.SearchKnowledge)
		}

		memories := v1.Group("/memories")
		{
			memories.GET("", h.ListMemories)
			memories.POST("", h.AddMemory)
			memories.POST("/update/:id", h.UpdateMemory)
			memories.POST("/delete/:id", h.DeleteMemory)
		}

		v1.GET("/tools", h.ListTools)
		v1.POST("/tools/toggle/:name", h.ToggleTool)
		v1.GET("/files", h.ListFiles)
		v1.GET("/models/capability", h.ModelCapability)
		v1.POST("/models/capabilities", h.ModelCapabilities)
		v1.GET("/models/config", h.GetModelConfig)
		v1.GET("/models/configs", h.ListModelConfigs)
		v1.POST("/models/config", h.UpsertModelConfig)
		v1.GET("/runtime", h.RuntimeStatus)
		v1.POST("/runtime/redetect", h.RedetectRuntime)
		// 环境引导（specs/18）：闸门快照与 PI 的安装更新。
		v1.GET("/env", h.EnvStatus)
		v1.POST("/env/install", h.InstallPi)
		v1.POST("/env/update", h.InstallPi)
		v1.POST("/env/recheck", h.EnvStatus)

		git := v1.Group("/git")
		{
			git.GET("/status", h.GitStatus)
			git.GET("/diff", h.GitDiff)
			// 版本历史（specs/09）：提交列表、单次提交、文件历史、分支清单。
			git.GET("/history", h.GitHistory)
			git.GET("/commit", h.GitCommitDetail)
			git.GET("/file-history", h.GitFileHistory)
			git.GET("/branches", h.GitBranches)
			// 受控写入：白名单六动作，命中禁止清单一律 1018。
			git.POST("/stage", h.GitStage)
			git.POST("/unstage", h.GitUnstage)
			git.POST("/commit", h.GitCommit)
			git.POST("/branch", h.GitCreateBranch)
			git.POST("/switch", h.GitSwitchBranch)
			git.POST("/push", h.GitPush)
			git.POST("/message", h.GitDraftMessage)
		}

		mcp := v1.Group("/mcp")
		{
			mcp.GET("/config", h.GetMCPConfig)
			mcp.POST("/config", h.SaveMCPConfig)
			mcp.GET("/template", h.MCPTemplate)
			mcp.POST("/reload", h.ReloadMCP)
			mcp.GET("/servers", h.ListMCPServers)

			mcp.GET("/tools", h.ListMCPTools)
		}

		runs := v1.Group("/runs")
		{
			runs.GET("", h.ListRuns)
			runs.GET("/:id", h.RunDetail)
		}

		checkpoints := v1.Group("/checkpoints")
		{
			checkpoints.GET("", h.ListCheckpoints)
			checkpoints.POST("/fork/:id", h.ForkCheckpoint)
		}

		evals := v1.Group("/evals")
		{
			evals.GET("", h.ListEvals)
			evals.POST("", h.RegisterEval)
			evals.GET("/baselines", h.ListBaselines)
			evals.POST("/baselines", h.SaveBaseline)
			evals.POST("/baselines/delete/:id", h.DeleteBaseline)
			evals.GET("/compare", h.CompareEval)
		}

		v1.POST("/archive", h.ArchiveNow)

		v1.GET("/docs", h.ListHelpDocs)
		v1.GET("/docs/:name", h.HelpDocContent)

		history := v1.Group("/history")
		{
			history.GET("/entries", h.ScanHistory)
			// id 里带盘符与斜杠（source:path），做不了路径参数，走 query
			history.GET("/entry", h.ReadHistory)
			history.GET("/folders", h.HistoryFolders)
			history.POST("/folders", h.AddHistoryFolder)
			history.POST("/folders/delete", h.RemoveHistoryFolder)
			history.POST("/continue", h.ContinueHistory)
		}

		archmap := v1.Group("/archmap")
		{
			archmap.POST("/analyze", h.AnalyzeArch)
			archmap.GET("/entrypoints", h.ArchEntrypoints)
		}

		v1.GET("/settings", h.AllSettings)
		v1.POST("/settings", h.SetSetting)
		v1.GET("/settings/background", h.GetBackground)
		v1.POST("/settings/background", h.SetBackground)
		v1.GET("/assets/background", h.BackgroundAsset)
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
			// 走统一错误契约而不是手写响应：前端按 code 分流，1101 与「会话不存在」同处置动作。
			err := pkg.New("1101", "事件流连接缺少 session_id", "")
			c.JSON(http.StatusOK, domain.Resp{Code: pkg.CodeOf(err), Message: err.Error()})
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
		// lastReplayed 记录重放推到的位点：Subscribe 与 Replay 快照之间进来的事件
		// 既在重放缓冲里又在客户端通道里，主循环按 seq 去重，否则正文重复拼接。
		lastReplayed := int64(0)
		if after > 0 {
			replay := hub.Replay(sessionID, after)
			if len(replay) == 0 || replay[0].Seq > after+1 {
				// 重放窗口溢出，或缓冲裁掉了中段：提示前端拉快照对账，避免丢事件却毫不知情。
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
				lastReplayed = env.Seq
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
				// 重放已送过的 seq 直接跳过：Subscribe 与 Replay 快照之间的窗口
				// 事件从两条路都会到达，不过滤就会重发。
				if env.Seq > 0 && env.Seq <= lastReplayed {
					continue
				}
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
