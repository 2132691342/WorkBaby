// 技能 handler：列表 / 建 / 启停 / 读正文 / 磁盘与压缩包导入 / 删。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"encoding/base64"
	"github.com/gin-gonic/gin"
)

// ListSkills 技能列表。
func (h *Handler) ListSkills(c *gin.Context) { ok(c, h.Svc.Skills.List()) }

// ToggleSkill 启停技能。
func (h *Handler) ToggleSkill(c *gin.Context) {
	var req domain.ToggleSkillREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
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
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
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
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Skills.Import(req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// ImportSkillZip 导入技能压缩包：Data 是 zip 原始字节的 base64。
func (h *Handler) ImportSkillZip(c *gin.Context) {
	var req domain.ImportSkillZipREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		fail(c, pkg.Wrap("1107", "技能包数据不是合法的 base64", err))
		return
	}
	if len(raw) == 0 {
		fail(c, pkg.New("1109", "技能包内容为空", req.Filename))
		return
	}
	resp, err := h.Svc.Skills.ImportZip(req.Filename, raw)
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
