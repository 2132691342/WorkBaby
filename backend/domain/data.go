package domain

// SessionRowVO 是会话清单的一行。
type SessionRowVO struct {
	// SessionId 会话 ID（取自 JSONL 头）
	SessionId string `json:"session_id"`
	// SessionPath 会话文件绝对路径
	SessionPath string `json:"session_path"`
	// SessionTitle 标题（首条用户消息截断，或改名后的名字）
	SessionTitle string `json:"session_title"`
	// SessionCwd 会话绑定的工作目录
	SessionCwd string `json:"session_cwd"`
	// SessionModel 最近使用的模型
	SessionModel string `json:"session_model"`
	// UpdatedAt 最后一条条目的时刻（毫秒）
	UpdatedAt int64 `json:"updated_at"`
	// SizeBytes 文件字节数
	SizeBytes int64 `json:"size_bytes"`
	// MessageCount 消息轮数
	MessageCount int `json:"message_count"`
	// TotalTokens token 合计
	TotalTokens int `json:"total_tokens"`
	// CostUsd 花费合计（美元）
	CostUsd float64 `json:"cost_usd"`
}

// SessionDetailRESP 是会话详情：清单行 + 逐条消息。
type SessionDetailRESP struct {
	// Session 清单行
	Session SessionRowVO `json:"session"`
	// Items 逐条消息
	Items []SessionItemVO `json:"items"`
}

// SessionItemVO 是会话详情里的一条消息。
type SessionItemVO struct {
	// ItemKind user / assistant / tool
	ItemKind string `json:"item_kind"`
	// ItemRole 原始角色
	ItemRole string `json:"item_role"`
	// ItemText 文本正文
	ItemText string `json:"item_text,omitempty"`
	// ItemModel 产生这条的模型
	ItemModel string `json:"item_model,omitempty"`
	// ToolName 工具名
	ToolName string `json:"tool_name,omitempty"`
	// ToolCallId 工具调用 ID
	ToolCallId string `json:"tool_call_id,omitempty"`
	// ToolIsError 工具是否失败
	ToolIsError bool `json:"tool_is_error,omitempty"`
	// CreatedAt 条目的时刻（毫秒）
	CreatedAt int64 `json:"created_at"`
	// ItemTokens 这条的 token
	ItemTokens int `json:"item_tokens,omitempty"`
	// CostUsd 这条的花费
	CostUsd float64 `json:"cost_usd,omitempty"`
}

// SessionPathREQ 是带会话文件路径的入参。
type SessionPathREQ struct {
	// SessionPath 会话文件绝对路径
	SessionPath string `json:"session_path"`
}

// SessionRenameREQ 是会话改名的入参。
type SessionRenameREQ struct {
	// SessionPath 会话文件绝对路径
	SessionPath string `json:"session_path"`
	// SessionTitle 新标题
	SessionTitle string `json:"session_title"`
}

// SessionListRESP 是会话清单。
type SessionListRESP struct {
	// Sessions 会话行，按 UpdatedAt 降序
	Sessions []SessionRowVO `json:"sessions"`
}

// SkillSourceKind 是技能的来源。
type SkillSourceKind string

const (
	// SkillSourceBuiltin 清单里登记的内置技能（随包分发、可重置、受版本管理）
	SkillSourceBuiltin SkillSourceKind = "builtin"
	// SkillSourceBundled 随包附带但未登记：删了不会回来
	SkillSourceBundled SkillSourceKind = "bundled"
	// SkillSourceManaged 用户在界面里新建的
	SkillSourceManaged SkillSourceKind = "managed"
	// SkillSourceProject 工作区 .pi\skills 下的
	SkillSourceProject SkillSourceKind = "project"
)

// SkillRowVO 是一个技能。
type SkillRowVO struct {
	// SkillName 显示名（frontmatter 的 name，缺省用目录名）
	SkillName string `json:"skill_name"`
	// SkillDir 目录名（启用 / 停用区之间搬移时用它做身份）
	SkillDirName string `json:"skill_dir_name"`
	// SkillDesc 一句话说明
	SkillDesc string `json:"skill_desc"`
	// SkillPath SKILL.md 绝对路径
	SkillPath string `json:"skill_path"`
	// SkillDir 技能目录绝对路径
	SkillDir string `json:"skill_dir"`
	// SkillSource 来源
	SkillSource SkillSourceKind `json:"skill_source"`
	// SkillGroup 分组标签
	SkillGroup string `json:"skill_group"`
	// SkillRequires 外部依赖说明
	SkillRequires string `json:"skill_requires,omitempty"`
	// SkillEnabled 是否启用
	SkillEnabled bool `json:"skill_enabled"`
	// SkillEditable 能否在界面里编辑
	SkillEditable bool `json:"skill_editable"`
	// SkillModified 本地副本是否与随包原件不同
	SkillModified bool `json:"skill_modified"`
}

