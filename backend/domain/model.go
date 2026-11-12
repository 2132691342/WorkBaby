package domain

// 服务商与模型：pi 的 models.json / auth.json 在界面上的形态。
// 它们与对话类型分文件：一条是「引擎怎么跑一轮」，一条是「引擎能选哪些模型」，
// 两者改动的原因不同，放一处只会让真正相关的那几行被埋掉。

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
	// Models 可选模型平铺
	Models []ModelOptionVO `json:"models"`
	// ModelSource 清单来源：engine（引擎实况）/ config（models.json 回退）
	ModelSource string `json:"model_source"`
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

// ModelDefaultVO 是 pi 的默认模型与默认思考档（settings.json 的三个键）。
// 它管的是「新会话从哪个模型起步」，不是当前会话正在用的模型。
type ModelDefaultVO struct {
	// DefaultProvider 默认服务商
	DefaultProvider string `json:"default_provider,omitempty"`
	// DefaultModel 默认模型 ID
	DefaultModel string `json:"default_model,omitempty"`
	// DefaultThinkingLevel 默认思考档
	DefaultThinkingLevel string `json:"default_thinking_level,omitempty"`
	// ThinkingLevels 该模型可用的思考档（空 = 以引擎报的为准）
	ThinkingLevels []string `json:"thinking_levels,omitempty"`
	// SettingsFile settings.json 路径（界面上标一下写到哪里）
	SettingsFile string `json:"settings_file"`
}

// ModelDefaultSaveREQ 是保存默认模型的入参；空串表示清掉该键。
type ModelDefaultSaveREQ struct {
	// DefaultProvider 默认服务商
	DefaultProvider string `json:"default_provider"`
	// DefaultModel 默认模型 ID
	DefaultModel string `json:"default_model"`
	// DefaultThinkingLevel 默认思考档
	DefaultThinkingLevel string `json:"default_thinking_level"`
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
