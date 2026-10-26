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
		// 首屏预热：设置页各面板的清单一次拉齐（只读磁盘，不拉起 PI 进程）。
		v1.GET("/warmup", h.Warmup)

		sessions := v1.Group("/sessions")
		{
			sessions.GET("", h.ListSessions)
			sessions.GET("/:id", h.GetSession)
			sessions.POST("/rename/:id", h.RenameSession)
			sessions.POST("/delete/:id", h.DeleteSession)
			sessions.POST("/model/:id", h.SetSessionModel)
			sessions.POST("/thinking/:id", h.SetSessionThinking)
			sessions.POST("/export/:id", h.ExportSession)
		}

		chat := v1.Group("/chat")
		{
			chat.POST("/send", h.SendMessage)
			chat.POST("/stop", h.StopRun)
			chat.POST("/steer", h.SteerMessage)
			chat.POST("/followup", h.FollowUpMessage)
			chat.POST("/retry-cancel", h.CancelRetry)
			chat.POST("/compact", h.Compact)
			chat.POST("/answer", h.Answer)
		}

		// 斜杠命令清单：真相在 PI 进程里（扩展命令只有它知道）。
		v1.GET("/commands", h.ListCommands)

		providers := v1.Group("/providers")
		{
			providers.GET("", h.ListProviders)
			providers.POST("/upsert", h.UpsertProvider)
			providers.POST("/delete", h.DeleteProvider)
			providers.POST("/test", h.TestProvider)
			providers.POST("/reveal", h.RevealProviderKey)
			providers.POST("/default", h.SetDefaultProvider)
			providers.POST("/models/fetch", h.FetchModels)
		}
		v1.GET("/models/available", h.AvailableModels)

		skills := v1.Group("/skills")
		{
			skills.GET("", h.ListSkills)
			skills.POST("/create", h.CreateSkill)
			skills.POST("/import", h.ImportSkills)
			skills.POST("/import-zip", h.ImportSkillZip)
			skills.POST("/toggle/:id", h.ToggleSkill)
			skills.POST("/delete/:id", h.DeleteSkill)
			skills.GET("/content/:id", h.SkillContent)
		}

		// 插件（PI 扩展与包）：路径里有盘符与斜杠，所以路径目标都走请求体。
		extensions := v1.Group("/extensions")
		{
			extensions.GET("", h.ListExtensions)
			extensions.POST("/toggle", h.ToggleExtension)
			extensions.POST("/import", h.ImportExtensions)
			extensions.POST("/delete", h.DeleteExtension)
			extensions.POST("/install", h.InstallPackage)
			extensions.POST("/remove", h.RemovePackage)
			extensions.POST("/reload", h.ReloadExtensions)
			extensions.GET("/market", h.MarketList)
		}

		v1.GET("/tools", h.ListTools)
		v1.POST("/tools/toggle", h.ToggleTool)

		mcp := v1.Group("/mcp")
		{
			mcp.GET("", h.ListMCPServers)
			mcp.POST("/save", h.SaveMCPConfig)
			mcp.POST("/remove", h.RemoveMCPServer)
			mcp.POST("/login", h.LoginMCPServer)
			mcp.POST("/logout", h.LogoutMCPServer)
			mcp.POST("/reload", h.ReloadMCP)
		}

		v1.GET("/files", h.ListFiles)

		// 环境引导：闸门快照、PI 的安装更新、命令执行环境与终端入口。
		v1.GET("/env", h.EnvStatus)
		v1.POST("/env/install", h.InstallPi)
		v1.POST("/env/update", h.EngineUpdate)
		v1.POST("/env/recheck", h.EnvRecheck)
		v1.POST("/env/shell", h.EnvFixShell)
		v1.POST("/env/terminal", h.EnvEnableTerminal)
		v1.POST("/env/terminal-level", h.EnvTerminalLevel)
		// 运行时工具链：用哪份 node / git、装引擎用的是哪份，以及提级与切换。
		v1.GET("/env/toolchain", h.EnvToolchain)
		v1.POST("/env/promote", h.EnvPromote)
		v1.POST("/env/node", h.EnvSwitchNode)
		v1.POST("/env/git/enable", h.EnvEnableBundledGit)

		// 引擎：PI 与运行时的状态、版本对账、升级（设置页「助手引擎」）。
		en := v1.Group("/engine")
		{
			en.GET("/status", h.EngineStatus)
			en.GET("/latest", h.EngineLatest)
			en.POST("/update", h.EngineUpdate)
			en.POST("/packages/update", h.EngineUpdatePackages)
		}

		git := v1.Group("/git")
		{
			git.GET("/status", h.GitStatus)
			git.GET("/diff", h.GitDiff)
			// 版本历史：提交列表、单次提交、文件历史、分支清单。
			git.GET("/history", h.GitHistory)
			git.GET("/commit", h.GitCommitDetail)
			git.GET("/file-history", h.GitFileHistory)
			git.GET("/branches", h.GitBranches)
			// 受控写入：白名单动作，命中禁止清单一律 1018。
			git.POST("/stage", h.GitStage)
			git.POST("/unstage", h.GitUnstage)
			git.POST("/discard", h.GitDiscard)
			git.POST("/resolve", h.GitResolve)
			git.POST("/commit", h.GitCommit)
			git.POST("/branch", h.GitCreateBranch)
			git.POST("/switch", h.GitSwitchBranch)
			git.POST("/branch/delete", h.GitDeleteBranch)
			git.POST("/merge", h.GitMerge)
			git.POST("/state", h.GitState)
			git.POST("/pull", h.GitPull)
			git.POST("/fetch", h.GitFetch)
			git.POST("/push", h.GitPush)
			git.POST("/init", h.GitInit)
			git.POST("/remote", h.GitRemote)
			git.POST("/message", h.GitDraftMessage)
		}

		v1.GET("/docs", h.ListHelpDocs)
		v1.GET("/docs/:name", h.HelpDocContent)

		v1.GET("/settings", h.AllSettings)
		v1.POST("/settings", h.SetSetting)
		// PI 侧的配置：默认模型、思考档、压缩参数与全部文件位置。
		v1.GET("/settings/pi", h.GetPISettings)
		v1.POST("/settings/pi/thinking", h.SetPIThinking)
		v1.POST("/settings/pi/compaction", h.SetPICompaction)
		v1.GET("/settings/workspaces", h.ListWorkspaces)
		v1.GET("/settings/background", h.GetBackground)
		v1.POST("/settings/background", h.SetBackground)
		v1.GET("/assets/background", h.BackgroundAsset)
		v1.GET("/stats", h.Stats)
	}

	e.GET("/api/v1/events", sseHandler(hub))
}

// sseHandler 是事件流端点：订阅维度是会话，支持 Last-Event-ID 重放。
// session_id 留空表示全局通道（安装进度发生在任何会话之前）；seq 优先取 query，header 兜底。
func sseHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Query("session_id")
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
