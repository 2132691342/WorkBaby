package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

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
		hasActive := pkg.Exists(active)
		hasHidden := pkg.Exists(hidden)
		// 两处都在 = 上次搬移中途被杀留下的破损态。清单里没有的技能才走这条，
		// 用户自建的同名技能在两边都有时不该被当成破损态搬走。
		if hasActive && hasHidden {
			s.healDuplicate(item.SkillDirName, active, hidden)
			continue
		}
		if hasActive || hasHidden {
			continue
		}
		target := active
		if !item.SkillDefaultOn {
			target = hidden
		}
		if err := copyTreeAtomic(filepath.Join(s.builtinSource(), item.SkillDirName), target); err != nil {
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

// healDuplicate 自愈「启用区与禁用区同时有同一个技能」的破损态：清单两行同名，一行开一行关。
// 判据用内容：两边完全一致才当成搬移中断的重复份，清掉禁用区那份；
// 内容不一致就是两个不同的东西（用户导入与内置同名），一个都不能动——删错了就是数据没了。
func (s *Skills) healDuplicate(dirName, active, hidden string) {
	activeRaw, activeErr := os.ReadFile(filepath.Join(active, "SKILL.md"))
	hiddenRaw, hiddenErr := os.ReadFile(filepath.Join(hidden, "SKILL.md"))
	if activeErr != nil || hiddenErr != nil || string(activeRaw) != string(hiddenRaw) {
		return
	}
	if err := removeAllQuiet(hidden); err != nil {
		pkg.Warn("技能 %s 破损态自愈失败：%v", dirName, err)
		return
	}
	pkg.Info("技能 %s 破损态自愈：清掉禁用区重复份", dirName)
}

// List 技能总表：启用区 + 禁用区 + 工作区项目技能。
// 禁用区的行 SkillEnabled=false，界面能看见、开关能打开——否则停用等于消失。
func (s *Skills) List() domain.SkillListRESP {
	rows := make([]domain.SkillRowVO, 0, len(BuiltinSkills)*2)
	rows = append(rows, s.scanDir(s.Layout.SkillsDir(), true)...)
	rows = append(rows, s.scanDir(s.Layout.SkillsOffDir(), false)...)
	rows = append(rows, s.scanDir(s.projectDir(), true)...)
	// 同一个目录名在启用区与禁用区都有一份时，只显示启用区那份：
	// 它才是引擎真正看得到的。显示两行会让用户看到同一个技能一开一关。
	rows = dedupeByDirName(rows)
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

// dedupeByDirName 同名技能只留启用区那份；启用区没有才留禁用区那份。
func dedupeByDirName(rows []domain.SkillRowVO) []domain.SkillRowVO {
	active := map[string]bool{}
	for _, row := range rows {
		if row.SkillEnabled {
			active[row.SkillDirName] = true
		}
	}
	out := make([]domain.SkillRowVO, 0, len(rows))
	for _, row := range rows {
		if !row.SkillEnabled && active[row.SkillDirName] {
			continue
		}
		out = append(out, row)
	}
	return out
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
	// 原子写：直接 WriteFile 在写到一半被打断会留下截断的 SKILL.md，
	// 而技能正文是引擎认它的唯一依据，坏了这个技能就废了。
	if err := pkg.WriteFileAtomic(file, []byte(text)); err != nil {
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
	skillDirPath := filepath.Dir(dir)
	skillDir := filepath.Base(skillDirPath)
	// 项目技能归那个仓库管：壳里的开关对它无效，也不该搬动同名内置技能。
	// 之前只取目录名，开掉一个项目技能会把全局同名内置技能搬进禁用区。
	if pkg.Inside(s.projectDir(), skillDirPath) && !pkg.Inside(s.Layout.SkillsDir(), skillDirPath) &&
		!pkg.Inside(s.Layout.SkillsOffDir(), skillDirPath) {
		return pkg.Err(pkg.CodeSkillSave, "项目技能由所在仓库管理，不能在此开关")
	}
	active := filepath.Join(s.Layout.SkillsDir(), skillDir)
	hidden := filepath.Join(s.Layout.SkillsOffDir(), skillDir)
	switch {
	case enabled && pkg.Exists(hidden):
		// 目标已在（破损态里两处都有）：Windows 上 Rename 拒绝覆盖非空目录，
		// 直接搬会一直失败。先把启用区那份合并掉，再搬。
		if pkg.Exists(active) {
			if err := removeAllQuiet(active); err != nil {
				return pkg.WrapErr(pkg.CodeSkillSave, "技能开关失败", err)
			}
		}
		if err := os.Rename(hidden, active); err != nil {
			return pkg.WrapErr(pkg.CodeSkillSave, "技能开关失败", err)
		}
	case !enabled && pkg.Exists(active):
		if pkg.Exists(hidden) {
			if err := removeAllQuiet(hidden); err != nil {
				return pkg.WrapErr(pkg.CodeSkillSave, "技能开关失败", err)
			}
		}
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
	// 传入的是 SKILL.md 文件路径，重置的对象是它所在的技能目录。
	skillDirPath := filepath.Dir(dir)
	skillDir := filepath.Base(skillDirPath)
	original := filepath.Join(s.builtinSource(), skillDir)
	if !pkg.Exists(original) {
		return pkg.Err(pkg.CodeSkillBad, "不是内置技能，无法重置："+skillDir)
	}
	// 重置要保住原来在哪个区：启用态的不能被顺手挪回禁用区。
	wasEnabled := pkg.Inside(s.Layout.SkillsDir(), skillDirPath)
	// 先复制到旁边的临时目录再换名：直接原地覆盖的话，中途失败就只剩半个技能，
	// 而它再也回不到可用状态。
	staging := skillDirPath + ".resetting"
	if err := removeAllQuiet(staging); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能重置失败", err)
	}
	if err := copyTree(original, staging); err != nil {
		_ = removeAllQuiet(staging)
		return pkg.WrapErr(pkg.CodeSkillSave, "技能重置失败", err)
	}
	if err := removeAllQuiet(skillDirPath); err != nil {
		_ = removeAllQuiet(staging)
		return pkg.WrapErr(pkg.CodeSkillSave, "技能重置失败", err)
	}
	if err := os.Rename(staging, skillDirPath); err != nil {
		// 换名失败就把原件搬回去：宁可保持旧内容，也不能让技能处于半残状态。
		if moveErr := os.Rename(staging, skillDirPath); moveErr != nil {
			pkg.Warn("技能重置后换名失败，原件暂存于 %s", staging)
		}
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

// Import 导入一个沙箱外的技能目录为托管技能。导入是复制不是引用：
// 沙箱原则要求技能内容只落在 data 里，源目录之后删改都不影响壳内副本。
func (s *Skills) Import(fromPath string) (domain.SkillImportRESP, error) {
	from := filepath.Clean(strings.TrimSpace(fromPath))
	if from == "" || !pkg.Exists(filepath.Join(from, "SKILL.md")) {
		return domain.SkillImportRESP{}, pkg.Err(pkg.CodeSkillBad, "所选目录没有 SKILL.md，不是有效技能", from)
	}
	name := sanitizeSkillName(filepath.Base(from))
	if name == "" {
		return domain.SkillImportRESP{}, pkg.Err(pkg.CodeSkillBad, "无法从目录名得到技能名："+filepath.Base(from))
	}
	target := filepath.Join(s.Layout.SkillsDir(), name)
	if pkg.Exists(target) || pkg.Exists(filepath.Join(s.Layout.SkillsOffDir(), name)) {
		return domain.SkillImportRESP{}, pkg.Err(pkg.CodeSkillBad, "同名技能已存在："+name)
	}
	if err := copyTreeAtomic(from, target); err != nil {
		return domain.SkillImportRESP{}, pkg.WrapErr(pkg.CodeSkillSave, "技能导入失败", err)
	}
	pkg.Info("技能导入：%s -> %s", from, name)
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{"skill_dir_name": name, "skill_imported": true}))
	return domain.SkillImportRESP{SkillDirName: name, SkillPath: filepath.Join(target, "SKILL.md")}, nil
}

// Create 新建技能：托管目录下一套 SKILL.md。
func (s *Skills) Create(name, description, body string) (string, error) {
	safe := sanitizeSkillName(name)
	if safe == "" {
		return "", pkg.Err(pkg.CodeSkillBad, "技能名只能含字母数字与短横线")
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
	if err := pkg.WriteFileAtomic(file, []byte(content)); err != nil {
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
	// 传入的是 SKILL.md 文件路径，要删的是它所在的技能目录。
	// 只删 SKILL.md 会把 scripts / references 留在沙箱里占盘，且内置技能再也回不来。
	skillDirPath := filepath.Dir(dir)
	skillDir := filepath.Base(skillDirPath)
	// 传进来的不是 SKILL.md 时（目录路径），一律拒掉：
	// 目录路径正好是 skills 根的话，删一次就是清空全部内置技能。
	if filepath.Base(dir) != "SKILL.md" {
		return pkg.Err(pkg.CodeSkillDelete, "只能删除技能文件，不能删除技能目录："+dir)
	}
	if pkg.Inside(s.projectDir(), skillDirPath) && !pkg.Inside(s.Layout.SkillsDir(), skillDirPath) &&
		!pkg.Inside(s.Layout.SkillsOffDir(), skillDirPath) {
		// 项目技能属于那个仓库，删它等于改别人的代码目录。
		return pkg.Err(pkg.CodeSkillDelete, "项目技能请在项目内自行删除")
	}
	if err := removeAllQuiet(skillDirPath); err != nil {
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

// sanitizeSkillName 把任意名字压成合法目录名：保留字母数字（含中文）与短横线。
// 目录名是开关搬移的身份，合法字符集合必须与「新建」「导入」两处共用。
func sanitizeSkillName(name string) string {
	safe := strings.ToLower(strings.TrimSpace(name))
	safe = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-':
			return r
		default:
			return '-'
		}
	}, safe)
	return strings.Trim(safe, "-")
}

// defaultIfEmpty 空值兜底。
func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
