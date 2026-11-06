package domain

// EngineKind 是引擎种类。本壳只装一个引擎，但保留这个字段让界面与路由的形状
// 不依赖「只有一个」这件事。
type EngineKind string

const (
	// EnginePi pi 引擎（唯一实现）
	EnginePi EngineKind = "pi"
)

// EngineCaps 是能力表：界面按它隐藏入口，而不是渲染一堆点不动的灰按钮。
type EngineCaps struct {
	// Subagents 子代理面板
	Subagents bool `json:"subagents"`
	// Goals 目标追踪
	Goals bool `json:"goals"`
	// Todos 实时待办
	Todos bool `json:"todos"`
	// Login OAuth 登录流
	Login bool `json:"login"`
	// QueueClear 整队清空
	QueueClear bool `json:"queue_clear"`
	// QueueEdit 单条队列编辑（pi 不支持单删）
	QueueEdit bool `json:"queue_edit"`
	// SkillToggle 技能开关
	SkillToggle bool `json:"skill_toggle"`
	// Extensions 扩展管理
	Extensions bool `json:"extensions"`
	// ExportHTML 会话导出为单文件 HTML
	ExportHTML bool `json:"export_html"`
	// SlashCommands 斜杠命令清单
	SlashCommands bool `json:"slash_commands"`
	// SessionClone 复制当前会话开新分支
	SessionClone bool `json:"session_clone"`
	// PluginToggle 插件启停
	PluginToggle bool `json:"plugin_toggle"`
	// PluginUpgrade 插件升级
	PluginUpgrade bool `json:"plugin_upgrade"`
}

// PiCaps 是 pi 引擎的能力表：RPC 协议能给到的那些。
var PiCaps = EngineCaps{
	QueueClear:    true,
	SkillToggle:   true,
	Extensions:    true,
	ExportHTML:    true,
	SlashCommands: true,
	SessionClone:  true,
	PluginToggle:  true,
	PluginUpgrade: true,
}

// EngineIdentity 是一个引擎的身份事实。
type EngineIdentity struct {
	// EngineKind 引擎种类
	EngineKind EngineKind `json:"engine_kind"`
	// EngineLabel 界面上的名字
	EngineLabel string `json:"engine_label"`
	// EngineExe 入口文件绝对路径
	EngineExe string `json:"engine_exe"`
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
	// RootDir 引擎根目录
	RootDir string `json:"root_dir"`
	// AgentDir 引擎家目录
	AgentDir string `json:"agent_dir"`
	// SessionsDir 会话目录
	SessionsDir string `json:"sessions_dir"`
	// SkillsDir 技能目录
	SkillsDir string `json:"skills_dir"`
	// PluginsDir 插件安装根
	PluginsDir string `json:"plugins_dir"`
	// SettingsFile 设置文件
	SettingsFile string `json:"settings_file"`
	// DirExists 根目录是否存在
	DirExists bool `json:"dir_exists"`
}

// AppRowVO 是应用页的一行：一个运行时 / 引擎的安装事实与可做的动作。
type AppRowVO struct {
	// AppKey 应用标识（node / git / python / pi）
	AppKey string `json:"app_key"`
	// AppName 界面上的名字
	AppName string `json:"app_name"`
	// AppVersion 已装版本；空表示没装
	AppVersion string `json:"app_version"`
	// AppPath 可执行文件绝对路径
	AppPath string `json:"app_path"`
	// Installed 是否已就绪
	Installed bool `json:"installed"`
	// Builtin 是否随包分发（内置运行时与 pi 都是）
	Builtin bool `json:"builtin"`
	// CanInstall 是否提供安装动作
	CanInstall bool `json:"can_install"`
	// CanUpdate 是否提供升级动作
	CanUpdate bool `json:"can_update"`
	// CanUninstall 是否提供卸载动作
	CanUninstall bool `json:"can_uninstall"`
	// Busy 是否有安装任务正在占用它
	Busy bool `json:"busy"`
	// Blocker 不能操作的原因；空表示没阻碍
	Blocker string `json:"blocker,omitempty"`
	// InstallNote 安装动作的一句话说明
	InstallNote string `json:"install_note,omitempty"`
}

// AppProgressVO 是一条安装进度帧（SSE app:progress）。
type AppProgressVO struct {
	// ProgressKind 进度所属的应用键（node / pi / plugin）
	ProgressKind string `json:"progress_kind"`
	// ProgressAction 动作（install / update / uninstall / plugin-install）
	ProgressAction string `json:"progress_action"`
	// ProgressPhase 阶段：start / line / done / fail
	ProgressPhase string `json:"progress_phase"`
	// ProgressLine 命令输出的一行
	ProgressLine string `json:"progress_line,omitempty"`
	// ProgressMessage 给人看的一句话
	ProgressMessage string `json:"progress_message,omitempty"`
	// PluginName 插件进度时才有的包名
	PluginName string `json:"plugin_name,omitempty"`
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
	// Runtime 内置运行时状态
	Runtime RuntimeStatusVO `json:"runtime"`
	// Prefs 应用偏好
	Prefs AppPrefsVO `json:"prefs"`
}

// RuntimeStatusVO 是三个内置运行时的就绪状态。
type RuntimeStatusVO struct {
	// NodeReady 内置 Node.js 是否就绪
	NodeReady bool `json:"node_ready"`
	// NodeVersion 内置 Node.js 版本
	NodeVersion string `json:"node_version"`
	// GitReady 内置 git 是否就绪
	GitReady bool `json:"git_ready"`
	// GitVersion 内置 git 版本
	GitVersion string `json:"git_version"`
	// PythonReady 内置 Python 是否就绪
	PythonReady bool `json:"python_ready"`
	// PythonVersion 内置 Python 版本
	PythonVersion string `json:"python_version"`
	// PiReady pi 本体是否就绪
	PiReady bool `json:"pi_ready"`
	// PiVersion pi 本体版本
	PiVersion string `json:"pi_version"`
	// Provisioning 是否有解压 / 安装正在后台进行
	Provisioning bool `json:"provisioning"`
}

// RuntimeRepairRESP 是运行时修复的结果。
type RuntimeRepairRESP struct {
	// RepairProblems 修复过程中的失败原因；空数组即全部成功
	RepairProblems []string `json:"repair_problems"`
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
	// TotalPlugins 插件总数
	TotalPlugins int `json:"total_plugins"`
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
	// PluginCount 已装且启用的插件数
	PluginCount int `json:"plugin_count"`
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
