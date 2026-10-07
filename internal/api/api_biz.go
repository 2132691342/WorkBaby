// 业务 HTTP handler：只做参数解析与响应组装，编排一律下沉到 service 层。
package api

import (
	"strconv"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"

	"github.com/gin-gonic/gin"
)

// ok 统一成功响应。
func ok(c *gin.Context, data any) {
	c.JSON(200, domain.Resp{Code: 0, Data: data})
}

// fail 统一失败响应：code 取自 AppError，前端据此分流。
func fail(c *gin.Context, err error) {
	c.JSON(200, domain.Resp{Code: pkg.CodeOf(err), Message: err.Error()})
}

// Health 存活探测。
func (h *Handler) Health(c *gin.Context) { ok(c, domain.HealthRESP{OK: true}) }

// Bootstrap 启动引导数据。
func (h *Handler) Bootstrap(c *gin.Context) {
	settings, err := h.Svc.Settings.All()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.BootstrapVO{
		Version:           h.Version,
		ContractVersion:   domain.ContractVersion,
		DefaultProviderID: settings[domain.SettingDefaultProvider],
		DefaultModel:      settings[domain.SettingDefaultModel],
		Workspace:         h.Cfg.Workspace,
		Permission:        settings[domain.SettingPermission],
		PythonReady:       runtimeReady(h.Paths),
		Settings:          settings,
	})
}

