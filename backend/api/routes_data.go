package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// sessionsList 会话清单；q 非空时按标题与正文过滤。
func (s *Server) sessionsList(c *gin.Context) {
	pkg.OK(c, s.svc.Sessions.List(strings.TrimSpace(c.Query("workspace")), c.Query("q")))
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

// modelsList 可选模型平铺（对话页模型下拉）。
func (s *Server) modelsList(c *gin.Context) {
	pkg.OK(c, s.svc.Models.Options())
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

// providerModelSave 单条模型增改：按 ModelId 定位，存在即改。
func (s *Server) providerModelSave(c *gin.Context) {
	var req domain.ProviderModelSaveREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Models.SaveModel(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// providerModelDelete 删除服务商下的一条模型。
func (s *Server) providerModelDelete(c *gin.Context) {
	var req domain.ProviderModelDeleteREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Models.DeleteModel(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// modelDefaults 读新会话的起步设置：默认服务商、默认模型与默认思考档。
func (s *Server) modelDefaults(c *gin.Context) {
	pkg.OK(c, s.svc.Models.Defaults())
}

// modelDefaultsSave 写新会话的起步设置。
func (s *Server) modelDefaultsSave(c *gin.Context) {
	var req domain.ModelDefaultSaveREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Models.SaveDefaults(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// skillsList 技能总表。
func (s *Server) skillsList(c *gin.Context) {
	pkg.OK(c, s.svc.Skills.List())
}

// skillInstall 把一条推荐技能装进用户的技能区。
func (s *Server) skillInstall(c *gin.Context) {
	var req domain.SkillInstallREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Skills.Install(req.SkillDirName); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
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

// skillImport 导入一个数据目录外的技能目录。
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
