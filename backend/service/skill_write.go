package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/skill"
)

// 技能的新建 / 导入 / 删除。
// 只有开关的技能面板不叫「能用」：用户想把自己那套流程固化下来时，
// 必须有一条不依赖命令行、不依赖模型的路径。

// Create 新建一个用户技能，写进数据目录的 skills 下并立刻注册。
func (s *SkillService) Create(req domain.CreateSkillREQ) (*domain.SkillVO, error) {
	if s.env.Skills == nil {
		return nil, domain.ErrSkillNotFound
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, pkg.New(8105, "给这个技能起个名字", "")
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, pkg.New(8105, "技能内容不能为空", "")
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = firstLineOf(body)
	}

	dir := filepath.Join(s.env.Paths.SkillsDir, name)
	if err := pkg.EnsureDir(dir); err != nil {
		return nil, err
	}
	raw := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, desc, body)
	path := filepath.Join(dir, "SKILL.md")
	if err := pkg.WriteText(path, raw); err != nil {
		return nil, err
	}

	parsed, err := skill.Parse(path, domain.SkillSourceGlobal)
	if err != nil {
		return nil, err
	}
	s.env.Skills.Add(parsed)
	vo := parsed.ToVO()
	return &vo, nil
}

// Import 从磁盘导入技能：支持 SKILL.md 文件、含 SKILL.md 的文件夹、
// 以及装满技能的大目录（只取一层）。
func (s *SkillService) Import(paths []string) (*domain.ImportSkillsRESP, error) {
	if s.env.Skills == nil {
		return nil, domain.ErrSkillNotFound
	}
	resp := &domain.ImportSkillsRESP{Skipped: []string{}}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		found := locateSkillFiles(p)
		if len(found) == 0 {
			resp.Skipped = append(resp.Skipped, filepath.Base(p))
			continue
		}
		for _, src := range found {
			name, err := s.copySkill(src)
			if err != nil {
				resp.Skipped = append(resp.Skipped, filepath.Base(src))
				pkg.Warnf("skill: 导入 %s 失败: %v", src, err)
				continue
			}
			resp.Imported++
			if _, ok := s.env.Skills.Get(name); !ok {
				// copySkill 已经把文件放进 skills 目录，重新扫一次即可注册
				for _, sk := range skill.LoadDir(s.env.Paths.SkillsDir, domain.SkillSourceGlobal) {
					s.env.Skills.Add(sk)
				}
			}
		}
	}
	return resp, nil
}

// copySkill 把一个 SKILL.md 复制进数据目录，名字冲突时自动加序号。
func (s *SkillService) copySkill(src string) (string, error) {
	parsed, err := skill.Parse(src, domain.SkillSourceGlobal)
	if err != nil {
		return "", err
	}
	name := parsed.Name
	dir := filepath.Join(s.env.Paths.SkillsDir, name)
	for i := 2; pkg.DirExists(dir); i++ {
		name = fmt.Sprintf("%s-%d", parsed.Name, i)
		dir = filepath.Join(s.env.Paths.SkillsDir, name)
	}
	if err := pkg.EnsureDir(dir); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(dir, "SKILL.md"), string(raw)); err != nil {
		return "", err
	}
	return name, nil
}

// Delete 删除用户技能；内置技能不可删——它们跟着安装包走。
func (s *SkillService) Delete(id string) error {
	if s.env.Skills == nil {
		return domain.ErrSkillNotFound
	}
	sk, ok := s.env.Skills.Get(id)
	if !ok {
		return domain.ErrSkillNotFound
	}
	if sk.Source == domain.SkillSourceBuiltin {
		return pkg.New(8106, "内置技能不能删除，可以先关掉", "")
	}
	// 用 Location 反推目录：导入时同名会自动加序号，重算 skillsDir/name 会指错地方
	if dir := filepath.Dir(sk.Location); dir != "" && dir != "." {
		if err := os.RemoveAll(dir); err != nil {
			return pkg.Wrap(8107, "删除技能失败", err)
		}
	}
	s.env.Skills.Remove(id)
	return nil
}

// locateSkillFiles 定位一个路径下可导入的 SKILL.md（最多一层）。
func locateSkillFiles(path string) []string {
	if !pkg.FileExists(path) && !pkg.DirExists(path) {
		return nil
	}
	if pkg.FileExists(path) {
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return []string{path}
		}
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(path, e.Name(), "SKILL.md")
			if pkg.FileExists(sub) {
				out = append(out, sub)
			}
			continue
		}
		if strings.EqualFold(e.Name(), "SKILL.md") {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

func firstLineOf(s string) string {
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len([]rune(line)) > 60 {
		return string([]rune(line)[:60]) + "…"
	}
	return line
}