// SkillListRESP 是技能总表。
type SkillListRESP struct {
	// Skills 技能行
	Skills []SkillRowVO `json:"skills"`
	// BuiltinTotal 内置清单总数
	BuiltinTotal int `json:"builtin_total"`
	// BuiltinInstalled 已落位的内置技能数
	BuiltinInstalled int `json:"builtin_installed"`
}

// SkillPathREQ 是带 SKILL.md 路径的入参。
type SkillPathREQ struct {
	// SkillPath SKILL.md 绝对路径
	SkillPath string `json:"skill_path"`
}

// SkillSaveREQ 是保存技能正文的入参。
type SkillSaveREQ struct {
	// SkillPath SKILL.md 绝对路径
	SkillPath string `json:"skill_path"`
	// FileText 新正文
	FileText string `json:"file_text"`
}

// SkillToggleREQ 是技能开关的入参。
type SkillToggleREQ struct {
	// SkillPath SKILL.md 绝对路径
	SkillPath string `json:"skill_path"`
	// SkillEnabled 目标状态
	SkillEnabled bool `json:"skill_enabled"`
}

// SkillCreateREQ 是新建技能的入参。
type SkillCreateREQ struct {
	// SkillName 技能名（只允许字母数字与短横线）
	SkillName string `json:"skill_name"`
	// SkillDesc 一句话说明
	SkillDesc string `json:"skill_desc"`
	// SkillBody 正文
	SkillBody string `json:"skill_body"`
}

// SkillImportREQ 是外部技能导入的入参。
type SkillImportREQ struct {
	// SkillFromPath 用户选中的技能源目录
	SkillFromPath string `json:"skill_from_path"`
}

// SkillImportRESP 是外部技能导入的结果。
type SkillImportRESP struct {
	// SkillDirName 导入后的目录名
	SkillDirName string `json:"skill_dir_name"`
	// SkillPath 导入后 SKILL.md 绝对路径
	SkillPath string `json:"skill_path"`
}

// SkillContentRESP 是技能正文。
type SkillContentRESP struct {
	// FileText 正文
	FileText string `json:"file_text"`
}

// PluginStateKind 是插件的运行期状态。
type PluginStateKind string

const (
	// PluginStateInstalled 已装且可用
	PluginStateInstalled PluginStateKind = "installed"
	// PluginStateMissing 声明了但包目录不在
	PluginStateMissing PluginStateKind = "missing"
	// PluginStateFailed 安装或加载失败
	PluginStateFailed PluginStateKind = "failed"
)

// PluginRowVO 是一个插件。
type PluginRowVO struct {
	// PluginName npm 包名（插件身份，不随版本变）
	PluginName string `json:"plugin_name"`
	// PluginVersion 已装版本
	PluginVersion string `json:"plugin_version"`
	// PluginSource packages 里的来源串
	PluginSource string `json:"plugin_source"`
	// PluginPath 包目录绝对路径
	PluginPath string `json:"plugin_path"`
	// PluginDesc 一句话说明
	PluginDesc string `json:"plugin_desc,omitempty"`
	// PluginTargetVersion 内置清单 pin 的版本
	PluginTargetVersion string `json:"plugin_target_version,omitempty"`
	// PluginLatest npm registry 的最新版；没查到给空
	PluginLatest string `json:"plugin_latest,omitempty"`
	// PluginUpgradable 已装版本与目标（pin 或 registry 最新）不一致，可升级
	PluginUpgradable bool `json:"plugin_upgradable"`
	// ResourceKinds 包内暴露的资源种类
	ResourceKinds []string `json:"resource_kinds"`
	// PluginBuiltin 是否内置清单里的
	PluginBuiltin bool `json:"plugin_builtin"`
	// PluginEnabled packages 条目是否启用
	PluginEnabled bool `json:"plugin_enabled"`
	// PluginInstalled 包目录是否真的存在
	PluginInstalled bool `json:"plugin_installed"`
	// PluginState 运行期状态
	PluginState PluginStateKind `json:"plugin_state"`
	// PluginError 失败原因
	PluginError string `json:"plugin_error,omitempty"`
}

// PluginListRESP 是插件清单。
type PluginListRESP struct {
	// Plugins 插件行
	Plugins []PluginRowVO `json:"plugins"`
	// RegistryPath settings.json 路径
	RegistryPath string `json:"registry_path"`
	// InstallRoot 插件安装根
	InstallRoot string `json:"install_root"`
	// BuiltinTotal 内置清单总数
	BuiltinTotal int `json:"builtin_total"`
	// BuiltinInstalled 已就绪的内置插件数
	BuiltinInstalled int `json:"builtin_installed"`
	// Busy 是否有安装任务正在占用
	Busy bool `json:"busy"`
}

