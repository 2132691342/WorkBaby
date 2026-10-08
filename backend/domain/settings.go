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
	SettingMinimizeToTray   = "minimize_to_tray"
	SettingLaunchOnLogin    = "launch_on_login"
	SettingFontFamily       = "font_family"
	SettingDensity          = "density"
	SettingSendOnEnter      = "send_on_enter"
	SettingShowThinking     = "show_thinking"
)

// DefaultSettings 首启写入的默认值。
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingTheme:          "light",
		SettingFontSize:       "md",
		SettingPermission:     PermissionAsk,
		SettingContextReserve: "16384",
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
