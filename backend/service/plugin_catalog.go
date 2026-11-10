package service

// BuiltinPlugin 是一个随壳分发的插件描述；版本号即钉住的 npm 精确版本。
type BuiltinPlugin struct {
	// PluginName npm 包名（插件身份，不随版本变）
	PluginName string
	// PluginVersion 精确版本；空串表示跟随 latest
	PluginVersion string
	// PluginDesc 界面上一句话说明
	PluginDesc string
	// PluginDefaultOn 是否默认启用
	PluginDefaultOn bool
}

// BuiltinPlugins 是内置插件清单：顺序即首启安装顺序，联网访问排最前（其余插件常依赖它），
// 之后只是阅读顺序。这份清单是产品规格，不多不少，增删要同步 docs 与 README。
//
// 口径是「能力不重叠」：联网 / 代码体检 / 计划 / 待办 / 提问 / 输出折叠各占一格，
// 没有第二个插件做同一件事。多装一个插件就是引擎冷启动多一轮加载，
// 能力重叠的装进来只赚噪音。用户想装其余插件，界面上照样能装。
var BuiltinPlugins = []BuiltinPlugin{
	{
		PluginName:      "pi-web-access",
		PluginVersion:   "0.46.0",
		PluginDesc:      "联网搜索、抓取 URL、克隆 GitHub、PDF 与视频理解",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-lens",
		PluginVersion:   "4.4.10",
		PluginDesc:      "LSP、lint、类型检查与符号结构化搜索",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-plan-mode",
		PluginVersion:   "0.4.8",
		PluginDesc:      "Plan 模式：只读探索与计划文件编辑",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@juicesharp/rpiv-todo",
		PluginVersion:   "2.15.0",
		PluginDesc:      "实时待办清单，跨 /reload 与上下文压缩存活",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@juicesharp/rpiv-ask-user-question",
		PluginVersion:   "2.15.0",
		PluginDesc:      "结构化问卷，让模型在拿不准时先问你",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-tidy",
		PluginVersion:   "0.9.3",
		PluginDesc:      "工具调用输出折叠：一次回复的多次工具调用收成一行",
		PluginDefaultOn: true,
	},
}

// BuiltinPluginOf 按包名取内置描述；非内置给 nil。
func BuiltinPluginOf(pluginName string) *BuiltinPlugin {
	for i := range BuiltinPlugins {
		if BuiltinPlugins[i].PluginName == pluginName {
			return &BuiltinPlugins[i]
		}
	}
	return nil
}

// NpmSourceOf npm 来源写法：pi 的 packages 数组认 `npm:<name>@<version>`。
func NpmSourceOf(item BuiltinPlugin) string {
	if item.PluginVersion == "" {
		return "npm:" + item.PluginName
	}
	return "npm:" + item.PluginName + "@" + item.PluginVersion
}

// removedBuiltinPlugin 是历史上随壳预置过、现不在清单里的条目。
type removedBuiltinPlugin struct {
	// PluginName npm 包名
	PluginName string
	// PluginVersion 当初钉住的版本
	PluginVersion string
}

// removedBuiltinPlugins 是收窄清单后要撤回的声明。
// 老数据目录里它们还躺在 packages[] 里，引擎每次启动都要加载一遍：
// 声明收窄了但老机器上照旧慢，清单就是假的。
//
// 判据只认「包名 + 当初钉住的那个版本」这个完整来源串：用户后来自己升级过的
// 版本串对不上，那就不是我们写的声明，一条都不动。
var removedBuiltinPlugins = []removedBuiltinPlugin{
	{PluginName: "pi-hermes-memory", PluginVersion: "0.10.0"},
	{PluginName: "@ff-labs/pi-fff", PluginVersion: "0.11.0"},
	{PluginName: "@narumitw/pi-lsp", PluginVersion: "0.49.12"},
	{PluginName: "@plannotator/pi-extension", PluginVersion: "0.28.48"},
	{PluginName: "pi-inspect", PluginVersion: "0.6.0"},
	{PluginName: "pi-context-view", PluginVersion: "0.8.2"},
	{PluginName: "pi-footer", PluginVersion: "0.5.1"},
	{PluginName: "pi-chrome", PluginVersion: "0.15.57"},
	{PluginName: "pi-subagents", PluginVersion: "0.84.0"},
	{PluginName: "pi-web", PluginVersion: "0.15.5"},
	{PluginName: "pi-smart-compact", PluginVersion: "10.7.3"},
	{PluginName: "pi-openui", PluginVersion: "0.2.0"},
	{PluginName: "pi-extension-doctor", PluginVersion: "0.1.0"},
	{PluginName: "pi-browser-harness", PluginVersion: "0.14.0"},
}

// isRetiredSource 判断一条声明是不是当初为下架插件写的那一条。
// 只认「包名 + 钉住的版本」拼出的完整来源串：用户自己升过级或换过来源，
// 版本串就对不上，那说明这条声明现在是用户的，不该被我们撤回。
func isRetiredSource(source string) bool {
	for _, item := range removedBuiltinPlugins {
		if source == "npm:"+item.PluginName+"@"+item.PluginVersion {
			return true
		}
	}
	return false
}
