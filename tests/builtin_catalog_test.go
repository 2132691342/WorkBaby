// Package tests 是场景测试：只断言「改这里会坏那里」的长链路。
// 本文件覆盖内置清单与落位：TestBuiltinManifestMatchesAssets（清单↔资产）、
// TestSkillProvisionAndToggle（技能生命周期）、TestPluginLifecycle / TestPluginVersionDrift（插件）。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

// catalogHarness 只装技能/插件需要的那一小撮依赖；assetsDir 为空时用临时目录。
func catalogHarness(t *testing.T, assetsDir string) (*service.App, pkg.Layout, *repo.Store) {
	t.Helper()
	dir := t.TempDir()
	if assetsDir == "" {
		assetsDir = filepath.Join(dir, "assets")
	}
	layout := pkg.Layout{
		DataDir:    dir,
		PiAgentDir: filepath.Join(dir, "pi"),
		AssetsDir:  assetsDir,
		DbPath:     filepath.Join(dir, "workbaby.db"),
		LogDir:     filepath.Join(dir, "logs"),
	}
	pkg.SetLogDir("")
	for _, sub := range []string{layout.PiAgentDir, layout.LogDir} {
		if err := pkg.EnsureDir(sub); err != nil {
			t.Fatalf("准备目录 %s 失败：%v", sub, err)
		}
	}
	store, err := repo.Open(layout.DbPath)
	if err != nil {
		t.Fatalf("开设置库失败：%v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	app := service.New(service.Options{Layout: layout, Store: store, Hub: sse.New(16), Version: "test"})
	return app, layout, store
}

// repoAssetsDir 定位仓库里的 assets 目录（测试从 tests/ 跑，往上一级）。
func repoAssetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "assets"))
	if err != nil {
		t.Fatalf("解析资产目录失败：%v", err)
	}
	if !pkg.IsDir(filepath.Join(dir, "skills")) {
		t.Fatalf("资产目录里没有 skills：%s", dir)
	}
	return dir
}

// TestBuiltinManifestMatchesAssets 断言随包清单与磁盘资产互相咬合：
// 包名唯一、版本钉住、来源串可往返；技能清单与 assets\skills 一一对应，
// 缺一边都会出现「永远装不上」或「永不落位」的死条目。
func TestBuiltinManifestMatchesAssets(t *testing.T) {
	// 插件清单自洽：包名唯一、版本钉住、来源串能往返。
	seen := map[string]struct{}{}
	for _, item := range service.BuiltinPlugins {
		if item.PluginName == "" {
			t.Fatal("内置插件必须有包名")
		}
		if _, dup := seen[item.PluginName]; dup {
			t.Fatalf("内置插件清单里有重复包名：%s", item.PluginName)
		}
		seen[item.PluginName] = struct{}{}
		if item.PluginVersion == "" {
			t.Errorf("%s 缺少钉住的版本号：空串会让安装退化成跟随 latest", item.PluginName)
		}
		source := service.NpmSourceOf(item)
		if want := "npm:" + item.PluginName + "@" + item.PluginVersion; source != want {
			t.Errorf("%s 的来源串应是 %q，实际 %q", item.PluginName, want, source)
		}
		if got := service.PackageNameOf(source); got != item.PluginName {
			t.Errorf("%s 反解包名错了：得到 %q", source, got)
		}
	}
	if len(service.BuiltinPlugins) != 15 {
		t.Errorf("内置插件应为规格的 15 个，实际 %d 个", len(service.BuiltinPlugins))
	}

	// 技能清单与 assets\skills 一一对应。
	assets := repoAssetsDir(t)
	entries, err := os.ReadDir(filepath.Join(assets, "skills"))
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
}

