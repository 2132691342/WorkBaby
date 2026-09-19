package llm

// ProviderPreset 常见模型服务的预置配置：新增一个兼容 provider 只需选中预设（零代码）——
// kind 决定协议实现，base_url / models 预填表单；预设之外的自定义端点同样只需一行配置。
type ProviderPreset struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	BaseURL string   `json:"base_url"`
	Models  []string `json:"models"`
	Note    string   `json:"note"`
}

// providerPresets 内置预设清单（base_url 为各服务公开的 API 地址；models 为常用首选项）。
var providerPresets = []ProviderPreset{
	{Name: "OpenAI", Kind: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-5", "gpt-4.1", "gpt-4o-mini"}},
	{Name: "Anthropic", Kind: "anthropic", BaseURL: "https://api.anthropic.com", Models: []string{"claude-sonnet-4-5", "claude-opus-4-1", "claude-3-5-haiku-latest"}},
	{Name: "DeepSeek", Kind: "openai", BaseURL: "https://api.deepseek.com/v1", Models: []string{"deepseek-chat", "deepseek-reasoner"}},
	{Name: "Moonshot Kimi", Kind: "openai", BaseURL: "https://api.moonshot.cn/v1", Models: []string{"kimi-k2-0905-preview", "kimi-k2-turbo-preview"}},
	{Name: "智谱 GLM", Kind: "openai", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Models: []string{"glm-4.6", "glm-4.5-air"}},
	{Name: "阿里百炼 Qwen", Kind: "openai", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Models: []string{"qwen3-max", "qwen-plus"}},
	{Name: "OpenRouter", Kind: "openai", BaseURL: "https://openrouter.ai/api/v1", Models: []string{"openai/gpt-4.1", "anthropic/claude-sonnet-4.5", "deepseek/deepseek-chat"}},
	{Name: "SiliconFlow", Kind: "openai", BaseURL: "https://api.siliconflow.cn/v1", Models: []string{"deepseek-ai/DeepSeek-V3", "Qwen/Qwen3-235B-A22B"}},
	{Name: "Ollama（本机）", Kind: "ollama", BaseURL: "http://127.0.0.1:11434", Note: "本地推理，无需 API Key"},
	{Name: "LM Studio（本机）", Kind: "openai", BaseURL: "http://127.0.0.1:1234/v1", Note: "本地推理，无需 API Key"},
}

// ProviderPresets 返回内置预设（api 层直出，前端「新增模型」一键预填）。
func ProviderPresets() []ProviderPreset {
	out := make([]ProviderPreset, len(providerPresets))
	copy(out, providerPresets)
	return out
}
