// Package llm 内部使用的思维强度归一化；service/api 层共享，避免重复定义。
package llm

import "strings"

// ThinkingStyle 思维（推理）参数的协议方言。
//
// OpenAI 兼容生态里「怎么开思考」没有统一标准：字段名、枚举值、是否要预算各不相同。
// 发错方言会直接被上游 400 拒掉（如 MiniMax 只接受 adaptive/disabled，
// 收到 enabled 报 invalid thinking.type）。故按上游方言渲染，而非统一发一种。
type ThinkingStyle string

const (
	// ThinkingStyleAuto 未指定：按 baseURL + 模型名探测（探测不到退化为 none，不发字段）。
	ThinkingStyleAuto ThinkingStyle = ""
	// ThinkingStyleNone 不支持/不上报思维参数（DeepSeek 等自动回吐 reasoning_content）。
	ThinkingStyleNone ThinkingStyle = "none"
	// ThinkingStyleAdaptive {"thinking":{"type":"adaptive"|"disabled"}}；MiniMax M 系列。
	ThinkingStyleAdaptive ThinkingStyle = "adaptive"
	// ThinkingStyleEnabled {"thinking":{"type":"enabled"|"disabled"}}；智谱 GLM / 火山方舟 / Kimi。
	ThinkingStyleEnabled ThinkingStyle = "enabled"
	// ThinkingStyleReasoningEffort {"reasoning_effort":"low"|"medium"|"high"}；OpenAI o 系列 / GPT-5。
	ThinkingStyleReasoningEffort ThinkingStyle = "reasoning_effort"
	// ThinkingStyleEnableBool {"enable_thinking":bool,"thinking_budget":N}；通义千问 Qwen3。
	ThinkingStyleEnableBool ThinkingStyle = "enable_thinking"
)

// AllThinkingStyles 供设置页下拉与入参校验；顺序即展示顺序。
var AllThinkingStyles = []ThinkingStyle{
	ThinkingStyleAuto,
	ThinkingStyleNone,
	ThinkingStyleEnabled,
	ThinkingStyleAdaptive,
	ThinkingStyleReasoningEffort,
	ThinkingStyleEnableBool,
}

// ParseThinkingStyle 解析方言字符串；未知值回落 auto（由探测决定，不静默丢弃用户输入）。
func ParseThinkingStyle(s string) ThinkingStyle {
	switch ThinkingStyle(strings.TrimSpace(s)) {
	case ThinkingStyleNone, ThinkingStyleAdaptive, ThinkingStyleEnabled,
		ThinkingStyleReasoningEffort, ThinkingStyleEnableBool:
		return ThinkingStyle(s)
	default:
		return ThinkingStyleAuto
	}
}

// ThinkingFromEffort 把 effort 字符串转成 ThinkingConfig：off/空 → nil（由 Provider 与
// 全局默认兜底）；low/medium/high → enabled + 对应 budget_tokens；未知值不传。
func ThinkingFromEffort(effort string) *ThinkingConfig {
	switch effort {
	case "off":
		return &ThinkingConfig{Type: "disabled"}
	case "low":
		return &ThinkingConfig{Type: "enabled", BudgetTokens: 2048}
	case "high":
		return &ThinkingConfig{Type: "enabled", BudgetTokens: 16384}
	case "medium":
		return &ThinkingConfig{Type: "enabled", BudgetTokens: 8192}
	default:
		return nil
	}
}

// thinkingProbe 一条方言探测规则：命中 host 关键词或模型名关键词即判定方言。
type thinkingProbe struct {
	style ThinkingStyle
	hosts []string // baseURL 小写子串
	match func(model string) bool
}