// TestSkillProvisionAndToggle 走一遍技能生命周期：首启落位（尊重默认值、
// 不覆盖用户改动）→ 开 → 关。开关的实现是搬目录：pi 只扫 skills/，移走即失联；
// 哪天改成改配置，这个测试会先坏——那正是提醒。
func TestSkillProvisionAndToggle(t *testing.T) {
	app, layout, store := catalogHarness(t, repoAssetsDir(t))

	offItem := defaultOffSkill(t)
	active := filepath.Join(layout.SkillsDir(), offItem.SkillDirName)
	hidden := filepath.Join(layout.SkillsOffDir(), offItem.SkillDirName)

	// 首启落位：默认停用的进禁用区；默认启用的进启用区。
	app.Skills.Provision()
	if pkg.Exists(active) || !pkg.Exists(hidden) {
		t.Fatalf("默认停用的 %s 应落进禁用区而不是启用区", offItem.SkillDirName)
	}
	onItem := defaultOnSkill(t)
	if !pkg.Exists(filepath.Join(layout.SkillsDir(), onItem.SkillDirName, "SKILL.md")) {
		t.Fatalf("默认启用的 %s 应落进启用区", onItem.SkillDirName)
	}

	// 已存在的目录不动：用户改过的内容不该被下次启动覆盖。
	probe := filepath.Join(active, "SKILL.md")
	_ = os.MkdirAll(active, 0o755)
	if err := os.WriteFile(probe, []byte("---\nname: x\n---\n用户改过\n"), 0o644); err != nil {
		t.Fatalf("写探针文件失败：%v", err)
	}
	app.Skills.Provision()
	raw, err := os.ReadFile(probe)
	if err != nil {
		t.Fatalf("重读失败：%v", err)
	}
	if string(raw) != "---\nname: x\n---\n用户改过\n" {
		t.Fatal("Provision 不能覆盖用户已改过的技能文件")
	}
	_ = os.RemoveAll(active)

	// 开：目录从禁用区搬回启用区，台账记 true。
	if err := app.Skills.Toggle(filepath.Join(hidden, "SKILL.md"), true); err != nil {
		t.Fatalf("启用技能失败：%v", err)
	}
	if pkg.Exists(hidden) || !pkg.Exists(active) {
		t.Fatal("启用后目录应从禁用区搬回启用区")
	}
	if enabled, ok := store.SkillEnabled(offItem.SkillDirName); !ok || !enabled {
		t.Fatalf("启用应记账为 true，实际 ok=%v enabled=%v", ok, enabled)
	}

	// 关：目录搬进禁用区，台账记 false。
	if err := app.Skills.Toggle(filepath.Join(active, "SKILL.md"), false); err != nil {
		t.Fatalf("停用技能失败：%v", err)
	}
	if pkg.Exists(active) || !pkg.Exists(hidden) {
		t.Fatal("停用后目录应从启用区搬到禁用区")
	}
	if enabled, ok := store.SkillEnabled(offItem.SkillDirName); !ok || enabled {
		t.Fatalf("停用应记账为 false，实际 ok=%v enabled=%v", ok, enabled)
	}
}

// defaultOffSkill 取清单里第一个默认停用的技能；没有则本测试失去意义。
func defaultOffSkill(t *testing.T) *service.BuiltinSkill {
	t.Helper()
	for i := range service.BuiltinSkills {
		if !service.BuiltinSkills[i].SkillDefaultOn {
			return &service.BuiltinSkills[i]
		}
	}
	t.Fatal("清单里应至少有一个默认停用的技能")
	return nil
}

// defaultOnSkill 取清单里第一个默认启用的技能。
func defaultOnSkill(t *testing.T) *service.BuiltinSkill {
	t.Helper()
	for i := range service.BuiltinSkills {
		if service.BuiltinSkills[i].SkillDefaultOn {
			return &service.BuiltinSkills[i]
		}
	}
	t.Fatal("清单里应至少有一个默认启用的技能")
	return nil
}

