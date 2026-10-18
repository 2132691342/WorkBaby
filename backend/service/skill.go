// Skill 的读取侧与启停：列表、启停（移目录）与正文查看。
// 技能目录就是 PI 的 agent 目录 + 工作区 .pi/skills + 各包自带的 skills。
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

// Reload 重扫全部技能来源；换工作区与导入后都要调它。
func (s *SkillService) Reload(workspace string) {
	if s.env.Skills == nil {
		return
	}
	s.env.Skills.DropSource(domain.SkillSourceGlobal)
	s.env.Skills.DropSource(domain.SkillSourcePackage)
	s.env.Skills.DropSource(domain.SkillSourceWorkspace)

	for _, sk := range skill.LoadDir(s.env.PiEnv.SkillsDir(), domain.SkillSourceGlobal) {
		s.env.Skills.Add(sk)
	}
	for _, sk := range skill.LoadDisabledDir(s.env.PiEnv.DisabledSkillsDir(), domain.SkillSourceGlobal) {
		s.env.Skills.Add(sk)
	}
	// 包自带的技能随包启停，界面只读。
	for _, pkg := range resolvePackages(s.env) {
		if pkg.Path == "" || pkg.Missing {
			continue
		}
		for _, dir := range packageSkills(pkg.Path) {
			for _, sk := range skill.LoadDir(dir, domain.SkillSourcePackage) {
				s.env.Skills.Add(sk)
			}
		}
	}
	s.loadWorkspace(workspace)
}

// loadWorkspace 装入一个工作区的技能：.pi/skills 与 .agents/skills 两个约定位置。
func (s *SkillService) loadWorkspace(workspace string) {
	if s.env.Skills == nil || workspace == "" {
		return
	}
	for _, dir := range []string{
		filepath.Join(workspace, ".pi", "skills"),
		filepath.Join(workspace, ".agents", "skills"),
	} {
		for _, sk := range skill.LoadDir(dir, domain.SkillSourceWorkspace) {
			s.env.Skills.Add(sk)
		}
	}
}

// Toggle 启停全局技能：停用 = 把目录移出 PI 的扫描范围，恢复 = 移回来。
// 工作区与包里的技能是别人的文件，界面不搬，只给一句说明。
func (s *SkillService) Toggle(id string, enabled bool) error {
	if s.env.Skills == nil {
		return domain.ErrSkillNotFound
	}
	sk, ok := s.env.Skills.Get(id)
	if !ok {
		return domain.ErrSkillNotFound
	}
	switch sk.Source {
	case domain.SkillSourceWorkspace:
		return pkg.New("8110", "工作区技能随项目走，请到项目的 .pi/skills 里移动它", "")
	case domain.SkillSourcePackage:
		return pkg.New("8112", "这个技能随插件包安装，请停用那个包", "")
	}
	name := filepath.Base(filepath.Dir(sk.Location))
	if !skill.ValidName(name) {
		return domain.ErrSkillName
	}
	from, to := filepath.Join(s.env.PiEnv.SkillsDir(), name), filepath.Join(s.env.PiEnv.DisabledSkillsDir(), name)
	if enabled {
		from, to = to, from
	}
	if !pkg.DirExists(from) {
		return domain.ErrSkillNotFound
	}
	if pkg.DirExists(to) {
		return pkg.New("8111", "目标位置已有同名技能，先处理掉再启停", name)
	}
	if err := pkg.EnsureDir(filepath.Dir(to)); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return pkg.Wrap("8111", "启停技能失败", err)
	}
	s.Reload(s.env.Workspace())
	return nil
}

// Content 读技能正文。
func (s *SkillService) Content(id string) (string, error) {
	if s.env.Skills == nil {
		return "", domain.ErrSkillNotFound
	}
	return s.env.Skills.Content(id)
}
