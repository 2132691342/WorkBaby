// 模型级配置：按「服务 + 模型」覆写目录认不出的参数——
// 上下文窗口、温度、top_p、最大输出与识图 / 工具调用能力。
package domain

// 模型参数缺省值：新建配置未填写时落这组。
const (
	DefaultTemperature = 0.25
	DefaultTopP        = 0.75
)

// ModelConfigDO 模型级配置。ProviderID + Model 复合主键。
// 零值语义：ContextWindow / MaxOutput 为 0 表示跟随内置目录。
type ModelConfigDO struct {
	ProviderID    string  `gorm:"primaryKey;size:64" json:"provider_id"`
	Model         string  `gorm:"primaryKey;size:255" json:"model"`
	ContextWindow int     `gorm:"not null;default:0" json:"context_window"`
	MaxOutput     int     `gorm:"not null;default:0" json:"max_output"`
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	Vision        bool    `json:"vision"`
	ToolCall      bool    `json:"tool_call"`
	UpdatedAt     int64   `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (ModelConfigDO) TableName() string { return "model_configs" }

// UpsertModelConfigREQ 保存模型配置。参数为空时落缺省值。
type UpsertModelConfigREQ struct {
	ProviderID    string  `json:"provider_id"`
	Model         string  `json:"model"`
	ContextWindow int     `json:"context_window"`
	MaxOutput     int     `json:"max_output"`
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	Vision        bool    `json:"vision"`
	ToolCall      bool    `json:"tool_call"`
}

// ModelConfigVO 出参：配置与内置目录合并后的最终值，前端直接显示。
type ModelConfigVO struct {
	ProviderID    string  `json:"provider_id"`
	Model         string  `json:"model"`
	ContextWindow int     `json:"context_window"`
	WindowKnown   bool    `json:"window_known"`
	MaxOutput     int     `json:"max_output"`
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	Vision        bool    `json:"vision"`
	ToolCall      bool    `json:"tool_call"`
	Thinking      bool    `json:"thinking"`
}