// PluginSourceREQ 是装插件的入参。
type PluginSourceREQ struct {
	// PluginSource npm 包名或来源串
	PluginSource string `json:"plugin_source"`
}

// PluginNameREQ 是按包名定向操作的入参。
type PluginNameREQ struct {
	// PluginName npm 包名
	PluginName string `json:"plugin_name"`
}

// PluginToggleREQ 是插件开关的入参。
type PluginToggleREQ struct {
	// PluginName npm 包名
	PluginName string `json:"plugin_name"`
	// PluginEnabled 目标状态
	PluginEnabled bool `json:"plugin_enabled"`
}

// PluginInstallRESP 是安装类动作的出参。
type PluginInstallRESP struct {
	// PluginName 装到的包名
	PluginName string `json:"plugin_name"`
	// Installed 成功数（批量动作才有意义）
	Installed int `json:"installed"`
	// Failed 失败数
	Failed int `json:"failed"`
}

// McpServerVO 是一个 MCP 服务器。
type McpServerVO struct {
	// ServerName 服务名
	ServerName string `json:"server_name"`
	// TransportKind stdio / http
	TransportKind string `json:"transport_kind"`
	// CommandLine stdio 类的命令行原文
	CommandLine string `json:"command_line,omitempty"`
	// ServerUrl http 类的地址
	ServerUrl string `json:"server_url,omitempty"`
	// McpDesc 一句话说明
	McpDesc string `json:"mcp_desc,omitempty"`
	// McpWorkDir stdio 类的工作目录；空串用当前目录
	McpWorkDir string `json:"mcp_work_dir,omitempty"`
	// McpEnvText 环境变量回显，逐行 KEY=VALUE
	McpEnvText string `json:"mcp_env_text,omitempty"`
	// McpTimeout 连接超时（秒，pi 的 mcp.json 语义）；0 表示没配
	McpTimeout int `json:"mcp_timeout,omitempty"`
	// McpEnabled 是否启用
	McpEnabled bool `json:"mcp_enabled"`
	// ScopeKind user / project
	ScopeKind string `json:"scope_kind"`
	// ConfigPath 定义所在的 mcp.json
	ConfigPath string `json:"config_path"`
}

// McpListRESP 是 MCP 清单。
type McpListRESP struct {
	// Servers 服务行
	Servers []McpServerVO `json:"servers"`
	// UserPath 用户级 mcp.json 路径
	UserPath string `json:"user_path"`
	// ProjectPath 项目级 mcp.json 路径；未选工作区时为空
	ProjectPath string `json:"project_path,omitempty"`
	// ReadErrors 读取失败原因
	ReadErrors []string `json:"read_errors,omitempty"`
}

// McpSaveREQ 是保存 MCP 服务的入参。
type McpSaveREQ struct {
	// ServerName 服务名
	ServerName string `json:"server_name"`
	// CommandText stdio 类命令行；与 ServerUrl 至少填一个
	CommandText string `json:"command_text"`
	// ServerUrl http 类地址
	ServerUrl string `json:"server_url"`
	// TransportKind stdio / http
	TransportKind string `json:"transport_kind"`
	// McpDesc 一句话说明
	McpDesc string `json:"mcp_desc"`
	// WorkDir stdio 类工作目录；空串清掉该字段
	WorkDir string `json:"work_dir"`
	// EnvList 环境变量，逐行 KEY=VALUE；空表示清掉该字段
	EnvList []string `json:"env_list"`
	// McpTimeout 连接超时（毫秒）；0 表示清掉该字段
	McpTimeout int `json:"mcp_timeout"`
	// McpScope user / project；project 落工作区 .pi\mcp.json，需要已选工作区
	McpScope string `json:"mcp_scope"`
}

// McpProbeRESP 是 MCP 连接测试的结果。
type McpProbeRESP struct {
	// ProbeOk 是否连通（完成 MCP 握手）
	ProbeOk bool `json:"probe_ok"`
	// ProbeMessage 结果一句话（服务名 / 版本或失败原因）
	ProbeMessage string `json:"probe_message"`
	// ProbeTools 握手后数到的工具数；没数到给 0
	ProbeTools int `json:"probe_tools"`
}

// McpToggleREQ 是 MCP 开关的入参。
type McpToggleREQ struct {
	// ServerName 服务名
	ServerName string `json:"server_name"`
	// McpEnabled 目标状态
	McpEnabled bool `json:"mcp_enabled"`
}

// McpNameREQ 是按服务名定向操作的入参。
type McpNameREQ struct {
	// ServerName 服务名
	ServerName string `json:"server_name"`
}
