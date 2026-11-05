package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// sessionsList 会话清单。
func (s *Server) sessionsList(c *gin.Context) {
	pkg.OK(c, s.svc.Sessions.List(strings.TrimSpace(c.Query("workspace"))))
}

// sessionDetail 会话详情。
func (s *Server) sessionDetail(c *gin.Context) {
	out, err := s.svc.Sessions.Detail(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// sessionDelete 删除会话文件。
func (s *Server) sessionDelete(c *gin.Context) {
	var req domain.SessionPathREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Sessions.Delete(req.SessionPath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// sessionRename 会话改名。
func (s *Server) sessionRename(c *gin.Context) {
	var req domain.SessionRenameREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Sessions.Rename(req.SessionPath, req.SessionTitle); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// providersGet 服务商清单（含掩码密钥）。
func (s *Server) providersGet(c *gin.Context) {
	pkg.OK(c, s.svc.Models.Providers())
}

// providerUpsert 保存服务商。
func (s *Server) providerUpsert(c *gin.Context) {
	var req domain.ProviderUpsertREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Models.Upsert(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// providerDelete 删除服务商。
func (s *Server) providerDelete(c *gin.Context) {
	var req domain.ProviderDeleteREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Models.Delete(req.ProviderName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// providerProbe 向上游问一次模型清单，顺带就是连通性测试。
func (s *Server) providerProbe(c *gin.Context) {
	var req domain.ProviderProbeREQ
	if !bind(c, &req) {
		return
	}
	pkg.OK(c, s.svc.Models.Probe(req))
}

// skillsList 技能总表。
func (s *Server) skillsList(c *gin.Context) {
	pkg.OK(c, s.svc.Skills.List())
}

// skillContent 读技能正文。
func (s *Server) skillContent(c *gin.Context) {
	out, err := s.svc.Skills.Content(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// skillSave 保存技能正文。
func (s *Server) skillSave(c *gin.Context) {
	var req domain.SkillSaveREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Skills.Save(req.SkillPath, req.FileText)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// skillToggle 技能开关。
func (s *Server) skillToggle(c *gin.Context) {
	var req domain.SkillToggleREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Skills.Toggle(req.SkillPath, req.SkillEnabled); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// skillCreate 新建技能。
func (s *Server) skillCreate(c *gin.Context) {
	var req domain.SkillCreateREQ
	if !bind(c, &req) {
		return
	}
	path, err := s.svc.Skills.Create(req.SkillName, req.SkillDesc, req.SkillBody)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"skill_path": path})
}

// skillImport 导入一个沙箱外的技能目录。
func (s *Server) skillImport(c *gin.Context) {
	var req domain.SkillImportREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Skills.Import(req.SkillFromPath)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// skillDelete 删除技能。
func (s *Server) skillDelete(c *gin.Context) {
	var req domain.SkillPathREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Skills.Delete(req.SkillPath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// skillReset 内置技能重置回随包原件。
func (s *Server) skillReset(c *gin.Context) {
	var req domain.SkillPathREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Skills.Reset(req.SkillPath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// pluginsList 插件清单。
func (s *Server) pluginsList(c *gin.Context) {
	pkg.OK(c, s.svc.Plugins.List())
}

// pluginInstall 装一个插件来源。
func (s *Server) pluginInstall(c *gin.Context) {
	var req domain.PluginSourceREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Plugins.Install(req.PluginSource)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginUninstall 卸载插件。
func (s *Server) pluginUninstall(c *gin.Context) {
	var req domain.PluginNameREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Plugins.Uninstall(req.PluginName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginToggle 插件启停。
func (s *Server) pluginToggle(c *gin.Context) {
	var req domain.PluginToggleREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Plugins.Toggle(req.PluginName, req.PluginEnabled)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginUpgrade 插件升级。
func (s *Server) pluginUpgrade(c *gin.Context) {
	var req domain.PluginNameREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Plugins.Upgrade(req.PluginName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginRetry 重试单个内置插件。
func (s *Server) pluginRetry(c *gin.Context) {
	var req domain.PluginNameREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Plugins.Retry(req.PluginName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginRetryAll 全部重试。
func (s *Server) pluginRetryAll(c *gin.Context) {
	out, err := s.svc.Plugins.RetryAll()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginUpgradeAll 批量升级：只升版本与清单不符的内置插件。
func (s *Server) pluginUpgradeAll(c *gin.Context) {
	out, err := s.svc.Plugins.UpgradeAll()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// pluginCheckUpdates 问 npm registry 各插件的最新版并回新清单。
func (s *Server) pluginCheckUpdates(c *gin.Context) {
	out, err := s.svc.Plugins.CheckUpdates()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// mcpList MCP 清单。
func (s *Server) mcpList(c *gin.Context) {
	pkg.OK(c, s.svc.Mcp.List())
}

// mcpSave 保存 MCP 服务。
func (s *Server) mcpSave(c *gin.Context) {
	var req domain.McpSaveREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Mcp.Save(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// mcpToggle MCP 开关。
func (s *Server) mcpToggle(c *gin.Context) {
	var req domain.McpToggleREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Mcp.Toggle(req.ServerName, req.McpEnabled)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// mcpProbe MCP 连接测试：完成一次 initialize 握手。
func (s *Server) mcpProbe(c *gin.Context) {
	var req domain.McpNameREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Mcp.Probe(req.ServerName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// mcpRemove 删除 MCP 服务。
func (s *Server) mcpRemove(c *gin.Context) {
	var req domain.McpNameREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Mcp.Remove(req.ServerName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// filesList 列目录。
func (s *Server) filesList(c *gin.Context) {
	out, err := s.svc.Files.List(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// filesSearch 按名搜索工作区。
func (s *Server) filesSearch(c *gin.Context) {
	out, err := s.svc.Files.Search(c.Query("query"), queryInt(c, "limit", 50))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// fileRead 读文本文件。
func (s *Server) fileRead(c *gin.Context) {
	out, err := s.svc.Files.Read(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}
