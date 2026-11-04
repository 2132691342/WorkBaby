package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/sse"
)

// Skills 是技能编排：内置清单落位、扫描、开关（移目录）、增删改。
type Skills struct {
	// Settings 设置（项目技能目录要跟着工作区走）
	Settings *Settings
	// Store 开关台账
	Store *repo.Store
	// Layout 沙箱布局
	Layout pkg.Layout
	// Hub 事件广播
	Hub *sse.Hub
}

// NewSkills 造一个技能门面。
func NewSkills(settings *Settings, store *repo.Store, layout pkg.Layout, hub *sse.Hub) *Skills {
	return &Skills{Settings: settings, Store: store, Layout: layout, Hub: hub}
}

// builtinSource 随包技能原件目录。
func (s *Skills) builtinSource() string {
	return filepath.Join(s.Layout.AssetsDir, "skills")
}

// projectDir 工作区的项目技能目录；未选工作区给一个永远不命中的占位路径。
func (s *Skills) projectDir() string {
	if ws := s.Settings.Workspace(); ws != "" {
		return filepath.Join(ws, ".pi", "skills")
	}
	return filepath.Join(s.Layout.PiAgentDir, ".no-project")
}

// Provision 首启把内置清单里的技能落进沙箱：启用态进 skills，停用态直接进禁用区。
// 已存在的目录不动——用户在界面里改过的内容不该被下次启动覆盖。
func (s *Skills) Provision() int {
	_ = pkg.EnsureDir(s.Layout.SkillsDir())
	_ = pkg.EnsureDir(s.Layout.SkillsOffDir())
	copied := 0
	for _, item := range BuiltinSkills {
		active := filepath.Join(s.Layout.SkillsDir(), item.SkillDirName)
		hidden := filepath.Join(s.Layout.SkillsOffDir(), item.SkillDirName)
		if pkg.Exists(active) || pkg.Exists(hidden) {
			continue
		}
		target := active
		if !item.SkillDefaultOn {
			target = hidden
		}
		if err := copyTree(filepath.Join(s.builtinSource(), item.SkillDirName), target); err != nil {
			pkg.Warn("技能包落位失败 %s：%v", item.SkillDirName, err)
			continue
		}
		copied++
	}
	if copied > 0 {
		pkg.Info("内置技能落位：%d 个", copied)
		s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{"provisioned": copied}))
	}
	return copied
}

// List 技能总表：启用区 + 禁用区 + 工作区项目技能。
// 禁用区的行 SkillEnabled=false，界面能看见、开关能打开——否则停用等于消失。
func (s *Skills) List() domain.SkillListRESP {
	rows := make([]domain.SkillRowVO, 0, len(BuiltinSkills)*2)
	rows = append(rows, s.scanDir(s.Layout.SkillsDir(), true)...)
	rows = append(rows, s.scanDir(s.Layout.SkillsOffDir(), false)...)
	rows = append(rows, s.scanDir(s.projectDir(), true)...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SkillGroup != rows[j].SkillGroup {
			return rows[i].SkillGroup < rows[j].SkillGroup
		}
		return rows[i].SkillName < rows[j].SkillName
	})
	present := map[string]struct{}{}
	for _, row := range rows {
		if row.SkillSource == domain.SkillSourceBuiltin || row.SkillSource == domain.SkillSourceBundled {
			present[row.SkillDirName] = struct{}{}
		}
	}
	installed := 0
	for _, item := range BuiltinSkills {
		if _, ok := present[item.SkillDirName]; ok {
			installed++
		}
	}
	return domain.SkillListRESP{
		Skills:           rows,
		BuiltinTotal:     len(BuiltinSkills),
		BuiltinInstalled: installed,
	}
}

