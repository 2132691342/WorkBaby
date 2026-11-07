// Package tests 的沙箱治理：内置清单与磁盘资产的咬合、技能的整条生命周期、
// 子进程环境对沙箱数据目录的钉扎。
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// TestSandbox 断言沙箱的三层契约：内置清单与磁盘资产互相咬合（少一边就是死条目）、
// 子进程环境把缓存与家目录钉进数据目录（少钉一条沙箱就漏）、
// 技能的落位/开关/重置/删除/自愈整条生命周期。
func TestSandbox(t *testing.T) {
	// ---- 清单咬合：包名唯一，且来源串能被反解回原包名（装 / 卸 / 开关全按包名索引）----
	// 逐版本比对是复述实现——每升一个插件版本都要改一遍测试；这里只守结构不变量。
	seen := map[string]struct{}{}
	for _, item := range service.BuiltinPlugins {
		if _, dup := seen[item.PluginName]; dup {
			t.Fatalf("内置插件清单里有重复包名：%s", item.PluginName)
		}
		seen[item.PluginName] = struct{}{}
		if got := service.PackageNameOf(service.NpmSourceOf(item)); got != item.PluginName {
			t.Errorf("来源串反解不出原包名：%s → %q", item.PluginName, got)
		}
	}

	// ---- 清单咬合：技能目录与 SKILL.md frontmatter ----
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
	inCatalog := map[string]struct{}{}
	for _, item := range service.BuiltinSkills {
		inCatalog[item.SkillDirName] = struct{}{}
		if _, ok := onDisk[item.SkillDirName]; !ok {
			t.Errorf("清单里的技能在 assets\\skills 下不存在：%s", item.SkillDirName)
		}
	}
	for name := range onDisk {
		if _, ok := inCatalog[name]; !ok {
			t.Errorf("assets\\skills 下的 %s 没进清单，永远不会被落位", name)
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

	// ---- 环境钉扎：家目录与两处包缓存必须落在沙箱数据目录里，漏一条沙箱就漏 ----
	layout := pkg.ResolveLayout(t.TempDir(), true)
	values := map[string]string{}
	for _, item := range layout.SandboxEnv(nil) {
		if idx := strings.IndexByte(item, '='); idx > 0 {
			values[item[:idx]] = item[idx+1:]
		}
	}
	for _, key := range []string{"HOME", "USERPROFILE", "npm_config_cache", "PIP_CACHE_DIR"} {
		got, ok := values[key]
		if !ok {
			t.Fatalf("沙箱环境缺 %s", key)
		}
		if !pkg.Inside(layout.DataDir, got) {
			t.Fatalf("%s 应钉在沙箱数据目录里，实际 %q", key, got)
		}
	}

	// ---- 技能生命周期：落位按默认值各归各区，已存在的不覆盖——用户改动是数据 ----
	app, skillLayout, store := catalogHarness(t, assets)
	offItem := defaultSkill(t, false)
	onItem := defaultSkill(t, true)
	active := filepath.Join(skillLayout.SkillsDir(), offItem.SkillDirName)
	hidden := filepath.Join(skillLayout.SkillsOffDir(), offItem.SkillDirName)

	app.Skills.Provision()
	if pkg.Exists(active) || !pkg.Exists(hidden) {
		t.Fatalf("默认停用的 %s 应落进禁用区", offItem.SkillDirName)
	}
	if !pkg.Exists(filepath.Join(skillLayout.SkillsDir(), onItem.SkillDirName, "SKILL.md")) {
		t.Fatalf("默认启用的 %s 应落进启用区", onItem.SkillDirName)
	}

	probe := filepath.Join(active, "SKILL.md")
	if err := pkg.EnsureDir(active); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	const userEdit = "---\nname: x\n---\n用户改过\n"
	if err := os.WriteFile(probe, []byte(userEdit), 0o644); err != nil {
		t.Fatalf("写探针文件失败：%v", err)
	}
	app.Skills.Provision()
	raw, err := os.ReadFile(probe)
	if err != nil {
		t.Fatalf("重读失败：%v", err)
	}
	if string(raw) != userEdit {
		t.Fatal("Provision 不能覆盖用户已改过的技能文件")
	}
	_ = os.RemoveAll(active)

	// ---- 技能生命周期：开关 = 搬目录 + 台账记账 ----
	if err := app.Skills.Toggle(filepath.Join(hidden, "SKILL.md"), true); err != nil {
		t.Fatalf("启用技能失败：%v", err)
	}
	if pkg.Exists(hidden) || !pkg.Exists(active) {
		t.Fatal("启用后目录应从禁用区搬回启用区")
	}
	if enabled, ok := store.SkillEnabled(offItem.SkillDirName); !ok || !enabled {
		t.Fatalf("启用应记账为 true，实际 ok=%v enabled=%v", ok, enabled)
	}
	if err := app.Skills.Toggle(filepath.Join(active, "SKILL.md"), false); err != nil {
		t.Fatalf("停用技能失败：%v", err)
	}
	if pkg.Exists(active) || !pkg.Exists(hidden) {
		t.Fatal("停用后目录应从启用区搬到禁用区")
	}
	if enabled, ok := store.SkillEnabled(offItem.SkillDirName); !ok || enabled {
		t.Fatalf("停用应记账为 false，实际 ok=%v enabled=%v", ok, enabled)
	}

	// ---- 技能生命周期：重置后 SKILL.md 必须仍是可读文件 ----
	// 复制目标错成它自己就变成目录，技能永久报废。
	onDir := filepath.Join(skillLayout.SkillsDir(), onItem.SkillDirName)
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

	// ---- 技能生命周期：删除清整个目录；目录路径拒绝且不动盘 ----
	if err := app.Skills.Delete(onFile); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if pkg.Exists(onDir) {
		t.Errorf("删除后技能目录仍在：%s", onDir)
	}
	before, _ := os.ReadDir(skillLayout.SkillsDir())
	if err := app.Skills.Delete(skillLayout.SkillsDir()); err == nil {
		t.Error("传入目录路径时应当拒绝删除")
	}
	after, _ := os.ReadDir(skillLayout.SkillsDir())
	if len(before) != len(after) {
		t.Errorf("被拒的删除不应改动技能目录：前 %d 个，后 %d 个", len(before), len(after))
	}

	// ---- 技能生命周期：自愈——两边同内容时清掉禁用区重复份，清单只出一行 ----
	if err := pkg.EnsureDir(active); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	raw, err = os.ReadFile(filepath.Join(hidden, "SKILL.md"))
	if err != nil {
		t.Fatalf("读原件失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(active, "SKILL.md"), raw, 0o644); err != nil {
		t.Fatalf("写副本失败：%v", err)
	}
	app.Skills.Provision()
	if pkg.Exists(hidden) {
		t.Error("破损态自愈后禁用区不该还留着重复份")
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
}
