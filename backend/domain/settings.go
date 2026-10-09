package domain

// SettingDO 键值设置。值统一存文本，读取方自行解析。
type SettingDO struct {
	Key       string `gorm:"primaryKey;size:64" json:"key"`
	Value     string `gorm:"type:text" json:"value"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (SettingDO) TableName() string { return "settings" }

// SetSettingREQ 写入设置入参。
type SetSettingREQ struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// 设置键。
const (
	SettingTheme           = "theme"
	SettingFontSize        = "font_size"
	SettingWorkspace       = "workspace"
	SettingPermission      = "permission"
	SettingDefaultProvider = "default_provider"
	SettingDefaultModel    = "default_model"
	SettingDisabledTools   = "disabled_tools"
	SettingDisabledSkills  = "disabled_skills"
	SettingContextReserve  = "context_reserve_tokens"
	SettingContextWindow   = "context_window"
	SettingMinimizeToTray  = "minimize_to_tray"
	SettingLaunchOnLogin   = "launch_on_login"
	SettingFontFamily      = "font_family"
	SettingDensity         = "density"
	SettingSendOnEnter     = "send_on_enter"
	SettingShowThinking    = "show_thinking"
	SettingMaxTurns        = "max_turns"
	SettingToolParallel    = "tool_parallel"
	SettingStreamIdleSec   = "stream_idle_seconds"
)

// Agent 运行护栏的缺省值：仅在设置表没有显式覆写时生效。
// 轮数只是失控护栏，正常任务由「模型不再发起工具调用」自然结束；
// 多文件批处理一步一轮，64 轮是实测长任务不会误停的下限。
// 工具并发 4 是磁盘 IO 的甜点，再高对本地读文件没有收益。
const (
	DefaultMaxTurns     = 64
	DefaultToolParallel = 4
)

// DefaultSettings 首启写入的默认值。
// context_reserve_tokens 留空 = 跟随模型真实输出预算（见 service.budget），
// 写死一个数字会让换模型后「余量」与「实际下发输出」脱节。
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingTheme:          "light",
		SettingFontSize:       "md",
		SettingPermission:     PermissionAsk,
		SettingContextReserve: "",
		SettingDisabledTools:  "",
		SettingDisabledSkills: "",
		SettingMinimizeToTray: "true",
		SettingLaunchOnLogin:  "false",
		SettingFontFamily:     "system",
		SettingDensity:        "comfortable",
		SettingSendOnEnter:    "true",
		SettingShowThinking:   "true",
	}
}
