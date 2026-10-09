// 模型级配置：按「服务 + 模型」覆写目录认不出的参数——
// 上下文窗口、温度、top_p、最大输出与识图 / 工具调用能力。
package domain

// SamplingUnset 是采样参数的「未设置」值：负值在协议层面非法，正好当哨兵。
// 不能用 0 表达未设置——0 是合法的确定性取值，要允许用户显式设 0。
const SamplingUnset = -1.0

// ClampSampling 归一采样参数：负值一律视为未设置，超范围的值夹回边界。
func ClampSampling(v float64) float64 {
	if v < 0 {
		return SamplingUnset
	}
	if v > 2 {
		return 2
	}
	return v
}

// ModelConfigDO 模型级配置。ProviderID + Model 复合主键。
// ContextWindow 为 0 表示跟随内置目录；MaxOutput 已废弃（按窗口派生），列仅保留。
// Temperature / TopP 为 SamplingUnset 时不下发，交给上游默认。

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
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
	Vision        bool    `json:"vision"`
	ToolCall      bool    `json:"tool_call"`
}

// ModelCapabilitiesREQ 批量查模型能力入参；换模型下拉一次列几十上百个模型。
type ModelCapabilitiesREQ struct {
	ProviderID string   `json:"provider_id"`
	Models     []string `json:"models"`
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
