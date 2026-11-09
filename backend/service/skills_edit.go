package service

// 技能正文的读写：读内容、保存内容。
// 与 skills.go 分开是因为「动一个文件的字」和「搬一个目录」是两种风险：
// 前者写坏了技能就没了，后者搬错了目录就影响了另一个技能。

import (
	"os"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Content 读技能正文。
func (s *Skills) Content(skillPath string) (domain.SkillContentRESP, error) {
	file, err := s.guardPath(skillPath)
	if err != nil {
		return domain.SkillContentRESP{}, err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return domain.SkillContentRESP{}, pkg.WrapErr(pkg.CodeSkillBad, "技能读取失败", err)
	}
	return domain.SkillContentRESP{FileText: string(raw)}, nil
}

// Save 保存技能正文。
func (s *Skills) Save(skillPath, text string) (domain.SkillContentRESP, error) {
	file, err := s.guardPath(skillPath)
	if err != nil {
		return domain.SkillContentRESP{}, err
	}
	// 原子写：直接 WriteFile 在写到一半被打断会留下截断的 SKILL.md，
	// 而技能正文是引擎认它的唯一依据，坏了这个技能就废了。
	if err := pkg.WriteFileAtomic(file, []byte(text)); err != nil {
		return domain.SkillContentRESP{}, pkg.WrapErr(pkg.CodeSkillSave, "技能保存失败", err)
	}
	return domain.SkillContentRESP{FileText: text}, nil
}
