// 本文件是模型与服务商的类型集合：服务商定义、模型清单与探测入参。
// 它们同属「模型管理」这一个域，所以放在一起；对话相关的类型在 chat.go。

package domain

// ProviderModelEntryVO 是一条服务商模型配置。
type ProviderModelEntryVO struct {
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
	// ModelName 显示名
	ModelName string `json:"model_name,omitempty"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api_protocol,omitempty"`
	// SupportsReasoning 是否支持思考
	SupportsReasoning bool `json:"supports_reasoning,omitempty"`
	// ContextWindow 上下文窗口
	ContextWindow int `json:"context_window,omitempty"`
	// MaxTokens 单次输出上限
	MaxTokens int `json:"max_tokens,omitempty"`
	// InputKinds 支持的输入种类
	InputKinds []string `json:"input_kinds,omitempty"`
}

// ModelOptionVO 是对话页模型下拉里的一条可选模型（服务商下的模型平铺）。
type ModelOptionVO struct {
	// ProviderName 所属服务商
	ProviderName string `json:"provider_name"`
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
	// ModelName 显示名
	ModelName string `json:"model_name,omitempty"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api_protocol,omitempty"`
	// ContextWindow 上下文窗口
	ContextWindow int `json:"context_window,omitempty"`
	// MaxTokens 单次输出上限
	MaxTokens int `json:"max_tokens,omitempty"`
	// SupportsReasoning 是否支持思考
	SupportsReasoning bool `json:"supports_reasoning,omitempty"`
	// InputKinds 支持的输入种类
	InputKinds []string `json:"input_kinds,omitempty"`
}

// ModelListRESP 是可选模型平铺清单。
type ModelListRESP struct {
	// Models 全部自定义服务商的模型
	Models []ModelOptionVO `json:"models"`
}

// ProviderVO 是一个服务商在界面上的样子；ApiKey 恒为掩码。
type ProviderVO struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// BaseUrl 接口地址
	BaseUrl string `json:"base_url"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api_protocol"`
	// AuthKind 鉴权方式
	AuthKind string `json:"auth_kind"`
	// ApiKeyMasked 掩码后的密钥
	ApiKeyMasked string `json:"api_key_masked"`
	// HasKey 是否已配置密钥
	HasKey bool `json:"has_key"`
	// Models 这个服务商下的模型
	Models []ProviderModelEntryVO `json:"models"`
	// ModelCount 模型数
	ModelCount int `json:"model_count"`
}

// ProvidersRESP 是服务商清单。
type ProvidersRESP struct {
	// Providers 服务商行
	Providers []ProviderVO `json:"providers"`
	// ConfigPath models.json 路径
	ConfigPath string `json:"config_path"`
	// ReadError 读取失败原因
	ReadError string `json:"read_error,omitempty"`
}

// ProviderUpsertREQ 是保存服务商的入参；ApiKey 空串语义是「保留原值」。
type ProviderUpsertREQ struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// BaseUrl 接口地址
	BaseUrl string `json:"base_url"`
	// ApiProtocol 协议（openai / anthropic / google）
	ApiProtocol string `json:"api_protocol"`
	// AuthKind 鉴权方式
	AuthKind string `json:"auth_kind"`
	// ApiKey 密钥原文；空串 = 保留原值
	ApiKey string `json:"api_key"`
	// Models 模型清单
	Models []ProviderModelEntryVO `json:"models,omitempty"`
}

// ProviderDeleteREQ 是删除服务商的入参。
type ProviderDeleteREQ struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
}

// ProviderModelSaveREQ 是单条模型增改的入参；服务商必须已存在。
type ProviderModelSaveREQ struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// Model 模型条目（按 ModelId 定位，存在即改，不存在即增）
	Model ProviderModelEntryVO `json:"model"`
}

// ProviderModelDeleteREQ 是删除服务商下一条模型的入参。
type ProviderModelDeleteREQ struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
}

// ProviderProbeREQ 是向上游探测的入参；空字段由后端从已保存配置里取。
type ProviderProbeREQ struct {
	// ProviderName 服务商名
	ProviderName string `json:"provider_name"`
	// BaseUrl 接口地址
	BaseUrl string `json:"base_url"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api_protocol"`
	// ApiKey 密钥原文；空串 = 用已保存的
	ApiKey string `json:"api_key"`
}

// ProviderProbeRESP 是探测结果。
type ProviderProbeRESP struct {
	// ProbeOk 是否连通
	ProbeOk bool `json:"probe_ok"`
	// ModelCount 拉到的模型数
	ModelCount int `json:"model_count"`
	// Models 拉到的模型
	Models []ProviderModelEntryVO `json:"models,omitempty"`
	// ProbeMessage 结果原话
	ProbeMessage string `json:"probe_message"`
}
