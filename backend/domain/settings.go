// 设置项聚合根：KV 表、可写键白名单、背景图与全部缺省值。
package domain

import "strconv"

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
	SettingContextReserve  = "context_reserve_tokens"
	SettingContextWindow   = "context_window"
	SettingMinimizeToTray  = "minimize_to_tray"
	SettingFontFamily      = "font_family"
	SettingSendOnEnter     = "send_on_enter"
	SettingShowThinking    = "show_thinking"
	// 压缩参数：直接写进 PI 的 settings.json（compaction.reserveTokens / keepRecentTokens）。
	SettingCompactionReserve = "compaction_reserve_tokens"
	SettingCompactionKeep    = "compaction_keep_recent"
	// 思考档位：off/minimal/low/medium/high/xhigh/max，写 PI 的 defaultThinkingLevel。
	SettingThinkingLevel = "thinking_level"
	// 隐藏会话（子代理）的保留期（天），0 = 永久保留：过期只丢逐字对话链。
	SettingRetentionDays = "orchestra_retention_days"
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
	Opacity float64 `json:"opacity"` // 0–0.9，图片强度
	Blur    float64 `json:"blur"`    // 0–24 px
}

// BackgroundVO 自定义背景出参；Image 为空表示没有背景。
type BackgroundVO struct {
	Image   string  `json:"image"`
	Opacity float64 `json:"opacity"`
	Blur    float64 `json:"blur"`
}

// Agent 运行护栏缺省值：轮数与并发都是内核参数，现在由 PI 管；
// 这里只留 WorkBaby 自己的保留期。
const (
	// DefaultRetentionDays 隐藏会话默认保留 30 天，0 表示永不归档。
	DefaultRetentionDays = 30
	// MaxRetentionDays 保留期上限（10 年）。
	MaxRetentionDays = 3650
)

// DefaultSettings 首启写入的默认值；context_reserve_tokens 留空表示跟随模型真实输出预算。
func DefaultSettings() map[string]string {
	return map[string]string{
		SettingTheme:          "light",
		SettingFontSize:       "md",
		SettingPermission:     PermissionAskWrite,
		SettingContextReserve: "",
		SettingDisabledTools:  "",
		SettingMinimizeToTray: "true",
		SettingFontFamily:     "system",
		SettingSendOnEnter:    "true",
		SettingShowThinking:   "true",
		SettingRetentionDays:  strconv.Itoa(DefaultRetentionDays),
	}
}
