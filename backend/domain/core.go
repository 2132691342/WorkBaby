package domain

// EngineKind 是引擎种类。引擎已经内置在本进程里，保留这个字段
// 让界面与路由的形状不依赖「只有一个」这件事。
type EngineKind string

const (
	// EngineNative 内置原生引擎（唯一实现）
	EngineNative EngineKind = "native"
)

// EngineCaps 是能力表：界面按它隐藏入口，而不是渲染一堆点不动的灰按钮。
type EngineCaps struct {
	// Subagents 子代理面板
	Subagents bool `json:"subagents"`
	// Goals 目标追踪
	Goals bool `json:"goals"`
	// Todos 实时待办
	Todos bool `json:"todos"`
	// QueueClear 整队清空
	QueueClear bool `json:"queue_clear"`
	// QueueEdit 单条队列编辑
	QueueEdit bool `json:"queue_edit"`
	// SkillToggle 技能开关
	SkillToggle bool `json:"skill_toggle"`
	// ExportHTML 会话导出为单文件 HTML
	ExportHTML bool `json:"export_html"`
	// SlashCommands 斜杠命令清单
	SlashCommands bool `json:"slash_commands"`
	// SessionClone 复制当前会话开新分支
	SessionClone bool `json:"session_clone"`
}

// NativeCaps 是内置引擎的能力表。
// 能力表只列界面真正能用的功能。
var NativeCaps = EngineCaps{
	Todos:         true,
	QueueClear:    true,
	QueueEdit:     true,
	SkillToggle:   true,
	ExportHTML:    true,
	SlashCommands: true,
	SessionClone:  true,
}

// EngineIdentity 是一个引擎的身份事实。
type EngineIdentity struct {
	// EngineKind 引擎种类
	EngineKind EngineKind `json:"engine_kind"`
	// EngineLabel 界面上的名字
	EngineLabel string `json:"engine_label"`
	// EngineExe 可直接执行的入口路径
	EngineExe string `json:"engine_exe"`
	// EngineScript 需要 node 解释的 CLI 脚本；空表示入口本身可执行
	EngineScript string `json:"engine_script,omitempty"`
	// EngineSource 发现方式：manual / path / npm-global / none
	EngineSource string `json:"engine_source"`
	// EngineVersion 版本
	EngineVersion string `json:"engine_version"`
	// Installed 是否已安装
	Installed bool `json:"installed"`
	// Caps 能力表
	Caps EngineCaps `json:"caps"`
}

// EngineLayoutVO 是引擎落盘布局的展示版。
type EngineLayoutVO struct {
	// EngineKind 引擎种类
	EngineKind EngineKind `json:"engine_kind"`
	// AgentDir 引擎数据根目录
	AgentDir string `json:"agent_dir"`
	// SessionsDir 会话目录
	SessionsDir string `json:"sessions_dir"`
	// SkillsDir 技能目录
	SkillsDir string `json:"skills_dir"`
	// SettingsFile 设置文件
	SettingsFile string `json:"settings_file"`
	// DirExists 根目录是否存在
	DirExists bool `json:"dir_exists"`
}

// AppRowVO 是应用页的一行：一个外部依赖的检测事实与可做的动作。
type AppRowVO struct {
	// AppKey 应用标识（node / git / python）
	AppKey string `json:"app_key"`
	// AppName 界面上的名字
	AppName string `json:"app_name"`
	// AppVersion 已装版本；空表示没装
	AppVersion string `json:"app_version"`
	// AppPath 可执行文件绝对路径
	AppPath string `json:"app_path"`
	// Installed 是否已就绪
	Installed bool `json:"installed"`
	// Required 是否为本应用的硬依赖
	Required bool `json:"required"`
	// AppSource 发现方式：manual / path / npm-global / none
	AppSource string `json:"app_source"`
	// CanLocate 是否提供「指定位置」动作
	CanLocate bool `json:"can_locate"`
	// Blocker 不能操作的原因；空表示没阻碍
	Blocker string `json:"blocker,omitempty"`
	// InstallNote 装不上时告诉用户该去装什么；壳不代装
	InstallNote string `json:"install_note,omitempty"`
}

// AppProgressVO 是一条安装进度帧（SSE app:progress）。
type AppProgressVO struct {
	// ProgressKind 进度所属的应用键（node / git / python）
	ProgressKind string `json:"progress_kind"`
	// ProgressAction 动作（install / update / uninstall）
	ProgressAction string `json:"progress_action"`
	// ProgressPhase 阶段：start / line / done / fail
	ProgressPhase string `json:"progress_phase"`
	// ProgressLine 命令输出的一行
	ProgressLine string `json:"progress_line,omitempty"`
	// ProgressMessage 给人看的一句话
	ProgressMessage string `json:"progress_message,omitempty"`
}

