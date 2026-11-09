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
// 之后只是阅读顺序。这份清单是产品规格，不多不少，增删要同步 docs 与 README；
// 默认停用的只限「与本壳 GUI 职责重叠」的几个，其余默认开启。
var BuiltinPlugins = []BuiltinPlugin{
	{
		PluginName:      "pi-web-access",
		PluginVersion:   "0.46.0",
		PluginDesc:      "联网搜索、抓取 URL、克隆 GitHub、PDF 与视频理解",
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
		PluginName:      "pi-hermes-memory",
		PluginVersion:   "0.10.0",
		PluginDesc:      "持久记忆、会话检索、密钥扫描与自动知识整合",
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
		PluginVersion:   "0.49.12",
		PluginDesc:      "语言无关的 LSP 工具，共享 runner",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "@plannotator/pi-extension",
		PluginVersion:   "0.28.48",
		PluginDesc:      "交互式计划评审、标注与 PR 审查",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-inspect",
		PluginVersion:   "0.6.0",
		PluginDesc:      "自省面板：工具、斜杠命令、技能与注入的系统提示",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-context-view",
		PluginVersion:   "0.8.2",
		PluginDesc:      "可视化上下文用量，检查隐藏的基础提示与工具定义；与内置的用量面板重复，默认停用",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-footer",
		PluginVersion:   "0.5.1",
		PluginDesc:      "可配置多行 footer / 状态栏",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-chrome",
		PluginVersion:   "0.15.57",
		PluginDesc:      "显式授权后复用已登录的 Chrome 配置文件",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-plan-mode",
		PluginVersion:   "0.4.8",
		PluginDesc:      "Plan 模式：只读探索与计划文件编辑",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-lens",
		PluginVersion:   "4.4.10",
		PluginDesc:      "LSP、lint、类型检查与符号结构化搜索",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-tidy",
		PluginVersion:   "0.9.3",
		PluginDesc:      "工具调用输出折叠：一次回复的多次工具调用收成一行",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-subagents",
		PluginVersion:   "0.84.0",
		PluginDesc:      "单 Agent 委派与脚本化多 Agent 工作流",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-web",
		PluginVersion:   "0.15.5",
		PluginDesc:      "pi 的 Web UI，浏览器里操作 pi；与本壳 GUI 职责重叠，默认停用",
		PluginDefaultOn: false,
	},
	{
		PluginName:      "pi-smart-compact",
		PluginVersion:   "10.7.3",
		PluginDesc:      "可恢复的上下文清理、校验式压缩、会话导航与项目记忆",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-openui",
		PluginVersion:   "0.2.0",
		PluginDesc:      "终端内交互式 dashboard、图表与地图",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-extension-doctor",
		PluginVersion:   "0.1.0",
		PluginDesc:      "只读诊断扩展冲突与过期 API",
		PluginDefaultOn: true,
	},
	{
		PluginName:      "pi-browser-harness",
		PluginVersion:   "0.14.0",
		PluginDesc:      "浏览器自动化",
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
