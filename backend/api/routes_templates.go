package api

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// templatesList 提示词模板总表。
func (s *Server) templatesList(c *gin.Context) {
	pkg.OK(c, s.svc.Templates.List())
}

// templateSave 新建或覆盖一份模板。
// 回清洗后的名字：界面拿原文再删会找不到刚存下的那份。
func (s *Server) templateSave(c *gin.Context) {
	var req domain.PromptTemplateREQ
	if !bind(c, &req) {
		return
	}
	name, err := s.svc.Templates.Save(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, domain.PromptTemplateSaveRESP{Ok: true, TemplateName: name})
}

// templateDelete 删一份模板。
func (s *Server) templateDelete(c *gin.Context) {
	var req domain.PromptTemplateREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Templates.Delete(strings.TrimSpace(req.TemplateName)); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}