// BootBootstrapRESP 是首屏总包：界面一次请求拿到画首屏要用的全部事实。
type BootBootstrapRESP struct {
	// AppVersion 壳版本
	AppVersion string `json:"app_version"`
	// AppContract 响应契约版本
	AppContract int `json:"app_contract"`
	// Engines 引擎身份
	Engines []EngineIdentity `json:"engines"`
	// ActiveEngine 当前引擎
	ActiveEngine EngineKind `json:"active_engine"`
	// Workspace 当前工作区
	Workspace string `json:"workspace"`
	// Settings 全部设置键值
	Settings map[string]string `json:"settings"`
	// Layouts 引擎布局
	Layouts []EngineLayoutVO `json:"layouts"`
	// Workspaces 工作区历史
	Workspaces []WorkspaceRowVO `json:"workspaces"`
	// Runtime 外部依赖的检测状态
	Runtime RuntimeStatusVO `json:"runtime"`
	// Prefs 应用偏好
	Prefs AppPrefsVO `json:"prefs"`
}

// RuntimeStatusVO 是外部依赖的检测状态。
// 引擎在进程内，不需要探测；这里只报用户环境里的工作区工具链。
type RuntimeStatusVO struct {
	// NodeReady 是否找到 node（部分技能的脚本要用）
	NodeReady bool `json:"node_ready"`
	// GitReady 是否找到 git（工作区的版本控制能力）
	GitReady bool `json:"git_ready"`
	// GitVersion git 版本
	GitVersion string `json:"git_version"`
	// PythonReady 是否找到 python（部分技能要用）
	PythonReady bool `json:"python_ready"`
	// EngineReady 引擎是否可用：内置引擎恒为真
	EngineReady bool `json:"engine_ready"`
	// DetectionHint 缺依赖时给用户的一句话指引
	DetectionHint string `json:"detection_hint,omitempty"`
	// Detecting 是否有探测正在后台进行
	Detecting bool `json:"detecting"`
}

// WebStatusVO 是联网能力的当前状态。
type WebStatusVO struct {
	// WebKind 搜索服务种类
	WebKind string `json:"web_kind"`
	// WebReady 是否可用
	WebReady bool `json:"web_ready"`
	// WebMessage 给界面看的一句话
	WebMessage string `json:"web_message"`
	// WebHasKey 是否已配置密钥
	WebHasKey bool `json:"web_has_key"`
}

// IndexStatusVO 是工作区预索引的状态。
type IndexStatusVO struct {
	// IndexWorkspace 当前索引对应的工作区
	IndexWorkspace string `json:"index_workspace"`
	// IndexDocCount 索引里的文档数
	IndexDocCount int `json:"index_doc_count"`
	// IndexBuilt 是否已建过索引
	IndexBuilt bool `json:"index_built"`
	// IndexBuilding 是否正在建索引
	IndexBuilding bool `json:"index_building"`
	// IndexBuiltAt 上次建索引的时刻（毫秒）
	IndexBuiltAt int64 `json:"index_built_at"`
}

// SearchHitVO 是一条检索结果。
type SearchHitVO struct {
	// DocPath 相对工作区的路径
	DocPath string `json:"doc_path"`
	// DocKind 种类：file / symbol
	DocKind string `json:"doc_kind"`
	// DocSymbol 符号名
	DocSymbol string `json:"doc_symbol,omitempty"`
	// DocSymbolKind 符号种类
	DocSymbolKind string `json:"doc_symbol_kind,omitempty"`
	// DocLine 符号所在行号
	DocLine int `json:"doc_line,omitempty"`
	// HitScore 排序分
	HitScore float64 `json:"hit_score"`
	// HitReason 为什么命中
	HitReason string `json:"hit_reason,omitempty"`
}

// IndexSearchREQ 是工作区检索的入参。
type IndexSearchREQ struct {
	// SearchQuery 检索词
	SearchQuery string `json:"search_query"`
	// SearchLimit 最多返回几条
	SearchLimit int `json:"search_limit"`
}

// IndexSearchRESP 是检索结果清单。
type IndexSearchRESP struct {
	// SearchHits 命中行
	SearchHits []SearchHitVO `json:"search_hits"`
	// IndexStatus 索引状态
	IndexStatus IndexStatusVO `json:"index_status"`
}

