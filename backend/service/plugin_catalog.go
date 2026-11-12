package service

// RecommendedPlugin 是一个推荐插件；版本号即钉住的 npm 精确版本。
// 推荐不等于预置：壳不会替用户往他自己的 settings.json 里写任何东西，
// 清单只在应用页摆出来，用户点了才装。
type RecommendedPlugin struct {
	// PluginName npm 包名（插件身份，不随版本变）
	PluginName string
	// PluginVersion 精确版本；空串表示跟随 latest
	PluginVersion string
	// PluginDesc 界面上一句话说明
	PluginDesc string
}

// RecommendedPlugins 是推荐插件清单：顺序即安装顺序，联网访问排最前（其余插件常依赖它），
// 之后只是阅读顺序。增删要同步 docs 与 README。
//
// 口径是「能力不重叠」：联网 / 代码体检 / 计划 / 待办 / 提问 / 输出折叠各占一格。
// 多装一个插件就是引擎冷启动多一轮加载，能力重叠的装进来只赚噪音。
// 用户想装别的一律走「安装」输入框。
var RecommendedPlugins = []RecommendedPlugin{
	{
		PluginName:    "pi-web-access",
		PluginVersion: "0.46.0",
		PluginDesc:    "联网搜索、抓取 URL、克隆 GitHub、PDF 与视频理解",
	},
	{
		PluginName:    "pi-lens",
		PluginVersion: "4.4.10",
		PluginDesc:    "LSP、lint、类型检查与符号结构化搜索",
	},
	{
		PluginName:    "pi-plan-mode",
		PluginVersion: "0.4.8",
		PluginDesc:    "Plan 模式：只读探索与计划文件编辑",
	},
	{
		PluginName:    "@juicesharp/rpiv-todo",
		PluginVersion: "2.15.0",
		PluginDesc:    "实时待办清单，跨 /reload 与上下文压缩存活",
	},
	{
		PluginName:    "@juicesharp/rpiv-ask-user-question",
		PluginVersion: "2.15.0",
		PluginDesc:    "结构化问卷，让模型在拿不准时先问你",
	},
	{
		PluginName:    "pi-tidy",
		PluginVersion: "0.9.3",
		PluginDesc:    "工具调用输出折叠：一次回复的多次工具调用收成一行",
	},
}

// RecommendedPluginOf 按包名取推荐描述；不在清单里给 nil。
func RecommendedPluginOf(pluginName string) *RecommendedPlugin {
	for i := range RecommendedPlugins {
		if RecommendedPlugins[i].PluginName == pluginName {
			return &RecommendedPlugins[i]
		}
	}
	return nil
}

// NpmSourceOf npm 来源写法：pi 的 packages 数组认 `npm:<name>@<version>`。
func NpmSourceOf(item RecommendedPlugin) string {
	if item.PluginVersion == "" {
		return "npm:" + item.PluginName
	}
	return "npm:" + item.PluginName + "@" + item.PluginVersion
}
