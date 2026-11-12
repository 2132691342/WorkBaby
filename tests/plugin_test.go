// 插件治理：壳只读用户的 settings.json、只写启停这一个字段，
// 装 / 卸 / 升一律交给 pi 自己的包命令——落盘只有一套。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// TestPlugins 断言插件治理里壳真正负责的那部分：
// 启停两态的写法、资源过滤不被开关抹掉、清单只出实际存在的东西、
// 推荐清单与来源串互相对得上。
func TestPlugins(t *testing.T) {
	app, layout, _ := catalogHarness(t, "")

	const pluginName = "pi-web-access"
	recommended := service.RecommendedPluginOf(pluginName)
	if recommended == nil {
		t.Fatalf("推荐清单里应有 %s", pluginName)
	}
	// 壳不再替用户声明任何插件：settings.json 由用户与 pi 决定。
	if pkg.Exists(layout.SettingsFile()) {
		t.Fatalf("壳不应在空闲时创建 settings.json：%s", layout.SettingsFile())
	}
	writePkg(t, layout, pluginName, recommended.PluginVersion)
	writeSettings(t, layout, "npm:"+pluginName+"@"+recommended.PluginVersion)

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

	// ---- 配置保留：开关两个方向都不许碰 extensions ----
	writeFilteredSettings(t, layout, pluginName)
	if _, err := app.Plugins.Toggle(pluginName, true); err != nil {
		t.Fatalf("启用失败：%v", err)
	}
	assertFiltersKept(t, readPackages(t, layout), pluginName, false)
	writeFilteredSettings(t, layout, pluginName)
	if _, err := app.Plugins.Toggle(pluginName, false); err != nil {
		t.Fatalf("停用失败：%v", err)
	}
	assertFiltersKept(t, readPackages(t, layout), pluginName, true)

	// ---- 清单：未声明的推荐项也要出现（状态是可安装，不是不可见） ----
	writeSettings(t, layout)
	rows := map[string]bool{}
	for _, row := range app.Plugins.List().Plugins {
		rows[row.PluginName] = row.PluginRecommended
	}
	for _, item := range service.RecommendedPlugins {
		if _, ok := rows[item.PluginName]; !ok {
			t.Errorf("推荐插件 %s 没出现在清单里——用户会以为壳不支持它", item.PluginName)
		}
	}

	// ---- 清单：推荐清单外的用户自装包也要出现，且不算推荐 ----
	const userPlugin = "my-own-plugin"
	writePkg(t, layout, userPlugin, "1.2.3")
	writeSettings(t, layout, "npm:"+userPlugin)
	var sawUser bool
	for _, row := range app.Plugins.List().Plugins {
		if row.PluginName == userPlugin {
			sawUser = true
			if row.PluginRecommended {
				t.Errorf("用户自装的 %s 不该被标成推荐插件", userPlugin)
			}
			if !row.PluginInstalled {
				t.Errorf("包目录已存在，%s 应显示为已安装", userPlugin)
			}
		}
	}
	if !sawUser {
		t.Errorf("用户自装的 %s 没出现在清单里", userPlugin)
	}

	// ---- 来源串：包名反解与来源规范必须闭环（装 / 卸 / 开关全按包名索引） ----
	for _, item := range service.RecommendedPlugins {
		if got := service.PackageNameOf(service.NpmSourceOf(item)); got != item.PluginName {
			t.Errorf("来源串反解不出原包名：%s → %q", item.PluginName, got)
		}
	}
}

// writeSettings 用一个给定的裸来源串列表重写 settings.json 的 packages。
func writeSettings(t *testing.T, layout pkg.Layout, sources ...string) {
	t.Helper()
	entries := make([]any, 0, len(sources))
	for _, source := range sources {
		entries = append(entries, source)
	}
	settings := map[string]any{}
	if len(entries) > 0 {
		settings["packages"] = entries
	}
	if err := pkg.WriteJSON(layout.SettingsFile(), settings); err != nil {
		t.Fatalf("写测试配置失败：%v", err)
	}
}

// writeFilteredSettings 造一条带 extensions 过滤的停用声明。
func writeFilteredSettings(t *testing.T, layout pkg.Layout, pluginName string) {
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