// WebSearchREQ 是联网搜索的入参。
type WebSearchREQ struct {
	// SearchQuery 搜索词
	SearchQuery string `json:"search_query"`
	// SearchLimit 最多返回几条
	SearchLimit int `json:"search_limit"`
}

// WebSearchItemVO 是一条搜索结果。
type WebSearchItemVO struct {
	// ResultTitle 标题
	ResultTitle string `json:"result_title"`
	// ResultURL 地址
	ResultURL string `json:"result_url"`
	// ResultSnippet 摘要
	ResultSnippet string `json:"result_snippet"`
	// ResultSource 来源站点
	ResultSource string `json:"result_source,omitempty"`
}

// WebSearchRESP 是搜索结果清单。
type WebSearchRESP struct {
	// SearchResults 结果行
	SearchResults []WebSearchItemVO `json:"search_results"`
	// SearchProvider 用的哪个搜索服务
	SearchProvider string `json:"search_provider"`
}

// WebFetchREQ 是抓取网页的入参。
type WebFetchREQ struct {
	// FetchUrl 要抓的地址
	FetchUrl string `json:"fetch_url"`
}

// WebFetchRESP 是抓取结果。
type WebFetchRESP struct {
	// FetchUrl 最终地址
	FetchUrl string `json:"fetch_url"`
	// FetchText 可读正文
	FetchText string `json:"fetch_text"`
	// FetchCharCount 正文字数
	FetchCharCount int `json:"fetch_char_count"`
}

// WebSettingsREQ 是保存联网设置的入参。
type WebSettingsREQ struct {
	// WebKind 搜索服务种类
	WebKind string `json:"web_kind"`
	// WebApiKey 密钥；空串 = 保留原值
	WebApiKey string `json:"web_api_key"`
}

// WebStatusRESP 是联网设置与状态。
type WebStatusRESP struct {
	// WebKind 搜索服务种类
	WebKind string `json:"web_kind"`
	// WebHasKey 是否已配置密钥
	WebHasKey bool `json:"web_has_key"`
	// WebMessage 给界面看的一句话
	WebMessage string `json:"web_message"`
	// WebKinds 可选的搜索服务种类
	WebKinds []string `json:"web_kinds"`
}

// WorkspaceRowVO 是一条工作区历史。
type WorkspaceRowVO struct {
	// WorkspacePath 目录绝对路径
	WorkspacePath string `json:"workspace_path"`
	// WorkspaceExists 目录是否还在
	WorkspaceExists bool `json:"workspace_exists"`
	// WorkspaceIsRepo 是否处于 git 仓库内
	WorkspaceIsRepo bool `json:"workspace_is_repo"`
	// WorkspaceBranch 当前分支名；非仓库为空
	WorkspaceBranch string `json:"workspace_branch,omitempty"`
	// WorkspaceDirty 未提交改动数量
	WorkspaceDirty int `json:"workspace_dirty"`
	// SessionCount 该工作区下的会话数
	SessionCount int `json:"session_count"`
	// UpdatedAt 最近一次使用时刻（毫秒）
	UpdatedAt int64 `json:"updated_at"`
}

// AppPrefsVO 是设置页看到的全部开关状态。
type AppPrefsVO struct {
	// PrefValues 全部设置键值
	PrefValues map[string]string `json:"pref_values"`
	// AutoStart 开机自启（以注册表为准）
	AutoStart bool `json:"auto_start"`
	// RestoreSession 启动时恢复上次会话
	RestoreSession bool `json:"restore_session"`
	// LastSessionFile 上次会话文件
	LastSessionFile string `json:"last_session_file"`
	// CloseAction 点关闭按钮时做什么：tray / quit
	CloseAction string `json:"close_action"`
	// DataDir 数据目录
	DataDir string `json:"data_dir"`
	// LogDir 日志目录
	LogDir string `json:"log_dir"`
}

// ConfigFileVO 是一个可查看的引擎配置文件。
type ConfigFileVO struct {
	// ConfigName 键名（settings / models / mcp / auth / agents）
	ConfigName string `json:"config_name"`
	// ConfigLabel 界面上的名字
	ConfigLabel string `json:"config_label"`
	// FilePath 绝对路径
	FilePath string `json:"file_path"`
	// FileExists 是否已生成
	FileExists bool `json:"file_exists"`
	// SizeBytes 字节数
	SizeBytes int64 `json:"size_bytes"`
}