// scanDir 扫一个目录成技能行；active 表示这个目录里的技能算启用态。
func (s *Skills) scanDir(root string, active bool) []domain.SkillRowVO {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	projectRoot := filepath.Clean(s.projectDir())
	rows := make([]domain.SkillRowVO, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		skillFile := filepath.Join(dir, "SKILL.md")
		if !pkg.Exists(skillFile) {
			continue
		}
		raw, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		fm := pkg.ParseFrontmatter(string(raw))
		builtin := BuiltinSkillOf(entry.Name())
		row := domain.SkillRowVO{
			SkillName:     defaultIfEmpty(fm.Name, entry.Name()),
			SkillDirName:  entry.Name(),
			SkillDesc:     fm.Description,
			SkillPath:     skillFile,
			SkillDir:      dir,
			SkillSource:   s.sourceOf(entry.Name(), active, root == projectRoot),
			SkillEnabled:  active,
			SkillEditable: root != projectRoot,
			SkillModified: s.isModified(entry.Name(), dir),
		}
		if builtin != nil {
			row.SkillGroup = builtin.SkillGroup
			row.SkillRequires = builtin.SkillRequires
		}
		if row.SkillSource == domain.SkillSourceProject {
			row.SkillEnabled = true
			row.SkillEditable = false
		}
		rows = append(rows, row)
	}
	return rows
}

// sourceOf 决定技能来源：项目 / 内置 / 托管 / 随包附带。
func (s *Skills) sourceOf(dirName string, active, isProject bool) domain.SkillSourceKind {
	if isProject {
		return domain.SkillSourceProject
	}
	if BuiltinSkillOf(dirName) != nil {
		return domain.SkillSourceBuiltin
	}
	// 磁盘上有、清单里没登记的：随包附带。它不享受版本管理，删了不会回来。
	if pkg.Exists(filepath.Join(s.builtinSource(), dirName)) {
		return domain.SkillSourceBundled
	}
	return domain.SkillSourceManaged
}

// isModified 本地副本与随包原件是否一致；非内置技能恒 false。
func (s *Skills) isModified(dirName, localDir string) bool {
	if BuiltinSkillOf(dirName) == nil {
		return false
	}
	original := filepath.Join(s.builtinSource(), dirName, "SKILL.md")
	local := filepath.Join(localDir, "SKILL.md")
	left, err1 := os.ReadFile(original)
	right, err2 := os.ReadFile(local)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(left) != string(right)
}

// guardPath 校验技能路径落在托管 / 禁用 / 项目目录内。
func (s *Skills) guardPath(skillPath string) (string, error) {
	cleaned := filepath.Clean(skillPath)
	for _, root := range []string{s.Layout.SkillsDir(), s.Layout.SkillsOffDir(), s.projectDir()} {
		if pkg.Inside(root, cleaned) {
			return cleaned, nil
		}
	}
	return "", pkg.Err(pkg.CodeSkillBad, "技能路径越界", skillPath)
}

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
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		return domain.SkillContentRESP{}, pkg.WrapErr(pkg.CodeSkillSave, "技能保存失败", err)
	}
	return domain.SkillContentRESP{FileText: text}, nil
}

// Toggle 开关技能：把目录搬进 / 搬出禁用区，引擎自然不再发现它。
// 为什么用搬目录而不是改配置：目录搬移对所有引擎版本都成立，且不需要引擎配合。
func (s *Skills) Toggle(skillPath string, enabled bool) error {
	dir, err := s.guardPath(skillPath)
	if err != nil {
		return err
	}
	skillDir := filepath.Base(filepath.Dir(dir))
	active := filepath.Join(s.Layout.SkillsDir(), skillDir)
	hidden := filepath.Join(s.Layout.SkillsOffDir(), skillDir)
	switch {
	case enabled && pkg.Exists(hidden):
		if err := os.Rename(hidden, active); err != nil {
			return pkg.WrapErr(pkg.CodeSkillSave, "技能开关失败", err)
		}
	case !enabled && pkg.Exists(active):
		if err := os.Rename(active, hidden); err != nil {
			return pkg.WrapErr(pkg.CodeSkillSave, "技能开关失败", err)
		}
	}
	if err := s.Store.SetSkillEnabled(skillDir, enabled); err != nil {
		pkg.Warn("技能开关记账失败：%v", err)
	}
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{
		"skill_dir_name": skillDir,
		"skill_enabled":  enabled,
	}))
	return nil
}