// ListSessions 会话列表。
func (h *Handler) ListSessions(c *gin.Context) {
	list, err := h.Svc.Sessions.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// CreateSession 新建会话。
func (h *Handler) CreateSession(c *gin.Context) {
	var req domain.CreateSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Sessions.Create(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GetSession 会话详情。
func (h *Handler) GetSession(c *gin.Context) {
	detail, err := h.Svc.Sessions.Detail(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, detail)
}

// RenameSession 重命名会话。
func (h *Handler) RenameSession(c *gin.Context) {
	var req domain.RenameSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.Rename(c.Param("id"), req.Title); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionWorkspace 切换会话工作目录：改完下一条消息立即生效。
func (h *Handler) SetSessionWorkspace(c *gin.Context) {
	var req domain.SetWorkspaceREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetWorkspace(c.Param("id"), req.Workspace); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// DeleteSession 删除会话。
func (h *Handler) DeleteSession(c *gin.Context) {
	if err := h.Svc.Sessions.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionModel 切换会话模型。
func (h *Handler) SetSessionModel(c *gin.Context) {
	var req domain.SetModelREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetModel(c.Param("id"), req.ProviderID, req.Model); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionPermission 切换会话权限档。
func (h *Handler) SetSessionPermission(c *gin.Context) {
	var req domain.SetPermissionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetPermission(c.Param("id"), req.Permission); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// BranchSession 从某条历史回溯。
func (h *Handler) BranchSession(c *gin.Context) {
	var req domain.BranchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.Branch(c.Param("id"), req.EntryID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SendMessage 发消息：立即返回 runID，后续经 SSE 推送。
func (h *Handler) SendMessage(c *gin.Context) {
	var req domain.SendMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Chat.Send(req.SessionID, req.Content, req.Attachments)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// StopRun 停止当前 run。
func (h *Handler) StopRun(c *gin.Context) {
	var req domain.StopRunREQ
	_ = c.ShouldBindJSON(&req)
	if req.SessionID == "" {
		req.SessionID = c.Query("session_id")
	}
	if err := h.Svc.Chat.Stop(req.SessionID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SteerMessage 插话。
func (h *Handler) SteerMessage(c *gin.Context) {
	var req domain.QueueMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.Steer(req.SessionID, req.Content); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// FollowUpMessage 排队。
func (h *Handler) FollowUpMessage(c *gin.Context) {
	var req domain.QueueMessageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Chat.FollowUp(req.SessionID, req.Content); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListApprovals 待决审批，供跨重启恢复决策卡。
func (h *Handler) ListApprovals(c *gin.Context) {
	list, err := h.Svc.Approvals.Pending(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// DecideApproval 审批决策；approve / deny 共用，靠动作区分。
func (h *Handler) DecideApproval(c *gin.Context) {
	var req domain.DecideApprovalREQ
	_ = c.ShouldBindJSON(&req)
	approved := c.Param("action") == "approve"
	if err := h.Svc.Approvals.Decide(c.Param("id"), approved, req.Scope); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListProviders 模型服务列表。
func (h *Handler) ListProviders(c *gin.Context) {
	list, err := h.Svc.Providers.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// UpsertProvider 新增或更新模型服务。
func (h *Handler) UpsertProvider(c *gin.Context) {
	var req domain.UpsertProviderREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Providers.Upsert(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// UpdateProvider 更新指定模型服务。
func (h *Handler) UpdateProvider(c *gin.Context) {
	var req domain.UpsertProviderREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	req.ID = c.Param("id")
	vo, err := h.Svc.Providers.Upsert(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// DeleteProvider 删除模型服务。
func (h *Handler) DeleteProvider(c *gin.Context) {
	if err := h.Svc.Providers.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// TestProvider 连通测试。
func (h *Handler) TestProvider(c *gin.Context) {
	var req domain.TestProviderREQ
	_ = c.ShouldBindJSON(&req)
	id := req.ID
	if id == "" {
		id = c.Param("id")
	}
	resp, err := h.Svc.Providers.Test(id)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// SetDefaultProvider 设为默认。
func (h *Handler) SetDefaultProvider(c *gin.Context) {
	if err := h.Svc.Providers.SetDefault(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListModels 拉取上游模型列表。
func (h *Handler) ListModels(c *gin.Context) {
	models, err := h.Svc.Providers.Models(c.Query("provider_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, models)
}

// ListSkills 技能列表。
func (h *Handler) ListSkills(c *gin.Context) { ok(c, h.Svc.Skills.List()) }

// ToggleSkill 启停技能。
func (h *Handler) ToggleSkill(c *gin.Context) {
	var req domain.ToggleSkillREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Skills.Toggle(c.Param("id"), req.Enabled); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SkillContent 读技能正文。
func (h *Handler) SkillContent(c *gin.Context) {
	body, err := h.Svc.Skills.Content(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.SkillContentRESP{Content: body})
}

// CreateSkill 新建用户技能。
func (h *Handler) CreateSkill(c *gin.Context) {
	var req domain.CreateSkillREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Skills.Create(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ImportSkills 从磁盘导入技能。
func (h *Handler) ImportSkills(c *gin.Context) {
	var req domain.ImportSkillsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Skills.Import(req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// DeleteSkill 删除用户技能。
func (h *Handler) DeleteSkill(c *gin.Context) {
	if err := h.Svc.Skills.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListDocs 知识库文档列表。
func (h *Handler) ListDocs(c *gin.Context) {
	list, err := h.Svc.Knowledge.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// AddDocs 添加文档。
func (h *Handler) AddDocs(c *gin.Context) {
	var req domain.AddDocsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	added, err := h.Svc.Knowledge.Add(req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.AddDocsRESP{Added: added})
}

// DeleteDoc 删除文档。
func (h *Handler) DeleteDoc(c *gin.Context) {
	if err := h.Svc.Knowledge.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ReindexDocs 全量重建索引。
func (h *Handler) ReindexDocs(c *gin.Context) {
	n, err := h.Svc.Knowledge.Reindex()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.ReindexRESP{Reindexed: n})
}

// SearchKnowledge 检索知识库。
func (h *Handler) SearchKnowledge(c *gin.Context) {
	var req domain.SearchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	hits, err := h.Svc.Knowledge.Search(c.Request.Context(), req.Query, req.Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.SearchRESP{Hits: hits})
}

// ListTools 工具清单：助手当前能干什么、风险多大、是否启用。
func (h *Handler) ListTools(c *gin.Context) { ok(c, h.Svc.Tools.Catalog()) }

// ToggleTool 启停单个工具。
func (h *Handler) ToggleTool(c *gin.Context) {
	var req domain.ToggleToolREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Tools.SetEnabled(c.Param("name"), req.Enabled); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ModelCapability 查模型能力：上下文窗口、是否支持思考、是否识图。
func (h *Handler) ModelCapability(c *gin.Context) {
	model := c.Query("model")
	if model == "" {
		fail(c, pkg.New(1105, "请指定要查询的模型", ""))
		return
	}
	ok(c, h.Svc.Chat.ModelCapability(c.Query("provider_id"), model))
}

// GetModelConfig 查单个模型配置（目录 + 覆写合并后的最终值）。
func (h *Handler) GetModelConfig(c *gin.Context) {
	model := c.Query("model")
	if model == "" {
		fail(c, pkg.New(1105, "请指定要查询的模型", ""))
		return
	}
	vo, err := h.Svc.Providers.ModelConfig(c.Query("provider_id"), model)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ListModelConfigs 列出一个服务下的全部模型配置。
func (h *Handler) ListModelConfigs(c *gin.Context) {
	list, err := h.Svc.Providers.ListModelConfigs(c.Query("provider_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// UpsertModelConfig 保存模型配置。
func (h *Handler) UpsertModelConfig(c *gin.Context) {
	var req domain.UpsertModelConfigREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Providers.UpsertModelConfig(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// FetchModels 用未保存的连接信息拉上游模型列表（新增服务场景）。
func (h *Handler) FetchModels(c *gin.Context) {
	var req domain.FetchModelsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if req.API == "" {
		req.API = domain.APIOpenAI
	}
	models, err := h.Svc.Providers.FetchModels(req.API, req.BaseURL, req.APIKey)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, models)
}

// RuntimeStatus 报告内置运行时是否就绪，附带失败原因。
func (h *Handler) RuntimeStatus(c *gin.Context) { ok(c, runtimeInfo(h.Paths)) }

// runtimeInfo 把运行时探测结果整成界面能直接显示的形状。
func runtimeInfo(p runtime.Paths) domain.RuntimeInfoVO {
	st := runtime.Status(p)
	return domain.RuntimeInfoVO{
		PythonExe:     st.Exe,
		PythonSource:  st.Source,
		PythonVersion: st.Version,
		PythonError:   st.Err,
		ArchivePath:   runtime.ArchivePath(p),
	}
}

// AllSettings 全部设置。
func (h *Handler) AllSettings(c *gin.Context) {
	all, err := h.Svc.Settings.All()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, all)
}

// SetSetting 写入设置。
func (h *Handler) SetSetting(c *gin.Context) {
	var req domain.SetSettingREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap(1107, "请求格式不正确", err))
		return
	}
	if err := h.Svc.Settings.Set(req.Key, req.Value); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// Stats 仪表盘统计：token 用量按天 / 按模型 / 按会话。
func (h *Handler) Stats(c *gin.Context) {
	days := 0
	if v := c.Query("days"); v != "" {
		days, _ = strconv.Atoi(v)
	}
	resp, err := h.Svc.Settings.Stats(days)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// runtimeReady 报告内置 Python 是否可用。
func runtimeReady(p runtime.Paths) bool { return runtime.PythonExe(p) != "" }