// ConfigTextRESP 是配置文件原文；Found 为假表示还没生成过，不是错误。
type ConfigTextRESP struct {
	// FilePath 绝对路径
	FilePath string `json:"file_path"`
	// FileText 原文
	FileText string `json:"file_text"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
	// Found 是否存在
	Found bool `json:"found"`
}

// HealthRESP 是概览页首屏要用的全部事实。
type HealthRESP struct {
	// HealthEngines 引擎健康快照
	HealthEngines []EngineHealthVO `json:"health_engines"`
	// ActiveEngine 当前引擎
	ActiveEngine EngineKind `json:"active_engine"`
	// Workspace 当前工作区
	Workspace string `json:"workspace"`
	// Runtime 内置运行时状态
	Runtime RuntimeStatusVO `json:"runtime"`
	// TotalSessions 会话总数
	TotalSessions int `json:"total_sessions"`
	// TotalSkills 技能总数
	TotalSkills int `json:"total_skills"`
	// TotalMcp MCP 服务总数
	TotalMcp int `json:"total_mcp"`
	// LiveEngines 存活引擎实例数
	LiveEngines int `json:"live_engines"`
	// MemoryUsedMb 本进程内存占用（MB）
	MemoryUsedMb int `json:"memory_used_mb"`
	// DetectedAt 上次探测完成的时刻（毫秒）
	DetectedAt int64 `json:"detected_at"`
}

// EngineHealthVO 是单个引擎的健康快照。
type EngineHealthVO struct {
	// EngineKind 引擎种类
	EngineKind EngineKind `json:"engine_kind"`
	// EngineLabel 界面上的名字
	EngineLabel string `json:"engine_label"`
	// EngineExe 入口路径
	EngineExe string `json:"engine_exe"`
	// EngineVersion 版本
	EngineVersion string `json:"engine_version"`
	// Installed 是否已安装
	Installed bool `json:"installed"`
	// Running 是否有存活实例
	Running bool `json:"running"`
	// Busy 是否正在跑一轮
	Busy bool `json:"busy"`
	// ProcessId 子进程 PID
	ProcessId int `json:"process_id"`
	// StartedAt 子进程启动时刻（毫秒）
	StartedAt int64 `json:"started_at"`
	// CallCount 已发出的命令数
	CallCount int `json:"call_count"`
	// LastError 最近一次失败原因
	LastError string `json:"last_error,omitempty"`
	// SessionCount 会话数
	SessionCount int `json:"session_count"`
	// SkillCount 启用态技能数
	SkillCount int `json:"skill_count"`
	// McpCount MCP 服务数
	McpCount int `json:"mcp_count"`
	// ConfigFiles 可查看的配置文件
	ConfigFiles []ConfigFileVO `json:"config_files"`
	// Caps 能力表
	Caps EngineCaps `json:"caps"`
}

// SetWorkspaceREQ 是切工作区的入参。
type SetWorkspaceREQ struct {
	// Workspace 目标目录绝对路径
	Workspace string `json:"workspace"`
}

// WorkspaceListRESP 是工作区清单。
type WorkspaceListRESP struct {
	// Workspace 当前工作区
	Workspace string `json:"workspace"`
	// WorkspaceRows 工作区行，最近打开在前
	WorkspaceRows []WorkspaceRowVO `json:"workspace_rows"`
	// ActiveEngine 当前引擎
	ActiveEngine EngineKind `json:"active_engine"`
}

// WorkspaceForgetREQ 是「从清单里移除一个工作区」的入参。
type WorkspaceForgetREQ struct {
	// WorkspacePath 要去掉的工作区路径
	WorkspacePath string `json:"workspace_path"`
}

// SetWorkspaceRESP 是切工作区的出参。
type SetWorkspaceRESP struct {
	// Workspace 生效后的工作区
	Workspace string `json:"workspace"`
}

// SaveSettingsREQ 是保存设置的入参：键值对象，键名即设置项。
type SaveSettingsREQ map[string]string

// PrefsToggleREQ 是偏好开关的通用入参。
type PrefsToggleREQ struct {
	// PrefEnabled 开关目标值
	PrefEnabled bool `json:"pref_enabled"`
}

// CloseActionREQ 是关窗行为的入参。
type CloseActionREQ struct {
	// CloseAction tray / quit
	CloseAction string `json:"close_action"`
}

// RememberSessionREQ 是记住上次会话的入参。
type RememberSessionREQ struct {
	// SessionFile 会话文件绝对路径
	SessionFile string `json:"session_file"`
}

// OpenTargetREQ 是「用系统程序打开 / 在资源管理器定位」的入参。
type OpenTargetREQ struct {
	// TargetPath 目标绝对路径
	TargetPath string `json:"target_path"`
}

// OkRESP 是只要一个成功标志的通用出参。
type OkRESP struct {
	// Ok 是否成功
	Ok bool `json:"ok"`
}
