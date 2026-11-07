// Package tests 的插件治理：一切落盘都写在 pi 的原生表达（settings.json 的 packages[]）上，
// 来源串规范、启停、重装、卸载、版本漂移全部围绕这份声明运转。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// TestPlugins 断言插件治理的全部契约，一段一类：
//
//	来源规范：用户输入必须规范成 pi 认的来源串；git 源带 git: 前缀引擎直接装不上。
//	启停：停用写对象形态 autoload:false——裸来源串没法表达关；启用写裸来源串——
//	plain 源写对象形态（哪怕 autoload:true）会让引擎多走一条分支。
//	配置保留：对象形态可以带 extensions 等资源过滤数组，开关只许动 autoload。
//	重试：只让包就位，不得把用户停用的插件静默重新打开。
//	卸载：声明与包目录一起消失。
//	版本漂移：判据是「在且版本对」，批量升级只碰漂移的包。
func TestPlugins(t *testing.T) {
	// ---- 来源规范：入口处先钉住，后面所有声明都经它产出 ----
	for _, c := range []struct{ in, wantSource, wantName string }{
		{"pi-web-access", "npm:pi-web-access", "pi-web-access"},
		{"npm:pi-web-access@0.46.0", "npm:pi-web-access@0.46.0", "pi-web-access"},
		{"@juicesharp/rpiv-todo", "npm:@juicesharp/rpiv-todo", "@juicesharp/rpiv-todo"},
		{"npm:@ff-labs/pi-fff@0.11.0", "npm:@ff-labs/pi-fff@0.11.0", "@ff-labs/pi-fff"},
		{"https://github.com/u/r", "https://github.com/u/r", "https://github.com/u/r"},
		{"git@github.com:u/r.git", "git@github.com:u/r.git", "git@github.com:u/r.git"},
		{"github.com/u/r", "github.com/u/r", "github.com/u/r"},
		{"./my-plugin", "./my-plugin", "./my-plugin"},
	} {
		if got := service.PluginSourceOf(c.in); got != c.wantSource {
			t.Errorf("PluginSourceOf(%q) 应为 %q，实际 %q", c.in, c.wantSource, got)
		}
		if got := service.PackageNameOf(service.PluginSourceOf(c.in)); got != c.wantName {
			t.Errorf("包名反解(%q) 应为 %q，实际 %q", c.in, c.wantName, got)
		}
	}
	if service.IsGitSource("git:https://github.com/u/r") {
		t.Error("git: 不是 pi 认的来源写法，不该被当成 git 源")
	}

	app, layout, _ := catalogHarness(t, "")
	app.Plugins.DeclareBuiltins()

	const pluginName = "pi-web-access"
	pinned := service.BuiltinPluginOf(pluginName)
	if pinned == nil {
		t.Fatalf("清单里应有 %s", pluginName)
	}
	writePkg(t, layout, pluginName, pinned.PluginVersion)

	// ---- 启停：停用对象形态、启用裸串 ----
	if _, err := app.Plugins.Toggle(pluginName, false); err != nil {
		t.Fatalf("停用插件失败：%v", err)
	}
	assertEntry(t, readPackages(t, layout), pluginName, false)
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

	// ---- 配置保留：手写一条带过滤的停用声明，开关两个方向都不许碰 extensions ----
	writeFilteredSettings := func(t *testing.T) {
		t.Helper()
		settings := map[string]any{
			"packages": []any{
				map[string]any{
					"source":     "npm:" + pluginName + "@0.0.1",
					"autoload":   false,
					"extensions": []any{"only-this-extension"},
				},
			},
		}
		if err := pkg.WriteJSON(layout.SettingsFile(), settings); err != nil {
			t.Fatalf("写测试配置失败：%v", err)
		}
	}
	writeFilteredSettings(t)
	if _, err := app.Plugins.Toggle(pluginName, true); err != nil {
		t.Fatalf("启用失败：%v", err)
	}
	assertFiltersKept(t, readPackages(t, layout), pluginName, false)
	writeFilteredSettings(t)
	if _, err := app.Plugins.Toggle(pluginName, false); err != nil {
		t.Fatalf("停用失败：%v", err)
	}
	assertFiltersKept(t, readPackages(t, layout), pluginName, true)

	// ---- 重试：只装包，不开开关 ----
	if _, err := app.Plugins.Retry(pluginName); err != nil {
		t.Fatalf("重试安装失败：%v", err)
	}
	assertEntry(t, readPackages(t, layout), pluginName, false)

	// ---- 卸载：目录与声明一起消失 ----
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

	// ---- 版本漂移：Provision 只碰「版本装歪」的；对齐的直接跳过 ----
	// 测试环境没有内置 node，npm 必然失败；断言的是「被尝试过」（进了失败表），不是「装成功」。
	const drifted, aligned = "pi-hermes-memory", "@ff-labs/pi-fff"
	driftedItem, alignedItem := service.BuiltinPluginOf(drifted), service.BuiltinPluginOf(aligned)
	if driftedItem == nil || alignedItem == nil {
		t.Fatalf("清单里应有 %s 与 %s", drifted, aligned)
	}
	writePkg(t, layout, drifted, "9.9.9")
	writePkg(t, layout, aligned, alignedItem.PluginVersion)
	app.Plugins.ProvisionBuiltins()
	if reason := pluginErrorOf(app, drifted); reason == "" {
		t.Fatalf("版本装歪（9.9.9 ≠ %s）的 %s 应被重装；没有失败记录说明被当成已装跳过了",
			driftedItem.PluginVersion, drifted)
	}
	if reason := pluginErrorOf(app, aligned); reason != "" {
		t.Fatalf("版本已对齐（%s）的 %s 不该被重装，却出现失败：%s", alignedItem.PluginVersion, aligned, reason)
	}

	// ---- 版本漂移：UpgradeAll 标对「可升级」，且只尝试漂移的那个 ----
	var driftedRow, alignedRow bool
	for _, row := range app.Plugins.List().Plugins {
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
	app.Plugins.UpgradeAll()
	if pluginErrorOf(app, drifted) == "" {
		t.Fatalf("版本不符的 %s 应被尝试升级", drifted)
	}
	if reason := pluginErrorOf(app, aligned); reason != "" {
		t.Fatalf("版本已对齐的 %s 不该被碰，却出现失败：%s", aligned, reason)
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

// entrySource 取一条声明的来源串（裸串与对象形态都支持）。
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

// assertFiltersKept 断言该插件的声明仍是对象形态且带着 extensions。
// wantAutoload 为 true 表示期望看到 autoload:false（停用态）。
func assertFiltersKept(t *testing.T, entries []any, pluginName string, wantAutoload bool) {
	t.Helper()
	for _, entry := range entries {
		if service.PackageNameOf(entrySource(entry)) != pluginName {
			continue
		}
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("声明被抹成了裸串，资源过滤丢了：%v", entry)
		}
		extensions, kept := obj["extensions"]
		if !kept || extensions == nil {
			t.Fatalf("资源过滤 extensions 被抹掉了：%v", obj)
		}
		_, hasAutoload := obj["autoload"]
		if hasAutoload != wantAutoload {
			t.Errorf("autoload 键的存在性不对：期望 %v，实际 %v（%v）", wantAutoload, hasAutoload, obj)
		}
		return
	}
	t.Fatalf("清单里找不到 %s 的声明", pluginName)
}