// thinkingProbes 按序匹配，先命中先返回。只登记「不发思维参数会削弱能力、或发错会 400」
// 的上游；探测不到的一律 none——宁可不开思考，也不能让整条链路 400 不可用。
var thinkingProbes = []thinkingProbe{
	{style: ThinkingStyleAdaptive, hosts: []string{"minimaxi.com", "minimax.chat", "api.minimax"}},
	{style: ThinkingStyleEnabled, hosts: []string{"bigmodel.cn", "open.bigmodel", "volces.com", "volcengineapi.com", "ark.cn-beijing", "moonshot.cn", "moonshot.ai"}},
	{style: ThinkingStyleEnableBool, hosts: []string{"dashscope", "bailian", "aliyuncs.com"}},
	{style: ThinkingStyleReasoningEffort, hosts: []string{"api.openai.com"}, match: isOpenAIReasoningModel},
	{style: ThinkingStyleNone, hosts: []string{"deepseek.com", "api.deepseek"}},
	{style: ThinkingStyleEnableBool, match: isQwenThinkingModel},
	{style: ThinkingStyleReasoningEffort, match: isOpenAIReasoningModel},
}

// DetectThinkingStyle 按 baseURL 与模型名探测方言；无法判定返回 none（安全默认，
// 避免未知上游因不认识的参数直接 400）。用户可在 Provider 设置里显式指定方言。
func DetectThinkingStyle(baseURL, model string) ThinkingStyle {
	host := strings.ToLower(baseURL)
	name := strings.ToLower(model)
	for _, p := range thinkingProbes {
		hostHit := false
		for _, h := range p.hosts {
			if strings.Contains(host, h) {
				hostHit = true
				break
			}
		}
		if hostHit {
			// 带模型断言的规则要求两者同时命中：同一家上游不同模型的
			// 思维方言可能不同（如 OpenAI 的 gpt-4o 与 o3 系列走不同参数）。
			if p.match == nil || p.match(name) {
				return p.style
			}
			continue
		}
		if p.match != nil && p.match(name) {
			return p.style
		}
	}
	return ThinkingStyleNone
}

func isOpenAIReasoningModel(model string) bool {
	return strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") ||
		strings.HasPrefix(model, "gpt-5")
}

func isQwenThinkingModel(model string) bool {
	return strings.Contains(model, "qwen3") || strings.Contains(model, "-thinking")
}

// ResolveThinkingStyle 合并「用户显式指定」与「自动探测」，显式优先。
// 探测只能覆盖已知上游，私有部署与新厂商需人工纠偏口。
func ResolveThinkingStyle(specified, baseURL, model string) ThinkingStyle {
	if s := ParseThinkingStyle(specified); s != ThinkingStyleAuto {
		return s
	}
	return DetectThinkingStyle(baseURL, model)
}

// RenderThinking 把归一化思维配置渲染成上游请求体的顶层字段。返回 nil 表示不注入
// 任何字段（dialect=none 或未开启）；disabled 仍需显式下发（部分上游默认开启思考）。
func RenderThinking(style ThinkingStyle, tc *ThinkingConfig) map[string]any {
	if tc == nil {
		return nil
	}
	on := tc.Type != "disabled"
	switch style {
	case ThinkingStyleAdaptive:
		if on {
			return map[string]any{"thinking": map[string]any{"type": "adaptive"}}
		}
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	case ThinkingStyleEnabled:
		if on {
			return map[string]any{"thinking": map[string]any{"type": "enabled"}}
		}
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	case ThinkingStyleReasoningEffort:
		if !on {
			return nil // o 系列无法关闭推理，关闭时改为不下发
		}
		return map[string]any{"reasoning_effort": effortFromBudget(tc.BudgetTokens)}
	case ThinkingStyleEnableBool:
		if !on {
			return map[string]any{"enable_thinking": false}
		}
		return map[string]any{"enable_thinking": true, "thinking_budget": tc.BudgetTokens}
	default:
		return nil
	}
}

// effortFromBudget 预算 token → OpenAI reasoning_effort 档位。
func effortFromBudget(budget int) string {
	switch {
	case budget > 0 && budget <= 2048:
		return "low"
	case budget >= 8192:
		return "high"
	default:
		return "medium"
	}
}