// Reset 把内置技能重置回随包原件（用户改坏了时的恢复口）。
func (s *Skills) Reset(skillPath string) error {
	dir, err := s.guardPath(skillPath)
	if err != nil {
		return err
	}
	skillDir := filepath.Base(filepath.Dir(dir))
	original := filepath.Join(s.builtinSource(), skillDir)
	if !pkg.Exists(original) {
		return pkg.Err(pkg.CodeSkillBad, "不是内置技能，无法重置："+skillDir)
	}
	// 重置要保住原来在哪个区：启用态的不能被顺手挪回禁用区。
	wasEnabled := pkg.Inside(s.Layout.SkillsDir(), dir)
	if err := removeAllQuiet(dir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能重置失败", err)
	}
	if err := copyTree(original, dir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能重置失败", err)
	}
	if err := s.Store.SetSkillEnabled(skillDir, wasEnabled); err != nil {
		pkg.Warn("技能开关记账失败：%v", err)
	}
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{
		"skill_dir_name": skillDir,
		"skill_reset":    true,
	}))
	return nil
}

// Create 新建技能：托管目录下一套 SKILL.md。
func (s *Skills) Create(name, description, body string) (string, error) {
	safe := strings.ToLower(strings.TrimSpace(name))
	safe = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, safe)
	safe = strings.Trim(safe, "-")
	if safe == "" {
		return "", pkg.Err(pkg.CodeSkillBad, "技能名只能是字母数字与短横线")
	}
	dir := filepath.Join(s.Layout.SkillsDir(), safe)
	if pkg.Exists(dir) || pkg.Exists(filepath.Join(s.Layout.SkillsOffDir(), safe)) {
		return "", pkg.Err(pkg.CodeSkillBad, "技能已存在："+safe)
	}
	if err := pkg.EnsureDir(dir); err != nil {
		return "", pkg.WrapErr(pkg.CodeSkillSave, "技能创建失败", err)
	}
	file := filepath.Join(dir, "SKILL.md")
	content := "---\nname: " + safe + "\ndescription: " + defaultIfEmpty(strings.TrimSpace(description), safe) +
		"\n---\n\n" + strings.TrimSpace(body) + "\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return "", pkg.WrapErr(pkg.CodeSkillSave, "技能创建失败", err)
	}
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{"skill_dir_name": safe}))
	return file, nil
}

// Delete 删除技能：只允许删托管与禁用区的。
func (s *Skills) Delete(skillPath string) error {
	dir, err := s.guardPath(skillPath)
	if err != nil {
		return err
	}
	if pkg.Inside(s.projectDir(), dir) && filepath.Clean(dir) != filepath.Clean(s.Layout.SkillsDir()) {
		// 项目技能属于那个仓库，删它等于改别人的代码目录。
		if !pkg.Inside(s.Layout.SkillsDir(), dir) && !pkg.Inside(s.Layout.SkillsOffDir(), dir) {
			return pkg.Err(pkg.CodeSkillDelete, "项目技能请在项目内自行删除")
		}
	}
	skillDir := filepath.Base(filepath.Dir(dir))
	if err := removeAllQuiet(dir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillDelete, "技能删除失败", err)
	}
	if err := s.Store.SetSkillEnabled(skillDir, true); err != nil {
		pkg.Warn("技能开关记账失败：%v", err)
	}
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{
		"skill_dir_name": skillDir,
		"skill_deleted":  true,
	}))
	return nil
}

// Count 已启用的技能数（健康面板用）。
func (s *Skills) Count() int {
	count := 0
	for _, row := range s.List().Skills {
		if row.SkillEnabled {
			count++
		}
	}
	return count
}

// copyTree 递归复制目录；目标已存在时合并。
func copyTree(from, to string) error {
	if err := pkg.EnsureDir(to); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(from, entry.Name())
		dst := filepath.Join(to, entry.Name())
		if entry.IsDir() {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			continue
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// defaultIfEmpty 空值兜底。
func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