// TestPluginLifecycle 断言插件的启停与卸载都写在 pi 的原生表达上：
// 停用是对象形态 autoload:false，启用是裸来源串，卸载撤声明并删目录。
func TestPluginLifecycle(t *testing.T) {
	app, layout, _ := catalogHarness(t, "")
	app.Plugins.DeclareBuiltins()

	const pluginName = "pi-web-access"
	pinned := service.BuiltinPluginOf(pluginName)
	if pinned == nil {
		t.Fatalf("清单里应有 %s", pluginName)
	}
	writePkg(t, layout, pluginName, pinned.PluginVersion)

	// 停用：对象形态。
	if _, err := app.Plugins.Toggle(pluginName, false); err != nil {
		t.Fatalf("停用插件失败：%v", err)
	}
	assertEntry(t, readPackages(t, layout), pluginName, false)

	// 启用：裸来源串——写对象形态（哪怕 autoload:true）会让引擎多走一条分支。
	if _, err := app.Plugins.Toggle(pluginName, true); err != nil {
		t.Fatalf("启用插件失败：%v", err)
	}
	entries := readPackages(t, layout)
	assertEntry(t, entries, pluginName, true)
	for _, entry := range entries {
		if obj, ok := entry.(map[string]any); ok && service.PackageNameOf(entrySource(obj)) == pluginName {
			t.Fatalf("启用态应写裸来源串，实际写了对象：%v", obj)
		}
	}

	// 卸载：目录与声明一起消失。
	if _, err := app.Plugins.Uninstall(pluginName); err != nil {
		t.Fatalf("卸载失败：%v", err)
	}
	if pkg.Exists(filepath.Join(layout.PluginRoot(), "node_modules", pluginName)) {
		t.Fatal("卸载后包目录应已删除")
	}
	for _, entry := range readPackages(t, layout) {
		if service.PackageNameOf(entrySource(entry)) == pluginName {
			t.Fatalf("卸载后不应还有 %s 的声明：%v", pluginName, entry)
		}
	}
}

// TestPluginVersionDrift 断言内置插件判据是「在且版本对」，批量升级只碰漂移的包。
// 测试环境没有内置 node，npm 必然失败；断言的是「被尝试过」（进了失败表），不是「装成功」。
func TestPluginVersionDrift(t *testing.T) {
	const drifted, aligned = "pi-hermes-memory", "@ff-labs/pi-fff"
	driftedItem, alignedItem := service.BuiltinPluginOf(drifted), service.BuiltinPluginOf(aligned)
	if driftedItem == nil || alignedItem == nil {
		t.Fatalf("清单里应有 %s 与 %s", drifted, aligned)
	}

	// Provision：版本装歪的应被尝试重装（记入失败表），版本对齐的直接跳过。
	app, layout, _ := catalogHarness(t, "")
	writePkg(t, layout, drifted, "9.9.9")
	writePkg(t, layout, aligned, alignedItem.PluginVersion)
	app.Plugins.ProvisionBuiltins()
	if reason := pluginErrorOf(app, drifted); reason == "" {
		t.Fatalf("版本装歪（9.9.9 ≠ %s）的 %s 应被重新安装；没有失败记录说明被当成已装跳过了",
			driftedItem.PluginVersion, drifted)
	}
	if reason := pluginErrorOf(app, aligned); reason != "" {
		t.Fatalf("版本已对齐（%s）的 %s 不该被重装，却出现了失败：%s", alignedItem.PluginVersion, aligned, reason)
	}

	// UpgradeAll：清单先标对「可升级」，然后只尝试漂移的那个。
	app2, layout2, _ := catalogHarness(t, "")
	writePkg(t, layout2, drifted, "0.0.1")
	writePkg(t, layout2, aligned, alignedItem.PluginVersion)
	var driftedRow, alignedRow bool
	for _, row := range app2.Plugins.List().Plugins {
		switch row.PluginName {
		case drifted:
			driftedRow = row.PluginUpgradable
		case aligned:
			alignedRow = row.PluginUpgradable
		}
	}
	if !driftedRow || alignedRow {
		t.Fatalf("升级标记错误：%s 应可升级=%v，%s 应不可升级=%v", drifted, driftedRow, aligned, alignedRow)
	}
	app2.Plugins.UpgradeAll()
	if reason := pluginErrorOf(app2, drifted); reason == "" {
		t.Fatalf("版本不符的 %s 应被尝试升级（应记入失败表）", drifted)
	}
	if reason := pluginErrorOf(app2, aligned); reason != "" {
		t.Fatalf("版本已对齐的 %s 不该被碰，却出现了失败：%s", aligned, reason)
	}
}

