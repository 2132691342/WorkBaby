package tests

import (
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// TestSkillResetKeepsFile 断言重置后 SKILL.md 仍是可读的文件。
// 重置的实现是「删掉技能目录再从原件复制回来」：复制目标写错成 SKILL.md 本身时，
// 它会变成一个目录，技能从清单里消失且再也回不来——用户点一次重置就报废一个技能。
func TestSkillResetKeepsFile(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))
	app.Skills.Provision()

	item := service.BuiltinSkills[0]
	skillDir := filepath.Join(layout.SkillsDir(), item.SkillDirName)
	skillFile := filepath.Join(skillDir, "SKILL.md")

	if err := app.Skills.Reset(skillFile); err != nil {
		t.Fatalf("重置失败：%v", err)
	}
	info, err := os.Stat(skillFile)
	if err != nil {
		t.Fatalf("重置后 SKILL.md 不存在：%v", err)
	}
	if info.IsDir() {
		t.Fatal("重置后 SKILL.md 变成了目录")
	}
	if _, err := os.ReadFile(skillFile); err != nil {
		t.Fatalf("重置后 SKILL.md 读不出来：%v", err)
	}
}

// TestSkillDeleteRemovesDir 断言删除技能时整个目录被清掉。
// 只删 SKILL.md 会把 scripts / references 留在沙箱里占盘；
// 而内置技能一旦被删就没有重置入口（它已不在清单里），残留永远清不掉。
func TestSkillDeleteRemovesDir(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))
	app.Skills.Provision()

	item := service.BuiltinSkills[0]
	skillDir := filepath.Join(layout.SkillsDir(), item.SkillDirName)
	skillFile := filepath.Join(skillDir, "SKILL.md")

	if err := app.Skills.Delete(skillFile); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if pkg.Exists(skillDir) {
		entries, _ := os.ReadDir(skillDir)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("删除后技能目录仍在，残留 %v", names)
	}
}

// TestSkillDeleteRejectsDirPath 断言传目录路径时拒绝删除。
// 目录路径可以正好是 skills 根：接住就等于一次误传清空全部内置技能。
func TestSkillDeleteRejectsDirPath(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))
	app.Skills.Provision()

	before, _ := os.ReadDir(layout.SkillsDir())
	if err := app.Skills.Delete(layout.SkillsDir()); err == nil {
		t.Error("传入目录路径时应当拒绝删除")
	}
	after, _ := os.ReadDir(layout.SkillsDir())
	if len(before) != len(after) {
		t.Errorf("被拒的删除不应改动技能目录：前 %d 个，后 %d 个", len(before), len(after))
	}
}

// TestSkillDuplicateHeals 断言「启用区与禁用区同时有同一个技能」的破损态能自愈。
// 破损态下清单里会出现两行同名技能，一行开一行关。
func TestSkillDuplicateHeals(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))
	app.Skills.Provision()

	item := service.BuiltinSkills[0]
	active := filepath.Join(layout.SkillsDir(), item.SkillDirName)
	hidden := filepath.Join(layout.SkillsOffDir(), item.SkillDirName)
	// 造出破损态：两边各一份同样的内容。
	if err := pkg.EnsureDir(hidden); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	raw, err := os.ReadFile(filepath.Join(active, "SKILL.md"))
	if err != nil {
		t.Fatalf("读原件失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), raw, 0o644); err != nil {
		t.Fatalf("写副本失败：%v", err)
	}

	app.Skills.Provision()
	if pkg.Exists(hidden) {
		t.Error("破损态自愈后禁用区不该还留着重复份")
	}
	// 清单里同一个技能只能出现一次。
	seen := map[string]int{}
	for _, row := range app.Skills.List().Skills {
		seen[row.SkillDirName]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("技能 %s 在清单里出现了 %d 次", name, count)
		}
	}
}
