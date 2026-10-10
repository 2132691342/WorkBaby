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
	SettingFontFamily      = "font_family"
	SettingSendOnEnter     = "send_on_enter"
	SettingShowThinking    = "show_thinking"
	SettingMaxTurns        = "max_turns"
	SettingToolParallel    = "tool_parallel"
	SettingStreamIdleSec   = "stream_idle_seconds"
	// 自定义背景：bg_image 存数据目录内副本的绝对路径，另两项是叠层参数。
	SettingBgImage   = "bg_image"
	SettingBgOpacity = "bg_opacity"
	SettingBgBlur    = "bg_blur"
)

// 自定义背景的取值边界。图片要复制到数据目录再引用：直接引用用户选中的原文件，
// 原文件一挪走背景就没了，而用户完全不记得这层关系。
const (
	MaxBgBytes = 8 << 20
	// bg_opacity 是「图片强度」：0 = 几乎看不见，1 = 原图。上限 0.9 保证
	// 之上始终压着一层至少 0.1 的遮罩，正文不会被背景吃掉。
	MaxBgOpacity = 0.9
	MaxBgBlurPx  = 24
)

// BackgroundREQ 设置自定义背景入参；空路径表示清除。
type BackgroundREQ struct {
	Path    string  `json:"path"`
	Opacity float64 `json:"opacity"`
	Blur    float64 `json:"blur"`
}

// BackgroundVO 自定义背景出参。
type BackgroundVO struct {
	Image   string  `json:"image"`
	Opacity float64 `json:"opacity"`
	Blur    float64 `json:"blur"`
}

// Agent 运行护栏的缺省值，仅在设置表没有显式覆写时生效：轮数只是失控护栏
// （多文件批处理一步一轮，64 轮是实测长任务不会误停的下限）；工具并发 4 是
// 磁盘 IO 的甜点，再高对本地读文件没有收益。
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
		SettingFontFamily:     "system",
		SettingSendOnEnter:    "true",
		SettingShowThinking:   "true",
	}
}
