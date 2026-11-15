// 技能：推荐清单与磁盘资产互相咬合，技能生命周期整条可跑。
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

func TestSkills(t *testing.T) {
	// 清单咬合：少一边就是死条目——清单里的目录必须在 assets\skills 下存在，
	// 磁盘上的目录必须进清单；SKILL.md 的 description 必须能被解析出来（触发全靠它）。
	t.Run("清单咬合", func(t *testing.T) {
		assets := repoAssetsDir(t)
		skillsDir := filepath.Join(assets, "skills")
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			t.Fatalf("读内置技能资产失败：%v", err)
		}
		onDisk := map[string]struct{}{}
		for _, entry := range entries {
			if entry.IsDir() {
				onDisk[entry.Name()] = struct{}{}
			}
		}
		for _, item := range service.RecommendedSkills {
			if _, ok := onDisk[item.SkillDirName]; !ok {
				t.Errorf("推荐清单里的技能在 assets\\skills 下不存在：%s", item.SkillDirName)
			}
		}
		for name := range onDisk {
			if service.RecommendedSkillOf(name) == nil {
				t.Errorf("assets\\skills 下的 %s 没进推荐清单，界面不会展示它", name)
			}
		}
		// description 是引擎决定「何时触发」的唯一依据，块标量（| 与 >）接不上就只剩一个字符。
		for name := range onDisk {
			raw, err := os.ReadFile(filepath.Join(skillsDir, name, "SKILL.md"))
			if err != nil {
				t.Errorf("技能 %s 没有 SKILL.md：%v", name, err)
				continue
			}
			fm := pkg.ParseFrontmatter(string(raw))
			if fm.Name == "" {
				t.Errorf("技能 %s 解析不出 name", name)
			}
			if len(fm.Description) < 10 {
				t.Errorf("技能 %s 的描述解析成了 %d 个字符（块标量没接上）：%q", name, len(fm.Description), fm.Description)
			}
		}
	})

	// 生命周期：壳不自动落位、点一次才落一份、重复安装拒绝且不覆盖用户改动、
	// 开关=搬目录+记账、重置后 SKILL.md 仍是可读文件、删除清整目录、两区重复只出一行。
	t.Run("生命周期", func(t *testing.T) {
		assets := repoAssetsDir(t)
		app, skillLayout, store := catalogHarness(t, assets)
		item := firstRecommendedSkill(t)
		active := filepath.Join(skillLayout.SkillsDir, item.SkillDirName)
		hidden := filepath.Join(skillLayout.SkillsOffDir, item.SkillDirName)

		// 壳不自动落位——技能目录只在他点了安装后才出现东西。
		if pkg.Exists(active) || pkg.Exists(hidden) {
			t.Fatalf("壳不该自动把 %s 写进技能目录", item.SkillDirName)
		}

		// 安装：点一次才落一份，再点一次拒绝（已有就不覆盖）。
		if err := app.Skills.Install(item.SkillDirName); err != nil {
			t.Fatalf("安装技能失败：%v", err)
		}
		if !pkg.Exists(filepath.Join(active, "SKILL.md")) {
			t.Fatalf("安装后 %s 应落在启用区", item.SkillDirName)
		}
		if err := app.Skills.Install(item.SkillDirName); err == nil {
			t.Error("重复安装应被拒绝，而不是默默覆盖用户的改动")
		}

		// 安装不覆盖用户已有内容。
		const userEdit = "---\nname: x\n---\n用户改过\n"
		probe := filepath.Join(active, "SKILL.md")
		if err := os.WriteFile(probe, []byte(userEdit), 0o644); err != nil {
			t.Fatalf("写探针文件失败：%v", err)
		}
		_ = app.Skills.Install(item.SkillDirName)
		raw, err := os.ReadFile(probe)
		if err != nil {
			t.Fatalf("重读失败：%v", err)
		}
		if string(raw) != userEdit {
			t.Fatal("安装不能覆盖用户已改过的技能文件")
		}

		// 开关 = 搬目录 + 台账记账。
		if err := app.Skills.Toggle(filepath.Join(active, "SKILL.md"), false); err != nil {
			t.Fatalf("停用技能失败：%v", err)
		}
		if pkg.Exists(active) || !pkg.Exists(hidden) {
			t.Fatal("停用后目录应从启用区搬到禁用区")
		}
		if enabled, ok := store.SkillEnabled(item.SkillDirName); !ok || enabled {
			t.Fatalf("停用应记账为 false，实际 ok=%v enabled=%v", ok, enabled)
		}
		if err := app.Skills.Toggle(filepath.Join(hidden, "SKILL.md"), true); err != nil {
			t.Fatalf("启用技能失败：%v", err)
		}
		if pkg.Exists(hidden) || !pkg.Exists(active) {
			t.Fatal("启用后目录应从禁用区搬回启用区")
		}
		if enabled, ok := store.SkillEnabled(item.SkillDirName); !ok || !enabled {
			t.Fatalf("启用应记账为 true，实际 ok=%v enabled=%v", ok, enabled)
		}

		// 重置后 SKILL.md 必须仍是可读文件，不能变成目录。
		onDir := active
		onFile := filepath.Join(onDir, "SKILL.md")
		if err := app.Skills.Reset(onFile); err != nil {
			t.Fatalf("重置失败：%v", err)
		}
		info, err := os.Stat(onFile)
		if err != nil {
			t.Fatalf("重置后 SKILL.md 不存在：%v", err)
		}
		if info.IsDir() {
			t.Fatal("重置后 SKILL.md 变成了目录")
		}
		if _, err := os.ReadFile(onFile); err != nil {
			t.Fatalf("重置后 SKILL.md 读不出来：%v", err)
		}

		// 删除清整个目录；传目录路径时拒绝且不动盘。
		if err := app.Skills.Delete(onFile); err != nil {
			t.Fatalf("删除失败：%v", err)
		}
		if pkg.Exists(onDir) {
			t.Errorf("删除后技能目录仍在：%s", onDir)
		}
		before, _ := os.ReadDir(skillLayout.SkillsDir)
		if err := app.Skills.Delete(skillLayout.SkillsDir); err == nil {
			t.Error("传入目录路径时应当拒绝删除")
		}
		after, _ := os.ReadDir(skillLayout.SkillsDir)
		if len(before) != len(after) {
			t.Errorf("被拒的删除不应改动技能目录：前 %d 个，后 %d 个", len(before), len(after))
		}

		// 同一个技能在两个区都有时清单只出一行，不重复展示。
		if err := app.Skills.Install(item.SkillDirName); err != nil {
			t.Fatalf("重新安装技能失败：%v", err)
		}
		if err := pkg.EnsureDir(hidden); err != nil {
			t.Fatalf("建目录失败：%v", err)
		}
		raw, err = os.ReadFile(filepath.Join(active, "SKILL.md"))
		if err != nil {
			t.Fatalf("读原件失败：%v", err)
		}
		if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), raw, 0o644); err != nil {
			t.Fatalf("写副本失败：%v", err)
		}
		rowCount := map[string]int{}
		for _, row := range app.Skills.List().Skills {
			rowCount[row.SkillDirName]++
		}
		for name, count := range rowCount {
			if count > 1 {
				t.Errorf("技能 %s 在清单里出现了 %d 次", name, count)
			}
		}
	})
}
