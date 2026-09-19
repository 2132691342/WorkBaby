package llm

import "WorkBaby/internal/domain"

// ProviderParams Provider 级采样参数（ai_providers.temperature/thinking 等）。
// 与 ChatRequest 同名字段，调用 ResolveParams 时取"最高优先级非零值"。
type ProviderParams struct {
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"topP,omitempty"`
	MaxTokens   *int            `json:"maxTokens,omitempty"`
	Thinking    *ThinkingConfig `json:"thinking,omitempty"`
	ExtraBody   map[string]any  `json:"extraBody,omitempty"`
}

// Defaults 全局默认（system_settings: chat.defaultTemperature=0.25；topP 固定 0.75）。
type Defaults struct {
	Temperature float64
	TopP        float64
	MaxTokens   int
	Thinking    *ThinkingConfig
}

// ResolvedParams 合并后的最终参数，下游 (openai/anthropic/ollama) 直接读这个。
type ResolvedParams struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
	Thinking    *ThinkingConfig
	ExtraBody   map[string]any
}

// ResolveParams 三级合并 req > provider > defaults：逐字段取最高优先级处的非 nil 值；
// ExtraBody 浅合并（req 覆盖同名 key）；Thinking 显式 disabled 保留，不被 defaults 覆盖。
func ResolveParams(req *ChatRequest, provider *ProviderParams, def Defaults) ResolvedParams {
	out := ResolvedParams{}
	// Temperature
	switch {
	case req.Temperature != nil:
		out.Temperature = req.Temperature
	case provider != nil && provider.Temperature != nil:
		out.Temperature = provider.Temperature
	default:
		t := def.Temperature
		if t > 0 {
			out.Temperature = &t
		}
	}
	// TopP
	switch {
	case req.TopP != nil:
		out.TopP = req.TopP
	case provider != nil && provider.TopP != nil:
		out.TopP = provider.TopP
	default:
		if def.TopP > 0 {
			tp := def.TopP
			out.TopP = &tp
		}
	}
	// MaxTokens
	switch {
	case req.MaxTokens != nil && *req.MaxTokens > 0:
		out.MaxTokens = req.MaxTokens
	case provider != nil && provider.MaxTokens != nil && *provider.MaxTokens > 0:
		out.MaxTokens = provider.MaxTokens
	default:
		if def.MaxTokens > 0 {
			mt := def.MaxTokens
			out.MaxTokens = &mt
		}
	}
	// Thinking
	switch {
	case req.Thinking != nil:
		out.Thinking = req.Thinking
	case provider != nil && provider.Thinking != nil:
		out.Thinking = provider.Thinking
	default:
		out.Thinking = def.Thinking
	}
	// ExtraBody 浅合并：req > provider
	out.ExtraBody = map[string]any{}
	for k, v := range providerDefaults(provider) {
		out.ExtraBody[k] = v
	}
	for k, v := range req.ExtraBody {
		out.ExtraBody[k] = v
	}
	if len(out.ExtraBody) == 0 {
		out.ExtraBody = nil
	}
	return out
}

func providerDefaults(p *ProviderParams) map[string]any {
	if p == nil || len(p.ExtraBody) == 0 {
		return nil
	}
	return p.ExtraBody
}

// ProviderParamsFromDO 把持久化 AiProviderDO 映射成 ProviderParams。
// 约定：Temperature==0 / TopP==0 / ThinkingEffort=="" 视作"未设置"，对应字段置 nil
// （被 ResolveParams 跳到下一层：全局默认）。
// TopP 不映射会让 Provider 表格里填的 top_p 静默失效——展示了输入框却不生效，
// 比没有这个字段更糟。
func ProviderParamsFromDO(p *domain.AiProviderDO) *ProviderParams {
	if p == nil {
		return nil
	}
	out := &ProviderParams{}
	if p.Temperature != 0 {
		t := p.Temperature
		out.Temperature = &t
	}
	if p.TopP != 0 {
		tp := p.TopP
		out.TopP = &tp
	}
	if tc := ThinkingFromEffort(p.ThinkingEffort); tc != nil {
		out.Thinking = tc
	}
	return out
}
