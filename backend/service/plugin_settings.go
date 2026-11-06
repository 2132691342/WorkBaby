package service

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// piSettings 是 settings.json 的读取形状。壳不另建注册表——
// 这个文件是插件唯一真相；按 map 读是为了保留用户手配的未知键。
type piSettings struct {
	// NpmCommand 引擎调用 npm 用的命令：必须指向内置 node + npm-cli.js
	NpmCommand []string `json:"npmCommand,omitempty"`
	// Packages 包声明：裸串 = 启用，对象 = 带 autoload 等过滤
	Packages []any `json:"packages,omitempty"`
}

// readPiSettings 读 settings.json；不存在或坏掉给空结构，绝不抛。
// 用户会手改这个文件，读坏等同于「还没配过」，让界面重写一份比报错有用。
func readPiSettings(layout pkg.Layout) map[string]any {
	out := map[string]any{}
	pkg.ReadJSON(layout.SettingsFile(), &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

// writePiSettings 写回 settings.json（原子写）。
func writePiSettings(layout pkg.Layout, value map[string]any) error {
	if err := pkg.WriteJSON(layout.SettingsFile(), value); err != nil {
		return pkg.WrapErr(pkg.CodeConfigWrite, "settings.json 写入失败", err)
	}
	return nil
}

// packageEntry 是一条 packages 声明。
type packageEntry struct {
	// Source 来源串（npm:name@ver）
	Source string
	// Enabled 是否启用：对象形态的 autoload === false 表示停用
	Enabled bool
}

// packagesOf 从 settings 里取出 packages 声明。
func packagesOf(settings map[string]any) []any {
	raw, ok := settings["packages"].([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(raw))
	for _, item := range raw {
		switch item.(type) {
		case string, map[string]any:
			out = append(out, item)
		}
	}
	return out
}

// parseEntry 把一条声明解成来源与启用态。
func parseEntry(item any) packageEntry {
	if source, ok := item.(string); ok {
		return packageEntry{Source: source, Enabled: true}
	}
	obj, ok := item.(map[string]any)
	if !ok {
		return packageEntry{}
	}
	source, _ := obj["source"].(string)
	enabled := true
	// 只有显式的 autoload:false 才是停用；缺省、null 都按启用算。
	if flag, ok := obj["autoload"].(bool); ok && !flag {
		enabled = false
	}
	return packageEntry{Source: source, Enabled: enabled}
}

// entryToJSON 把声明编码回 settings.json 的形状。
func entryToJSON(source string, enabled bool) any {
	if enabled {
		return source
	}
	return map[string]any{"source": source, "autoload": false}
}

// PackageNameOf 从一个来源串里取 npm 包名。
// `npm:@scope/pkg@1.2.3` → `@scope/pkg`；git / 本地来源原样返回（不做猜测）。
func PackageNameOf(source string) string {
	raw := strings.TrimSpace(source)
	raw = strings.TrimPrefix(raw, "npm:")
	if IsGitSource(raw) || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return strings.TrimSuffix(raw, "/")
	}
	// 从后往前找 @：scope 包的 @ 在最前面，不能被当成分隔符切掉。
	if at := strings.LastIndexByte(raw, '@'); at > 0 {
		raw = raw[:at]
	}
	return strings.TrimSuffix(raw, "/")
}

// PluginSourceOf 把用户输入规范成 pi 认的来源串。
// 只有 npm 包名需要加 `npm:`；git 源写 URL 本身（github.com/u/r、https://…、git@…），
// 本地源写相对路径。不存在 `git:` 前缀——git 会把 git:https://… 当非法协议直接报错。
func PluginSourceOf(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, "npm:"), IsGitSource(trimmed):
		return trimmed
	case strings.HasPrefix(trimmed, "./"), strings.HasPrefix(trimmed, "../"):
		return trimmed
	default:
		return "npm:" + trimmed
	}
}

// IsGitSource 判断是不是 pi 认的 git 来源：协议 URL、scp 式地址、host/path 缩写。
func IsGitSource(source string) bool {
	switch {
	case strings.HasPrefix(source, "git+"), strings.HasPrefix(source, "git@"):
		return true
	case strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "ssh://"):
		return true
	}
	// host/path 缩写（github.com/user/repo）：首段带点才算，避免把目录名当域名。
	if head, _, ok := strings.Cut(source, "/"); ok && strings.Contains(head, ".") {
		return true
	}
	return false
}

// EnsureNpmCommand 把 npmCommand 落到 settings.json：pi 自己装插件也会走
// PATH 上的 npm，不钉到内置 node 就等于把插件装进用户环境。
func EnsureNpmCommand(layout pkg.Layout, npmCommand []string) {
	if !layout.NodeReady() {
		// 内置 node 还没解压：写半成品路径会让引擎启动时找不到 npm。
		return
	}
	settings := readPiSettings(layout)
	if sameStrings(asStringSlice(settings["npmCommand"]), npmCommand) {
		return
	}
	settings["npmCommand"] = toAnySlice(npmCommand)
	if err := writePiSettings(layout, settings); err != nil {
		pkg.Warn("npmCommand 写入失败：%v", err)
	}
}

// sameStrings 比较两个字符串切片。
func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// asStringSlice 把 JSON 数组转成字符串切片；非数组给 nil。
func asStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		if direct, ok := v.([]string); ok {
			return direct
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// toAnySlice 把字符串切片转成 JSON 可编码的通用切片。
func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// installedPathOf 某个 npm 包在插件安装根下的目录；scope 包多一层。
func installedPathOf(layout pkg.Layout, pluginName string) string {
	return filepath.Join(layout.PluginRoot(), "node_modules", filepath.FromSlash(pluginName))
}

// pluginInstalled 包是否真的落到磁盘（以 package.json 为准）。
func pluginInstalled(layout pkg.Layout, pluginName string) bool {
	return pkg.Exists(filepath.Join(installedPathOf(layout, pluginName), "package.json"))
}

// pluginMeta 读已装包的版本与说明；没装给零值。
func pluginMeta(layout pkg.Layout, pluginName string) (version, description string) {
	var meta struct {
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if pkg.ReadJSON(filepath.Join(installedPathOf(layout, pluginName), "package.json"), &meta) == nil {
		return "", ""
	}
	return meta.Version, meta.Description
}

// resourceKindsOf 包内暴露的资源种类：读 pi 的 manifest，退回惯例目录。
func resourceKindsOf(layout pkg.Layout, pluginName string) []string {
	dir := installedPathOf(layout, pluginName)
	var meta struct {
		Pi map[string]any `json:"pi"`
	}
	if pkg.ReadJSON(filepath.Join(dir, "package.json"), &meta) == nil && len(meta.Pi) > 0 {
		kinds := make([]string, 0, len(meta.Pi))
		for kind := range meta.Pi {
			if kind == "image" || kind == "video" {
				continue
			}
			kinds = append(kinds, kind)
		}
		return kinds
	}
	kinds := make([]string, 0, 4)
	for _, kind := range []string{"extensions", "skills", "prompts", "themes"} {
		if pkg.IsDir(filepath.Join(dir, kind)) {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) == 0 {
		return []string{"extensions"}
	}
	return kinds
}

// removeAllQuiet 删目录；不存在不算失败。
func removeAllQuiet(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return pkg.WrapErr(pkg.CodeProvision, "目录删除失败", err)
	}
	return nil
}
