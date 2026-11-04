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

// BuiltinPlugins 是内置插件清单：顺序即首启安装顺序，联网访问排最前
// （其余插件常依赖它）。
var BuiltinPlugins = []BuiltinPlugin{
	{
		PluginName:      "pi-web-access",
		PluginVersion:   "0.42.0",
		PluginDesc:      "联网搜索、抓取 URL、克隆 GitHub、PDF 与视频理解",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@juicesharp/rpiv-todo",
		PluginVersion:   "2.13.9",
		PluginDesc:      "实时待办清单，跨 /reload 与上下文压缩存活",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@juicesharp/rpiv-ask-user-question",
		PluginVersion:   "2.13.9",
		PluginDesc:      "结构化问卷，让模型在拿不准时先问你",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-hermes-memory",
		PluginVersion:   "0.9.10",
		PluginDesc:      "持久记忆、会话检索、密钥扫描与自动知识整合",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "context-mode",
		PluginVersion:   "1.0.169",
		PluginDesc:      "省上下文：沙箱执行 + FTS5 知识库 + 意图搜索",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@ff-labs/pi-fff",
		PluginVersion:   "0.11.0",
		PluginDesc:      "模糊文件名与内容搜索",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@narumitw/pi-lsp",
		PluginVersion:   "0.49.10",
		PluginDesc:      "语言无关的 LSP 工具，共享 runner",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@plannotator/pi-extension",
		PluginVersion:   "0.28.23",
		PluginDesc:      "交互式计划评审、标注与 PR 审查",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-codex-goal",
		PluginVersion:   "0.6.1",
		PluginDesc:      "Codex 式目标追踪与续跑",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-loadout",
		PluginVersion:   "0.0.37",
		PluginDesc:      "交互式工具与技能装载配置；与 GUI 的管理面职责重叠，默认停用",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-inspect",
		PluginVersion:   "0.5.0",
		PluginDesc:      "自省面板：工具、斜杠命令、技能与注入的系统提示",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-context-view",
		PluginVersion:   "0.7.2",
		PluginDesc:      "可视化上下文用量，检查隐藏的基础提示与工具定义；与内置的用量面板重复，默认停用",
		PluginDefaultOn: false,
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
