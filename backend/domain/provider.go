// 模型服务聚合根：读写 PI 的 models.json，界面与 pi 命令共用同一份配置。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrProviderNotFound = pkg.New("3101", "模型服务不存在", "")
	ErrProviderName     = pkg.New("3109", "给这个服务起个名字（会作为它的标识）", "")
	ErrProviderAPI      = pkg.New("3108", "不支持的接口类型", "")
	ErrProviderModel    = pkg.New("3110", "这个服务下还没有模型，至少加一个", "")
	ErrProviderModelID  = pkg.New("3111", "有一个模型的 ID 是空的，补上或删掉它", "")
	ErrProviderBaseURL  = pkg.New("3113", "先填接口地址（形如 https://api.example.com/v1）", "")
)

// API 取值：与 PI 的 models.json 一致。
const (
	APIOpenAICompletions   = "openai-completions"
	APIOpenAIResponses     = "openai-responses"
	APIAnthropicMessages   = "anthropic-messages"
	APIGoogleGenerative    = "google-generative-ai"
	APIAzureOpenAIResponse = "azure-openai-responses"
)

// APIAll 是界面上可选的接口类型。
var APIAll = []string{
	APIOpenAICompletions, APIOpenAIResponses,
	APIAnthropicMessages, APIGoogleGenerative, APIAzureOpenAIResponse,
}

// 模型能力的兜底：没填上下文窗口 / 输出上限时按这两个值走。
// 留空不能等于 0：水位环会一直显示 0%，PI 也拿不到可用的预算。
const (
	DefaultContextWindow = 300000
	DefaultMaxTokens     = 64000
)

// ProviderVO 一个服务商；密钥不回传明文，只给掩码。
type ProviderVO struct {
	// ID 是 models.json 里的键，也是 PI 的 provider 名。
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	API     string    `json:"api"`
	BaseURL string    `json:"base_url"`
	HasKey  bool      `json:"has_key"`
	KeyHint string    `json:"key_hint"`
	Models  []ModelVO `json:"models"`
	// IsDefault 由 PI 的 settings.json 里 defaultProvider 决定。
	IsDefault bool `json:"is_default"`
}

// ModelVO 一个模型条目；未填的字段由 PI 的内置目录兜底。
type ModelVO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Vision        bool     `json:"vision"`
	ContextWindow int      `json:"context_window"`
	MaxTokens     int      `json:"max_tokens"`
	ThinkingLevel []string `json:"thinking_levels,omitempty"`
}

// ProviderListRESP 出参：服务商清单 + 默认模型 + 配置文件位置。
type ProviderListRESP struct {
	Providers    []ProviderVO `json:"providers"`
	DefaultID    string       `json:"default_provider"`
	DefaultModel string       `json:"default_model"`
	Path         string       `json:"path"`
}

// UpsertProviderREQ 新增或更新一个服务商。
// 模型列表整体替换：界面上那一行行就是它的全部模型。
type UpsertProviderREQ struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	API     string     `json:"api"`
	BaseURL string     `json:"base_url"`
	APIKey  string     `json:"api_key"`
	Models  []ModelREQ `json:"models"`
}

// ModelREQ 一个模型条目的可编辑字段。
type ModelREQ struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Reasoning     bool     `json:"reasoning"`
	Vision        bool     `json:"vision"`
	ContextWindow int      `json:"context_window"`
	MaxTokens     int      `json:"max_tokens"`
	ThinkingLevel []string `json:"thinking_levels,omitempty"`
}

// DeleteProviderREQ 删除入参（id 里有斜杠与点，走请求体）。
type DeleteProviderREQ struct {
	ID string `json:"id"`
}

// ProviderIDREQ 只带服务商名的入参。
type ProviderIDREQ struct {
	ID string `json:"id"`
}

// TestProviderRESP 连通测试出参。
type TestProviderRESP struct {
	OK      bool   `json:"ok"`
	Model   string `json:"model"`
	Message string `json:"message"`
}

// FetchModelsREQ 用未保存的连接信息向上游要模型列表。
type FetchModelsREQ struct {
	API     string `json:"api"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	// ID 非空时复用已存密钥（编辑已存在的服务商时不必重填）。
	ID string `json:"id"`
}

// FetchModelsRESP 上游模型清单。
type FetchModelsRESP struct {
	Models []string `json:"models"`
}

// SetDefaultREQ 设默认模型（写 PI settings.json 的 defaultProvider / defaultModel）。
type SetDefaultREQ struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RevealProviderKeyVO 显式查看密钥时的出参。
type RevealProviderKeyVO struct {
	APIKey string `json:"api_key"`
}

// AvailableModelVO 是 PI 进程报上来的可用模型（含内置服务商与登录态）。
type AvailableModelVO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
	Reasoning     bool   `json:"reasoning"`
	Vision        bool   `json:"vision"`
}
