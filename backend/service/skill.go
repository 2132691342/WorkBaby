// Skill 的读取侧与启停：列表、启停（移目录）与正文查看（specs/03）。
package service

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/skill"
)

// SkillService 暴露技能的启停与查看。
type SkillService struct {
	env *Env
}

// NewSkillService 构造技能服务。
func NewSkillService(env *Env) *SkillService { return &SkillService{env: env} }

// List 列出全部技能（含停用区——它们只是不被模型看见，界面仍要列得出）。
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

// Toggle 启停全局技能：停用 = 把目录移出 PI 的扫描范围，恢复 = 移回来（specs/03）。
// 工作区技能是用户项目里的文件，界面不搬。
func (s *SkillService) Toggle(id string, enabled bool) error {
	if s.env.Skills == nil {
		return domain.ErrSkillNotFound
	}
	sk, ok := s.env.Skills.Get(id)
	if !ok {
		return domain.ErrSkillNotFound
	}
	if sk.Source == domain.SkillSourceWorkspace {
		return pkg.New("8110", "工作区技能随项目走，请到项目目录里移动它的文件夹", "")
	}
	name := filepath.Base(filepath.Dir(sk.Location))
	if !skill.ValidName(name) {
		return domain.ErrSkillName
	}
	from, to := filepath.Join(s.env.Paths.SkillsDir, name), filepath.Join(s.env.Paths.DisabledSkillsDir, name)
	if enabled {
		from, to = to, from
	}
	if !pkg.DirExists(from) {
		return domain.ErrSkillNotFound
	}
	if pkg.DirExists(to) {
		return pkg.New("8111", "目标位置已有同名技能，先处理掉再启停", name)
	}
	if err := os.Rename(from, to); err != nil {
		return pkg.Wrap("8111", "启停技能失败", err)
	}
	s.reloadGlobal()
	return nil
}

// reloadGlobal 重扫全局的两区目录：移完目录后界面与真相一致。
func (s *SkillService) reloadGlobal() {
	s.env.Skills.DropSource(domain.SkillSourceGlobal)
	for _, sk := range skill.LoadDir(s.env.Paths.SkillsDir, domain.SkillSourceGlobal) {
		s.env.Skills.Add(sk)
	}
	for _, sk := range skill.LoadDisabledDir(s.env.Paths.DisabledSkillsDir, domain.SkillSourceGlobal) {
		s.env.Skills.Add(sk)
	}
}

// Content 读技能正文。
func (s *SkillService) Content(id string) (string, error) {
	if s.env.Skills == nil {
		return "", domain.ErrSkillNotFound
	}
	return s.env.Skills.Content(id)
}
