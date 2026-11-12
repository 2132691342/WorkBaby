package service

import (
	"os"
	"path/filepath"
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
	// Layout 目录布局
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
	return filepath.Join(s.Layout.DataDir, ".no-project")
}

// Install 把一个内置技能装进用户的技能区；已存在就不动。
// 壳不自动落位：技能目录要用户亲自点了才写进去。
func (s *Skills) Install(dirName string) error {
	item := RecommendedSkillOf(dirName)
	if item == nil {
		return pkg.Err(pkg.CodeSkillBad, "不认识的推荐技能："+dirName)
	}
	if err := pkg.EnsureDir(s.Layout.SkillsDir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能目录不可写", err)
	}
	target := filepath.Join(s.Layout.SkillsDir, dirName)
	if pkg.Exists(target) {
		return pkg.Err(pkg.CodeSkillBad, "技能已存在："+dirName)
	}
	if err := copyTreeAtomic(filepath.Join(s.builtinSource(), dirName), target); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能安装失败", err)
	}
	s.Hub.Broadcast("skill:status", sse.EngineTag(map[string]any{"skill_dir_name": dirName}))
	return nil
}

// guardPath 校验技能路径落在托管 / 禁用 / 项目目录内。
func (s *Skills) guardPath(skillPath string) (string, error) {
	cleaned := filepath.Clean(skillPath)
	for _, root := range []string{s.Layout.SkillsDir, s.Layout.SkillsOffDir, s.projectDir()} {
		if pkg.Inside(root, cleaned) {
			return cleaned, nil
		}
	}
	return "", pkg.Err(pkg.CodeSkillBad, "技能路径越界", skillPath)
}

// Toggle 开关技能：把目录搬进 / 搬出禁用区，引擎自然不再发现它。
// 为什么用搬目录而不是改配置：目录搬移对所有引擎版本都成立，且不需要引擎配合。
func (s *Skills) Toggle(skillPath string, enabled bool) error {
	dir, err := s.guardPath(skillPath)
	if err != nil {
		return err
	}
	// 传入的必须是 SKILL.md 文件路径：开关的对象是它所在的技能目录，
	// 深层任意文件算出的目录会把别的技能搬走。
	if filepath.Base(dir) != "SKILL.md" {
		return pkg.Err(pkg.CodeBadRequest, "技能路径必须指向 SKILL.md 文件")
	}
	skillDirPath := filepath.Dir(dir)
	skillDir := filepath.Base(skillDirPath)
	// 项目技能归那个仓库管：壳里的开关对它无效。开关对象按传入的 SKILL.md
	// 定位技能目录——只取目录名会把项目技能误认成同名内置技能，殃及无辜。
	if pkg.Inside(s.projectDir(), skillDirPath) && !pkg.Inside(s.Layout.SkillsDir, skillDirPath) &&
		!pkg.Inside(s.Layout.SkillsOffDir, skillDirPath) {
		return pkg.Err(pkg.CodeSkillSave, "项目技能由所在仓库管理，不能在此开关")
	}
	active := filepath.Join(s.Layout.SkillsDir, skillDir)
	hidden := filepath.Join(s.Layout.SkillsOffDir, skillDir)
	// 两个区都可能因为用户自己删过而不存在：搬移前先确保目标父目录在，
	// 否则 Rename 会死在「找不到路径」上而不是真的搬不动。
	if err := pkg.EnsureDir(s.Layout.SkillsDir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能目录不可写", err)
	}
	if err := pkg.EnsureDir(s.Layout.SkillsOffDir); err != nil {
		return pkg.WrapErr(pkg.CodeSkillSave, "技能禁用目录不可写", err)
	}
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
	wasEnabled := pkg.Inside(s.Layout.SkillsDir, skillDirPath)
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

// Import 导入一个数据目录外的技能目录为托管技能。导入是复制不是引用：
// 技能内容只落在 data 里，源目录之后删改都不影响壳内副本。
func (s *Skills) Import(fromPath string) (domain.SkillImportRESP, error) {
	from := filepath.Clean(strings.TrimSpace(fromPath))
	if from == "" || !pkg.Exists(filepath.Join(from, "SKILL.md")) {
		return domain.SkillImportRESP{}, pkg.Err(pkg.CodeSkillBad, "所选目录没有 SKILL.md，不是有效技能", from)
	}
	name := sanitizeSkillName(filepath.Base(from))
	if name == "" {
		return domain.SkillImportRESP{}, pkg.Err(pkg.CodeSkillBad, "无法从目录名得到技能名："+filepath.Base(from))
	}
	target := filepath.Join(s.Layout.SkillsDir, name)
	if pkg.Exists(target) || pkg.Exists(filepath.Join(s.Layout.SkillsOffDir, name)) {
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
	dir := filepath.Join(s.Layout.SkillsDir, safe)
	if pkg.Exists(dir) || pkg.Exists(filepath.Join(s.Layout.SkillsOffDir, safe)) {
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
	// 只删 SKILL.md 会把 scripts / references 留在数据目录里占盘，且内置技能再也回不来。
	skillDirPath := filepath.Dir(dir)
	skillDir := filepath.Base(skillDirPath)
	// 传进来的不是 SKILL.md 时（目录路径），一律拒掉：
	// 目录路径正好是 skills 根的话，删一次就是清空全部内置技能。
	if filepath.Base(dir) != "SKILL.md" {
		return pkg.Err(pkg.CodeSkillDelete, "只能删除技能文件，不能删除技能目录："+dir)
	}
	if pkg.Inside(s.projectDir(), skillDirPath) && !pkg.Inside(s.Layout.SkillsDir, skillDirPath) &&
		!pkg.Inside(s.Layout.SkillsOffDir, skillDirPath) {
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
