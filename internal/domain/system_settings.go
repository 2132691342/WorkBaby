package domain

import "WorkBaby/internal/pkg"

// SystemSettingDO 是运行时配置 KV 表（runtime KV）；与 config.yaml 不同，这里是用户在 UI 改写即生效。
type SystemSettingDO struct {
	K         string `gorm:"primaryKey;size:128" json:"k"`
	V         string `gorm:"type:text"            json:"v"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (SystemSettingDO) TableName() string { return "system_settings" }

// SystemSettingREQ 写入请求（KV 合并）。
type SystemSettingREQ struct {
	K string `json:"k"`
	V string `json:"v"`
}

// SystemSettingRESP 读取响应。
type SystemSettingRESP struct {
	K         string `json:"k"`
	V         string `json:"v"`
	UpdatedAt int64  `json:"updated_at"`
}

// 常见配置键（运行时变更）。
const (
	SettingKeyChatDefaultTemperature = "chat.defaultTemperature"
	SettingKeyChatDefaultThinking    = "chat.defaultThinking"
	SettingKeyChatCompressionRatio   = "chat.compressionRatio"
	SettingKeyChatMaxInputChars      = "chat.maxInputChars"
	// SettingKeyChatMaxRunTokens 单次 run 的 token 花费上限（0/缺省 = 不限）。
	SettingKeyChatMaxRunTokens = "chat.maxRunTokens"
	// SettingKeyMemoryEnabled 全局记忆开关（true/false；缺省开启）。
	SettingKeyMemoryEnabled = "memory.enabled"
)

// WebSearch 配置键（websearch 工具）。
const (
	SettingKeyWebSearchEnabled = "websearch.enabled"
	SettingKeyWebSearchEngine  = "websearch.engine"
)

// SettingKeyAgentSessionMode 工具策略门会话模式（default / auto_edit / yolo；见 tool.SessionMode）。
const SettingKeyAgentSessionMode = "agent.session_mode"

// SettingKeyTrayCloseToTray 关闭主窗口行为（"true" = 隐藏到托盘常驻，"false"/缺省 = 退出应用）。
const SettingKeyTrayCloseToTray = "tray.close_to_tray"

// 通用设置键（settings/general：主题 / 背景 / 字体等前端外观偏好，JSON 值）。
const SettingKeyGeneral = "settings.general"

// WebSearchConfigRESP 联网搜索配置（settings/websearch；DuckDuckGo 无需 apiKey）。
type WebSearchConfigRESP struct {
	Enabled string `json:"enabled"`
	Engine  string `json:"engine"`
	APIKey  string `json:"api_key"`
}

// 默认值（service 解析 system_settings 缺失项时的回退；前端 store 也用同一常量做 fallback，
// 避免前后端默认不一致）。MAX_INPUT_CHARS 不在 system_settings 时落 32000。
const (
	DefaultChatMaxInputChars = 32000
)

// SettingKeyToolEnabledPrefix 工具启停键前缀：tool.enabled.{name} = "true"/"false"。
const SettingKeyToolEnabledPrefix = "tool.enabled."

// SettingKeyExecWhitelist exec 工具二进制白名单（JSON 数组）。
const SettingKeyExecWhitelist = "tool.exec.whitelist"

// VersionInfo 给前端的版本快照；启动期判定 phase 显示。
type VersionInfo struct {
	AppName       string `json:"app_name"`
	Version       string `json:"version"`
	Phase         string `json:"phase"`
	Env           string `json:"env"`
	DefaultTenant string `json:"default_tenant"`
	LocalUserID   string `json:"local_user_id"`
}

// HealthInfo 接口用；运行时用于前端确认后端就绪。
type HealthInfo struct {
	Status    string `json:"status"` // ok | degraded
	Phase     string `json:"phase"`
	UptimeMs  int64  `json:"uptime_ms"`
	DBEnabled bool   `json:"db_enabled"`
	Providers int    `json:"providers"`
}

// 包级错误变量（CLAUDE §2.4）：错误码 2010 段（2000-2999 配置/持久化）。
var (
	ErrSettingKeyEmpty = pkg.New(2014, "setting key is empty", "")
	ErrSettingNotFound = pkg.New(2015, "setting not found", "")
)
