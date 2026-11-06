// Package tests 的沙箱治理：内置清单与磁盘资产互相咬合，技能的落位、开关、重置、删除与破损自愈。
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// TestBuiltinManifest 断言随包清单与磁盘资产互相咬合，且资产能被引擎消费。
// 少一边就会出死条目：清单里有而资产没有＝永远装不上；资产里有而清单没有＝永不落位；
// SKILL.md 的 frontmatter 解析不出来＝引擎没有触发这个技能的依据。
func TestBuiltinManifest(t *testing.T) {
	// 逐条钉住规格清单：只断数量的话，删一个多一个都看不出来。
	spec := map[string]string{
		"pi-web-access":                      "0.46.0",
		"pi-hermes-memory":                   "0.10.0",
		"@ff-labs/pi-fff":                    "0.11.0",
		"@narumitw/pi-lsp":                   "0.49.12",
		"pi-plan-mode":                       "0.4.8",
		"pi-subagents":                       "0.84.0",
		"pi-smart-compact":                   "10.7.3",
		"pi-openui":                          "0.2.0",
		"@juicesharp/rpiv-todo":              "2.15.0",
		"@juicesharp/rpiv-ask-user-question": "2.15.0",
		"@plannotator/pi-extension":          "0.28.48",
		"pi-footer":                          "0.5.1",
		"pi-extension-doctor":                "0.1.0",
		"pi-browser-harness":                 "0.14.0",
		"pi-inspect":                         "0.6.0",
		"pi-context-view":                    "0.8.2",
		"pi-chrome":                          "0.15.57",
		"pi-web":                             "0.15.5",
	}
	if len(service.BuiltinPlugins) != len(spec) {
		t.Errorf("内置插件应为规格的 %d 个，实际 %d 个", len(spec), len(service.BuiltinPlugins))
	}
	seen := map[string]struct{}{}
	for _, item := range service.BuiltinPlugins {
		if item.PluginName == "" {
			t.Fatal("内置插件必须有包名")
		}
		if _, dup := seen[item.PluginName]; dup {
			t.Fatalf("内置插件清单里有重复包名：%s", item.PluginName)
		}
		seen[item.PluginName] = struct{}{}
		want, ok := spec[item.PluginName]
		if !ok {
			t.Errorf("内置插件不在规格清单里：%s", item.PluginName)
		} else if item.PluginVersion != want {
			t.Errorf("%s 的钉住版本应是 %s，实际 %s", item.PluginName, want, item.PluginVersion)
		}
		if item.PluginVersion == "" {
			t.Errorf("%s 缺钉住的版本号：空串会让安装退化成跟随 latest", item.PluginName)
		}
		source := service.NpmSourceOf(item)
		if want := "npm:" + item.PluginName + "@" + item.PluginVersion; source != want {
			t.Errorf("%s 的来源串应是 %q，实际 %q", item.PluginName, want, source)
		}
		if got := service.PackageNameOf(source); got != item.PluginName {
			t.Errorf("%s 反解包名错了：得到 %q", source, got)
		}
	}
	for name := range spec {
		if _, ok := seen[name]; !ok {
			t.Errorf("规格清单里的插件不在内置清单中：%s", name)
		}
	}

	// 技能：清单 ↔ 磁盘目录双向咬合。
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

	// 每个技能的 SKILL.md 都要解析出 name 与足够长的 description：
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
}

// TestSkillLifecycle 走完技能的整条生命周期，每一步锁一个会真实发生的破损：
//
//	落位：默认停用的进禁用区、默认启用的进启用区；已存在的目录绝不覆盖——用户改动是数据。
//	开关：实现是搬目录（引擎只扫 skills/，移走即失联），目录位置与台账必须同时变化。
//	重置：删掉再从原件复制回来，SKILL.md 必须仍是文件——复制目标错成它自己就变成目录，技能永久报废。
//	删除：整个目录清掉；只删 SKILL.md 会把 scripts/references 留在沙箱里，且删掉的内置技能没有重置入口。
//	自愈：搬移中途被杀会留下「两边各一份」的破损态，内容一致时清掉禁用区那份，清单只出一行。
func TestSkillLifecycle(t *testing.T) {
	app, layout, store := catalogHarness(t, repoAssetsDir(t))

	offItem := defaultSkill(t, false)
	onItem := defaultSkill(t, true)
	active := filepath.Join(layout.SkillsDir(), offItem.SkillDirName)
	hidden := filepath.Join(layout.SkillsOffDir(), offItem.SkillDirName)

	// ---- 落位：按默认值各归各区，已存在的不覆盖 ----
	app.Skills.Provision()
	if pkg.Exists(active) || !pkg.Exists(hidden) {
		t.Fatalf("默认停用的 %s 应落进禁用区", offItem.SkillDirName)
	}
	if !pkg.Exists(filepath.Join(layout.SkillsDir(), onItem.SkillDirName, "SKILL.md")) {
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

	// ---- 开关：目录搬移 + 台账记账 ----
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

	// ---- 重置：SKILL.md 必须仍是可读文件 ----
	onDir := filepath.Join(layout.SkillsDir(), onItem.SkillDirName)
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

	// ---- 删除：清整个目录；目录路径拒绝且不动盘 ----
	if err := app.Skills.Delete(onFile); err != nil {
		t.Fatalf("删除失败：%v", err)
	}
	if pkg.Exists(onDir) {
		t.Errorf("删除后技能目录仍在：%s", onDir)
	}
	before, _ := os.ReadDir(layout.SkillsDir())
	if err := app.Skills.Delete(layout.SkillsDir()); err == nil {
		t.Error("传入目录路径时应当拒绝删除")
	}
	after, _ := os.ReadDir(layout.SkillsDir())
	if len(before) != len(after) {
		t.Errorf("被拒的删除不应改动技能目录：前 %d 个，后 %d 个", len(before), len(after))
	}

	// ---- 自愈：两边同内容时清掉禁用区重复份，清单只出一行 ----
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
