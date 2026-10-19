// 设置项聚合根：WorkBaby 自己的界面偏好、背景图与缺省值。
// PI 侧的配置（默认模型、工具集、压缩参数）不在这里——它们属于 settings.json。
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
	SettingTheme          = "theme"
	SettingFontSize       = "font_size"
	SettingFontFamily     = "font_family"
	SettingWorkspace      = "workspace"
	SettingMinimizeToTray = "minimize_to_tray"
	SettingSendOnEnter    = "send_on_enter"
	SettingShowThinking   = "show_thinking"
	// 界面语言：zh-CN / en，前端 i18n 与 Element Plus 文案都读它。
	SettingLanguage = "language"
	// 自定义背景：bg_image 存数据目录内副本的绝对路径，另两项是叠层参数。
	SettingBgImage   = "bg_image"
	SettingBgOpacity = "bg_opacity"
	SettingBgBlur    = "bg_blur"
)

// 自定义背景的取值边界；图片必须复制进数据目录，不能直接引用用户选中的原文件。
const (
	MaxBgBytes = 8 << 20
	// MaxBgOpacity 是「图片强度」上限：之上永远压着一层遮罩，正文不被背景吃掉。
	MaxBgOpacity = 0.9
	MaxBgBlurPx  = 24
)

// BackgroundREQ 设置自定义背景入参；Path 为空表示清除背景。
type BackgroundREQ struct {
	Path    string  `json:"path"`
	Opacity float64 `json:"opacity"`
	Blur    float64 `json:"blur"`
}

// BackgroundVO 自定义背景出参；Image 为空表示没有背景。
type BackgroundVO struct {
	Image   string  `json:"image"`
	Opacity float64 `json:"opacity"`
	Blur    float64 `json:"blur"`
}

// PISettingsVO 是 PI 侧配置的只读快照：界面要能回答「我的配置到底在哪」。
type PISettingsVO struct {
	DefaultProvider      string   `json:"default_provider"`
	DefaultModel         string   `json:"default_model"`
	DefaultThinkingLevel string   `json:"default_thinking_level"`
	DefaultTools         []string `json:"default_tools"`
	CompactionReserve    int      `json:"compaction_reserve"`
	CompactionKeep       int      `json:"compaction_keep"`

	AgentDir      string `json:"agent_dir"`
	SettingsPath  string `json:"settings_path"`
	ModelsPath    string `json:"models_path"`
	MCPPath       string `json:"mcp_path"`
	SessionsDir   string `json:"sessions_dir"`
	ExtensionsDir string `json:"extensions_dir"`
	SkillsDir     string `json:"skills_dir"`
}

// SetCompactionREQ 设置 PI 的压缩参数；0 表示恢复默认。
type SetCompactionREQ struct {
	Reserve int `json:"reserve"`
	Keep    int `json:"keep"`
}

// WorkspacesRESP 最近用过的工作目录。
type WorkspacesRESP struct {
	Workspaces []string `json:"workspaces"`
	Current    string   `json:"current"`
}

// DefaultSettings 首启写入的默认值。
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingTheme:          "light",
		SettingFontSize:       "md",
		SettingMinimizeToTray: "true",
		SettingFontFamily:     "system",
		SettingSendOnEnter:    "true",
		SettingShowThinking:   "true",
		SettingLanguage:       "zh-CN",
	}
}
