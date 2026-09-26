package server

// 元信息与运维路由：meta / admin / 内置文档 / 仪表盘 / SSE / 受管文件静态服务。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"net/http"
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
func registerChatRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- chat / session ----
	v1.GET("/chat/sessions", func(c *gin.Context) {
		page := atoi(c.DefaultQuery("page", "1"), 1)
		pageSize := atoi(c.DefaultQuery("page_size", "20"), 20)
		v, err := h.ListSessions(page, pageSize)
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions", func(c *gin.Context) {
		var req domain.ChatSessionREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreateSession(req)
		unwrap(c, v, err)
	})
	v1.GET("/chat/sessions/:id", func(c *gin.Context) {
		v, err := h.GetSession(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/rename", func(c *gin.Context) {
		var req domain.ChatSessionRenameREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.RenameSession(c.Param("id"), req.Name)
		unwrap(c, v, err)
	})
	v1.GET("/chat/sessions/:id/params", func(c *gin.Context) {
		v, err := h.EffectiveParams(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/permission", func(c *gin.Context) {
		var req domain.ChatSessionPermissionREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSessionPermission(c.Param("id"), req)
		unwrap(c, v, err)
	})
	// 置顶 / 归档（侧栏管理；归档自动取消置顶）
	v1.POST("/chat/sessions/:id/pin", func(c *gin.Context) {
		var req domain.SessionPinREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSessionPinned(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/archive", func(c *gin.Context) {
		var req domain.SessionArchiveREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSessionArchived(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/model", func(c *gin.Context) {
		var req domain.ChatSessionModelREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSessionModel(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/agent", func(c *gin.Context) {
		var req domain.SetSessionAgentREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.SetSessionAgent(c.Param("id"), req))
	})
	v1.GET("/chat/sessions/:id/goal", func(c *gin.Context) {
		v, err := h.GetSessionGoal(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/goal", func(c *gin.Context) {
		var req domain.GoalREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSessionGoal(c.Param("id"), req)
		unwrap(c, v, err)
	})
	// 辅助对话（右栏并行小会话）：GET 返回已有的（null = 无），POST 幂等确保（无则建）
	v1.GET("/chat/sessions/:id/side", func(c *gin.Context) {
		v, err := h.GetSideConversation(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/side", func(c *gin.Context) {
		v, err := h.EnsureSideConversation(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/delete", func(c *gin.Context) { Fail(c, h.DeleteSession(c.Param("id"))) })
	v1.POST("/chat/sessions/delete-batch", func(c *gin.Context) {
		var ids []string
		if err := BindJSON(c, &ids); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.DeleteSessions(ids)
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/workspace", func(c *gin.Context) {
		var req struct {
			WorkspacePath string `json:"workspace_path"`
			WorkspaceID   string `json:"workspace_id"` // 旧前端字段，仅作兜底
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		p := req.WorkspacePath
		if p == "" {
			p = req.WorkspaceID
		}
		v, err := h.UpdateSessionWorkspace(c.Param("id"), p)
		unwrap(c, v, err)
	})
	v1.GET("/chat/sessions/:id/messages", func(c *gin.Context) {
		afterSeq := atoi64(c.DefaultQuery("after_seq", "0"), 0)
		limit := atoi(c.DefaultQuery("limit", "200"), 200)
		v, err := h.ListMessages(c.Param("id"), afterSeq, limit)
		unwrap(c, v, err)
	})
	v1.POST("/chat/stream", func(c *gin.Context) {
		var req domain.SendStreamREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SendStream(req)
		unwrap(c, v, err)
	})
	// 中止会话内正在跑的 run：路径参数是 sessionID，与 resume 的 runID 显式区分。
	v1.POST("/chat/sessions/:id/cancel", func(c *gin.Context) { Fail(c, h.CancelStream(c.Param("id"))) })
	// 检查点续跑：路径参数是 runID（中断/崩溃后同一 runID 恢复）。
	v1.POST("/chat/runs/:id/resume", func(c *gin.Context) {
		v, err := h.ResumeChat(c.Param("id"))
		unwrap(c, v, err)
	})
	// 运行历史索引（事件明细回放走 /chat/runs/:id/events）
	v1.GET("/chat/runs", func(c *gin.Context) {
		var req domain.RunRecordListREQ
		if err := c.ShouldBindQuery(&req); err != nil {
			Fail(c, pkg.Wrap(1001, "invalid query", err))
			return
		}
		v, err := h.ListRuns(req)
		unwrap(c, v, err)
	})
	// run 事件 JSONL 无头导出
	v1.GET("/chat/runs/:id/events", func(c *gin.Context) {
		v, err := h.ExportRunEvents(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/steer", func(c *gin.Context) {
		var req domain.SteerREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SteerSession(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.GET("/chat/sessions/:id/todos", func(c *gin.Context) {
		OK(c, h.GetSessionTodos(c.Param("id")))
	})
	// 计划项勾选：变量参数在路径末尾（AGENTS.md）
	v1.POST("/chat/sessions/:id/todos/:itemID/toggle", func(c *gin.Context) {
		v, err := h.ToggleSessionTodo(c.Param("id"), c.Param("itemID"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/sessions/:id/clear", func(c *gin.Context) { Fail(c, h.ClearMessages(c.Param("id"))) })
	v1.POST("/chat/messages/:sid/delete/:id", func(c *gin.Context) {
		Fail(c, h.DeleteMessage(c.Param("sid"), c.Param("id")))
	})
	v1.POST("/chat/messages/:sid/truncate", func(c *gin.Context) {
		var req domain.TruncateMessagesREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.TruncateMessages(c.Param("sid"), req))
	})
	v1.POST("/chat/messages/:sid/fork", func(c *gin.Context) {
		var req domain.ForkSessionREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.ForkSession(c.Param("sid"), req)
		unwrap(c, v, err)
	})
	v1.POST("/chat/approval/:id/decide", func(c *gin.Context) {
		var req domain.DecideApprovalREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.DecideApproval(c.Param("id"), req))
	})
	v1.POST("/chat/approval/:id/answer", func(c *gin.Context) {
		var req domain.AnswerInputREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.AnswerInput(c.Param("id"), req))
	})
	v1.POST("/chat/approval/:id/skip", func(c *gin.Context) {
		Fail(c, h.SkipApproval(c.Param("id")))
	})
	v1.GET("/chat/approvals/pending", func(c *gin.Context) {
		OK(c, h.ListPendingApprovals())
	})
	// 免审授权管理（「本会话允许」的持久化授权：查看 / 撤销）
	v1.GET("/chat/approval-grants", func(c *gin.Context) {
		OK(c, h.ListApprovalGrants())
	})
	v1.POST("/chat/approval-grants/:id/delete", func(c *gin.Context) {
		Fail(c, h.RevokeApprovalGrant(c.Param("id")))
	})
	// 斜杠命令：元数据（命令面板）+ 需后端能力的动作。
	// session_id 可选：带上时一并加载该会话工作区下的命令文件（就近覆盖）。
	v1.GET("/chat/commands", func(c *gin.Context) {
		v, err := h.ListChatCommands(c.Query("session_id"))
		unwrap(c, v, err)
	})
	// 自定义斜杠命令（保存的提示词模板）：面板合并展示 + 设置页管理
	v1.GET("/chat/commands/custom", func(c *gin.Context) {
		v, err := h.ListCustomCommands()
		unwrap(c, v, err)
	})
	v1.POST("/chat/commands/custom", func(c *gin.Context) {
		var req domain.UserCommandREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpsertCustomCommand(req)
		unwrap(c, v, err)
	})
	v1.POST("/chat/commands/custom/:name/delete", func(c *gin.Context) {
		Fail(c, h.DeleteCustomCommand(c.Param("name")))
	})
	v1.POST("/chat/sessions/:id/compact", func(c *gin.Context) {
		var req domain.CompactREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CompactSession(c.Param("id"), req)
		unwrap(c, v, err)
	})
	// 跨会话快速检索（/resume）：title / content / all 三档
	v1.GET("/chat/sessions/search", func(c *gin.Context) {
		req := domain.SessionSearchREQ{
			Query: c.Query("q"),
			Scope: c.Query("scope"),
			Limit: atoi(c.DefaultQuery("limit", "20"), 20),
		}
		v, err := h.SearchSessions(req)
		unwrap(c, v, err)
	})
	// 上下文占用分段快照（/context 侧栏环）
	v1.GET("/chat/sessions/:id/usage/context", func(c *gin.Context) {
		v, err := h.ContextUsage(c.Param("id"))
		unwrap(c, v, err)
	})

}
func registerFileRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- folders / files ----
	v1.GET("/folders/tree", func(c *gin.Context) {
		v, err := h.GetFolderTree(c.Query("workspaceID"))
		unwrap(c, v, err)
	})
	v1.GET("/folders", func(c *gin.Context) {
		v, err := h.ListFolders(c.Query("parentID"))
		unwrap(c, v, err)
	})
	v1.POST("/folders", func(c *gin.Context) {
		var req domain.FolderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreateFolder(req)
		unwrap(c, v, err)
	})
	v1.POST("/folders/:id/update", func(c *gin.Context) {
		var req domain.FolderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateFolder(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/folders/:id/delete", func(c *gin.Context) {
		v, err := h.DeleteFolder(c.Param("id"))
		unwrap(c, v, err)
	})

	v1.GET("/files/search", func(c *gin.Context) {
		v, err := h.SearchFiles(c.Query("q"))
		unwrap(c, v, err)
	})
	v1.GET("/files", func(c *gin.Context) {
		v, err := h.ListFiles()
		unwrap(c, v, err)
	})
	v1.POST("/files/upload", func(c *gin.Context) {
		var req struct {
			Name       string `json:"name"`
			SourcePath string `json:"source_path"`
			SessionID  string `json:"session_id"`
			FolderID   string `json:"folder_id"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UploadFile(req.Name, req.SourcePath, req.SessionID, req.FolderID)
		unwrap(c, v, err)
	})
	v1.POST("/files/upload-data", func(c *gin.Context) {
		var req domain.UploadDataREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UploadFileData(req)
		unwrap(c, v, err)
	})
	v1.POST("/files/:id/delete", func(c *gin.Context) {
		v, err := h.DeleteFile(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.GET("/files/:id/preview-url", func(c *gin.Context) {
		v, err := h.GetFilePreviewURL(c.Param("id"))
		unwrap(c, v, err)
	})
}
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
func registerKnowledgeRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- knowledge (kdocs) ----
	v1.GET("/kdocs", func(c *gin.Context) {
		v, err := h.ListKnowledgeDocs()
		unwrap(c, v, err)
	})
	v1.POST("/kdocs", func(c *gin.Context) {
		var req domain.KnowledgeDocREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.AddKnowledgeDoc(req)
		unwrap(c, v, err)
	})
	// 本地文件导入受管知识库：前端原生对话框选路径 → 后端复制到 {home}/knowledge 并后台索引
	v1.POST("/kdocs/import-file", func(c *gin.Context) {
		var req struct {
			Name       string  `json:"name"`
			SourcePath string  `json:"source_path"`
			FolderID   *string `json:"folder_id"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		folderID := ""
		if req.FolderID != nil {
			folderID = *req.FolderID
		}
		v, err := h.ImportKnowledgeFile(req.Name, req.SourcePath, folderID)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/search", func(c *gin.Context) {
		topK := atoi(c.DefaultQuery("limit", "20"), 20)
		v, err := h.SearchKnowledge(c.Query("q"), topK)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/groups", func(c *gin.Context) {
		v, err := h.ListKnowledgeGroups()
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/group/:group", func(c *gin.Context) {
		v, err := h.ListKnowledgeByGroup(c.Param("group"))
		unwrap(c, v, err)
	})
	v1.POST("/kdocs/:id/update", func(c *gin.Context) {
		var req domain.KnowledgeDocREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateKnowledgeDoc(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/:id", func(c *gin.Context) {
		v, err := h.GetKnowledgeDoc(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/kdocs/:id/delete", func(c *gin.Context) { Fail(c, h.DeleteKnowledgeDoc(c.Param("id"))) })
	v1.POST("/kdocs/:id/reindex", func(c *gin.Context) { Fail(c, h.ReindexKnowledgeDoc(c.Param("id"))) })

	// 受管文件下载：前端预览 PDF/Word/Excel 用；text/url 类型由前端直接渲染 source 文本。
	v1.GET("/kdocs/:id/file", func(c *gin.Context) {
		data, mime, err := h.GetKnowledgeFile(c.Param("id"))
		if err != nil {
			Fail(c, err)
			return
		}
		c.Data(http.StatusOK, mime, data)
	})

	// ---- memory（一份 MEMORY.md：概览 / 列表 / 检索 / 读写） ----
	v1.GET("/memory", func(c *gin.Context) {
		v, err := h.GetMemoryOverview()
		unwrap(c, v, err)
	})
	v1.GET("/memory/list", func(c *gin.Context) {
		v, err := h.ListMemory(c.Query("section"))
		unwrap(c, v, err)
	})
	v1.GET("/memory/search", func(c *gin.Context) {
		topK := atoi(c.DefaultQuery("k", "20"), 20)
		v, err := h.SearchMemory(c.Query("q"), topK)
		unwrap(c, v, err)
	})
	v1.GET("/memory/text", func(c *gin.Context) {
		v, err := h.GetMemoryText()
		unwrap(c, v, err)
	})
	v1.POST("/memory/append", func(c *gin.Context) {
		var req domain.MemoryWriteREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.AppendMemory(req))
	})
	v1.POST("/memory/delete", func(c *gin.Context) {
		var req domain.MemoryDeleteREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.DeleteMemory(req))
	})
	v1.POST("/memory/replace", func(c *gin.Context) {
		var req domain.MemoryReplaceREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.ReplaceMemory(req))
	})

}
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

	// ---- files (本地受管文件：media/files/workspace) ----
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
func registerProviderRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- ai-provider ----
	v1.GET("/ai-provider", func(c *gin.Context) {
		v, err := h.ListProviders()
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider", func(c *gin.Context) {
		var req domain.AiProviderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreateProvider(req)
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/available", func(c *gin.Context) {
		v, err := h.ListAvailableModels()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/kinds", func(c *gin.Context) {
		v, err := h.ListProviderKinds()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/tiers", func(c *gin.Context) {
		v, err := h.ListProviderTiers()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/presets", func(c *gin.Context) {
		v, err := h.ListProviderPresets()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/:id", func(c *gin.Context) {
		v, err := h.GetProvider(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/update", func(c *gin.Context) {
		var req domain.AiProviderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateProvider(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/delete", func(c *gin.Context) { Fail(c, h.DeleteProvider(c.Param("id"))) })
	v1.POST("/ai-provider/:id/test", func(c *gin.Context) { Fail(c, h.TestProviderConnect(c.Param("id"))) })
	v1.POST("/ai-provider/reload", func(c *gin.Context) {
		// model.json 热重载：重新读文件 → 同步 DB → 重建 Registry
		v, err := h.ReloadProvidersFromFile()
		unwrap(c, v, err)
	})
	v1.GET("/ai-provider/circuit-status", func(c *gin.Context) {
		v, err := h.GetCircuitStatus()
		unwrap(c, v, err)
	})
	v1.POST("/ai-provider/:id/reset-circuit", func(c *gin.Context) {
		v, err := h.ResetCircuit(c.Param("id"))
		unwrap(c, v, err)
	})

	// ---- settings ----
	v1.GET("/settings", func(c *gin.Context) {
		v, err := h.ListSettings()
		unwrap(c, v, err)
	})
	v1.GET("/settings/general", func(c *gin.Context) {
		v, err := h.GetGeneralSettings()
		unwrap(c, v, err)
	})
	v1.POST("/settings/general", func(c *gin.Context) {
		var req map[string]any
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveGeneralSettings(req)
		unwrap(c, v, err)
	})
	v1.GET("/settings/websearch", func(c *gin.Context) {
		v, err := h.GetWebSearchConfig()
		unwrap(c, v, err)
	})
	v1.POST("/settings/websearch", func(c *gin.Context) {
		var req domain.WebSearchConfigRESP
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveWebSearchConfig(req)
		unwrap(c, v, err)
	})
	v1.GET("/settings/exec/agent", func(c *gin.Context) {
		v, err := h.GetExecWhitelist()
		unwrap(c, v, err)
	})
	v1.POST("/settings/exec/agent", func(c *gin.Context) {
		var req []string
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SaveExecWhitelist(req)
		unwrap(c, v, err)
	})
	v1.GET("/kv/:key", func(c *gin.Context) {
		v, err := h.GetSetting(c.Param("key"))
		unwrap(c, v, err)
	})
	v1.POST("/kv/:key", func(c *gin.Context) {
		var req struct {
			Value string `json:"value"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.SetSetting(c.Param("key"), req.Value)
		unwrap(c, v, err)
	})

}
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
func registerWorkspaceRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// 工作目录信任（工作目录信任三态）
	v1.GET("/trust", func(c *gin.Context) {
		v, err := h.ListTrust()
		unwrap(c, v, err)
	})
	v1.GET("/trust/resolve", func(c *gin.Context) {
		v, err := h.ResolveTrust(c.Query("path"))
		unwrap(c, v, err)
	})
	v1.POST("/trust", func(c *gin.Context) {
		var req domain.WorkspaceTrustREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.DecideTrust(req)
		unwrap(c, v, err)
	})
	v1.POST("/trust/revoke", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.RevokeTrust(req.Path))
	})
	v1.GET("/trust/roots", func(c *gin.Context) {
		OK(c, h.TrustRoots())
	})

	// ---- workspace files ----
	v1.GET("/chat/workspace/:id/files", func(c *gin.Context) {
		v, err := h.ListWorkspaceFiles(c.Param("id"))
		unwrap(c, v, err)
	})
	// 懒加载列工作区单层目录（真实磁盘内容；path 相对工作区根）
	v1.GET("/chat/workspace/:id/ls", func(c *gin.Context) {
		v, err := h.ListWorkspaceDir(c.Param("id"), c.Query("path"))
		unwrap(c, v, err)
	})
	v1.GET("/chat/workspace/:id/file", func(c *gin.Context) {
		v, err := h.ReadWorkspaceFile(c.Param("id"), c.Query("path"))
		unwrap(c, v, err)
	})
	// 工作区文件管理：重命名 / 副本 / 删除（沙箱校验；面板右键菜单消费）
	v1.POST("/chat/workspace/:id/rename", func(c *gin.Context) {
		var req struct {
			Path    string `json:"path"`
			NewName string `json:"new_name"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.RenameWorkspaceEntry(c.Param("id"), req.Path, req.NewName)
		unwrap(c, v, err)
	})
	v1.POST("/chat/workspace/:id/copy", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CopyWorkspaceEntry(c.Param("id"), req.Path)
		unwrap(c, v, err)
	})
	v1.POST("/chat/workspace/:id/remove", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.DeleteWorkspaceEntry(c.Param("id"), req.Path)
		unwrap(c, v, err)
	})

	// ---- file changes ----
	v1.GET("/chat/sessions/:id/changes", func(c *gin.Context) {
		limit := atoi(c.DefaultQuery("limit", "100"), 100)
		v, err := h.ListSessionChanges(c.Param("id"), limit)
		unwrap(c, v, err)
	})
	v1.GET("/chat/changes/:cid", func(c *gin.Context) {
		v, err := h.GetSessionChange(c.Param("cid"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/changes/:cid/rollback", func(c *gin.Context) {
		Fail(c, h.RollbackSessionChange(c.Param("cid")))
	})

	// ---- artifacts ----
	v1.GET("/chat/sessions/:id/artifacts", func(c *gin.Context) {
		limit := atoi(c.DefaultQuery("limit", "100"), 100)
		v, err := h.ListSessionArtifacts(c.Param("id"), limit)
		unwrap(c, v, err)
	})
	v1.POST("/chat/artifacts/:aid/delete", func(c *gin.Context) {
		Fail(c, h.DeleteSessionArtifact(c.Param("aid")))
	})

}