// TestPluginSourceNormalization 断言用户输入被规范成 pi 认的来源串。
// 这里锁的是一处真实 bug：曾经把 git 源写成 `git:https://…`，而 git 不认这个协议，
// 安装直接失败。pi 的 git 源就是 URL 本身，不存在 `git:` 前缀。
func TestPluginSourceNormalization(t *testing.T) {
	cases := []struct{ in, want, wantName string }{
		// npm：没前缀补 npm:，带前缀原样保留。
		{"pi-web-access", "npm:pi-web-access", "pi-web-access"},
		{"npm:pi-web-access@0.46.0", "npm:pi-web-access@0.46.0", "pi-web-access"},
		// scope 包：@ 在最前面，不能被当版本分隔符切掉。
		{"@juicesharp/rpiv-todo", "npm:@juicesharp/rpiv-todo", "@juicesharp/rpiv-todo"},
		{"npm:@ff-labs/pi-fff@0.11.0", "npm:@ff-labs/pi-fff@0.11.0", "@ff-labs/pi-fff"},
		// git 源：URL 本身，不加任何前缀。
		{"https://github.com/u/r", "https://github.com/u/r", "https://github.com/u/r"},
		{"git@github.com:u/r.git", "git@github.com:u/r.git", "git@github.com:u/r.git"},
		{"github.com/u/r", "github.com/u/r", "github.com/u/r"},
		// 本地源。
		{"./my-plugin", "./my-plugin", "./my-plugin"},
	}
	for _, c := range cases {
		if got := service.PluginSourceOf(c.in); got != c.want {
			t.Errorf("PluginSourceOf(%q) 应为 %q，实际 %q", c.in, c.want, got)
		}
		if got := service.PackageNameOf(service.PluginSourceOf(c.in)); got != c.wantName {
			t.Errorf("包名反解(%q) 应为 %q，实际 %q", c.in, c.wantName, got)
		}
	}

	// 自造前缀必须被判为非 git 源，否则会带着非法协议发出去。
	if service.IsGitSource("git:https://github.com/u/r") {
		t.Error("git: 不是 pi 认的来源写法，不该被当成 git 源")
	}
}

// writePkg 在插件根下造一个假包目录（只有 package.json）。
func writePkg(t *testing.T, layout pkg.Layout, name, version string) {
	t.Helper()
	dir := filepath.Join(layout.PluginRoot(), "node_modules", name)
	if err := pkg.EnsureDir(dir); err != nil {
		t.Fatalf("建包目录失败：%v", err)
	}
	meta := `{"name":"` + name + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("写 package.json 失败：%v", err)
	}
}

// pluginErrorOf 从公开清单里取一个插件的失败原因；没有给空串。
func pluginErrorOf(app *service.App, pluginName string) string {
	for _, row := range app.Plugins.List().Plugins {
		if row.PluginName == pluginName {
			return row.PluginError
		}
	}
	return ""
}

// readPackages 读 settings.json 的 packages 数组。
func readPackages(t *testing.T, layout pkg.Layout) []any {
	t.Helper()
	raw, err := os.ReadFile(layout.SettingsFile())
	if err != nil {
		t.Fatalf("读 settings.json 失败：%v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json 不是合法 JSON：%v", err)
	}
	entries, _ := settings["packages"].([]any)
	return entries
}

// entrySource 取一条声明的来源串（裸串或对象形态都支持）。
func entrySource(entry any) string {
	switch value := entry.(type) {
	case string:
		return value
	case map[string]any:
		source, _ := value["source"].(string)
		return source
	}
	return ""
}

// assertEntry 断言某个包在 packages 里的启用态。
func assertEntry(t *testing.T, entries []any, pluginName string, wantEnabled bool) {
	t.Helper()
	for _, entry := range entries {
		if service.PackageNameOf(entrySource(entry)) != pluginName {
			continue
		}
		enabled := true
		if obj, ok := entry.(map[string]any); ok {
			if flag, ok := obj["autoload"].(bool); ok {
				enabled = flag
			}
		}
		if enabled != wantEnabled {
			t.Fatalf("%s 的启用态应是 %v，实际 %v（条目=%v）", pluginName, wantEnabled, enabled, entry)
		}
		return
	}
	t.Fatalf("packages 里找不到 %s；实际 %v", pluginName, entries)
}
