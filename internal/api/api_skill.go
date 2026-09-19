package api

import (
	"WorkBaby/internal/domain"
)

// ListSkills 返回全部 Skill（含 disabled），供技能设置页展示。
func (h *Handler) ListSkills() ([]domain.SkillRESP, error) {
	return h.skillSvc.List(h.ctx)
}

// SetSkillEnabled 启停 Skill。
func (h *Handler) SetSkillEnabled(name string, enabled bool) error {
	return h.skillSvc.SetEnabled(h.ctx, name, enabled)
}

// ImportSkill 导入自定义 Skill（UI 编辑；同名覆盖）。
func (h *Handler) ImportSkill(req domain.SkillREQ) (domain.SkillRESP, error) {
	p, err := h.skillSvc.ImportCustom(h.ctx, &req)
	if err != nil {
		return domain.SkillRESP{}, err
	}
	return *p, nil
}

// UpdateSkill 更新自定义 Skill 内容（内置 Skill 拒绝）。
func (h *Handler) UpdateSkill(name string, req domain.SkillREQ) (domain.SkillRESP, error) {
	p, err := h.skillSvc.UpdateCustom(h.ctx, name, &req)
	if err != nil {
		return domain.SkillRESP{}, err
	}
	return *p, nil
}

// DeleteSkill 删除 Skill（内置 Skill 拒绝）。
func (h *Handler) DeleteSkill(name string) error {
	return h.skillSvc.Delete(h.ctx, name)
}

// ImportSkillsZip 批量导入技能包 zip（zipPath 由原生文件对话框提供）；
// 返回 { imported: []string, skipped: []string, failed: [{path, error}] }。
func (h *Handler) ImportSkillsZip(zipPath string) (map[string]any, error) {
	return h.skillSvc.ImportZip(h.ctx, zipPath)
}
