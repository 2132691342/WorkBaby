package service

// 技能扫描：把三个技能区读成界面能用的行。
// 与 skills.go 分开是因为它只读不写——落位、搬移、删改都在那边，
// 这边只负责「现在有什么、每个是什么来路」。

import (
	"os"
	"path/filepath"
	"sort"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// List 技能总表：启用区 + 禁用区 + 工作区项目技能。
// 禁用区的行 SkillEnabled=false，界面能看见、开关能打开——否则停用等于消失。
func (s *Skills) List() domain.SkillListRESP {
	rows := make([]domain.SkillRowVO, 0, len(RecommendedSkills)*2)
	rows = append(rows, s.scanDir(s.Layout.SkillsDir, true)...)
	rows = append(rows, s.scanDir(s.Layout.SkillsOffDir, false)...)
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
	// 推荐清单里没装的要单独列出来：它们不在任何磁盘目录里，
	// 不列就只剩「怎么都找不到这个技能」的困惑。
	available := make([]domain.SkillAvailableVO, 0, len(RecommendedSkills))
	for _, item := range RecommendedSkills {
		if _, ok := present[item.SkillDirName]; ok {
			continue
		}
		available = append(available, domain.SkillAvailableVO{
			SkillDirName:  item.SkillDirName,
			SkillGroup:    item.SkillGroup,
			SkillDesc:     skillDescOf(s.builtinSource(), item.SkillDirName),
			SkillRequires: item.SkillRequires,
		})
	}
	return domain.SkillListRESP{
		Skills:               rows,
		RecommendedTotal:     len(RecommendedSkills),
		RecommendedInstalled: len(RecommendedSkills) - len(available),
		Available:            available,
	}
}

// skillDescOf 从随包原件里读一句话说明；读不到给空串（界面退化成只显示目录名）。
func skillDescOf(source, dirName string) string {
	raw, err := os.ReadFile(filepath.Join(source, dirName, "SKILL.md"))
	if err != nil {
		return ""
	}
	return pkg.ParseFrontmatter(string(raw)).Description
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
		recommended := RecommendedSkillOf(entry.Name())
		row := domain.SkillRowVO{
			SkillName:     defaultIfEmpty(fm.Name, entry.Name()),
			SkillDirName:  entry.Name(),
			SkillDesc:     fm.Description,
			SkillPath:     skillFile,
			SkillDir:      dir,
			SkillSource:   s.sourceOf(entry.Name(), root == projectRoot),
			SkillEnabled:  active,
			SkillEditable: root != projectRoot,
			SkillModified: s.isModified(entry.Name(), dir),
		}
		if recommended != nil {
			row.SkillGroup = recommended.SkillGroup
			row.SkillRequires = recommended.SkillRequires
		}
		if row.SkillSource == domain.SkillSourceProject {
			row.SkillEnabled = true
			row.SkillEditable = false
		}
		rows = append(rows, row)
	}
	return rows
}

// sourceOf 决定技能来源：项目 / 推荐 / 托管 / 随包附带。
func (s *Skills) sourceOf(dirName string, isProject bool) domain.SkillSourceKind {
	if isProject {
		return domain.SkillSourceProject
	}
	if RecommendedSkillOf(dirName) != nil {
		return domain.SkillSourceBuiltin
	}
	// 磁盘上有、清单里没登记的：随包附带。它不享受版本管理，删了不会回来。
	if pkg.Exists(filepath.Join(s.builtinSource(), dirName)) {
		return domain.SkillSourceBundled
	}
	return domain.SkillSourceManaged
}

// isModified 本地副本与随包原件是否一致；非推荐技能恒 false。
func (s *Skills) isModified(dirName, localDir string) bool {
	if RecommendedSkillOf(dirName) == nil {
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
