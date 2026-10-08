package service

import (
	"strings"

	"WorkBaby/backend/domain"
)

// SkillService 暴露技能的启停与查看。
type SkillService struct {
	env *Env
}

// NewSkillService 构造技能服务。
func NewSkillService(env *Env) *SkillService { return &SkillService{env: env} }

// List 列出全部技能。
func (s *SkillService) List() []domain.SkillVO {
	out := []domain.SkillVO{}
	if s.env.Skills == nil {
		return out
	}
	for _, sk := range s.env.Skills.List() {
		out = append(out, sk.ToVO())
	}
	return out
}

// Toggle 启停技能，并把停用名单落库——只改内存的话重启就丢了。
func (s *SkillService) Toggle(id string, enabled bool) error {
	if s.env.Skills == nil || !s.env.Skills.SetEnabled(id, enabled) {
		return domain.ErrSkillNotFound
	}
	disabled := s.env.Skills.DisabledNames()
	return s.env.Repo.SetSetting(domain.SettingDisabledSkills, strings.Join(disabled, ","))
}

// Content 读技能正文。
func (s *SkillService) Content(id string) (string, error) {
	if s.env.Skills == nil {
		return "", domain.ErrSkillNotFound
	}
	return s.env.Skills.Content(id)
}
