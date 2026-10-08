// 模型能力目录：上下文窗口与能力判定。上游模型列表不带这些信息，
// 认不出的模型走保守缺省并标记 unknown，让界面说实话而不是拿猜的数字当权威。
package domain

import "strings"

// 模型能力缺省值：认不出来时用这一组，并在 ModelCapability.Known 上标 false。
const DefaultContextWindow = 128000

// MaxOutputOf 输出预算 = 上下文窗口的 1/8：窗口是唯一入口，输出永远跟着窗口走。
func MaxOutputOf(contextWindow int) int {
	if contextWindow <= 0 {
		return 0
	}
	return contextWindow / 8
}

// ModelCapability 是单个模型的能力画像。
type ModelCapability struct {
	ID            string `json:"id"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	Thinking      bool   `json:"thinking"`
	Vision        bool   `json:"vision"`
	ToolCall      bool   `json:"tool_call"`
	Known         bool   `json:"known"` // false 表示这是缺省值，不是查到的
	Note          string `json:"note"`  // 认不出来时给界面的一句话说明
}

// capabilityRule 是目录里的一条声明。
type capabilityRule struct {
	match  []string // 模型名包含任一子串即命中，按顺序匹配
	window int
	think  bool
	vision bool
}

// capabilityRules 是内置目录。顺序即优先级：更具体的写前面。
// 新增模型只改这张表，不需要动任何调用方。
var capabilityRules = []capabilityRule{
	// OpenAI
	{match: []string{"gpt-4.1-mini", "gpt-4o-mini", "gpt-4o"}, window: 128000, think: false, vision: true},
	{match: []string{"gpt-4.1"}, window: 1000000, think: false, vision: true},
	{match: []string{"o4-mini", "o3-mini"}, window: 200000, think: true, vision: true},
	{match: []string{"o3", "o1-mini", "o1"}, window: 200000, think: true, vision: true},
	// Anthropic
	{match: []string{"claude-3-5-haiku"}, window: 200000, think: true, vision: true},
	{match: []string{"claude-3-5-sonnet", "claude-sonnet-4", "claude-opus-4"}, window: 200000, think: true, vision: true},
	{match: []string{"claude-3-opus", "claude-3-haiku"}, window: 200000, think: false, vision: true},
	// Google
	{match: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, window: 1000000, think: true, vision: true},
	{match: []string{"gemini"}, window: 1000000, think: false, vision: true},
	// DeepSeek
	{match: []string{"deepseek-reasoner", "deepseek-r1"}, window: 128000, think: true, vision: false},
	// v4 / v3.2 起的混合推理模型：同一端点既能思考也能带视觉
	{match: []string{"deepseek-v4", "deepseek-v3.2", "deepseek-v3.1"}, window: 128000, think: true, vision: true},
	{match: []string{"deepseek"}, window: 128000, think: false, vision: false},
	// Qwen
	{match: []string{"qwen3", "qwq"}, window: 131072, think: true, vision: false},
	{match: []string{"qwen2.5-vl", "qwen-vl"}, window: 131072, think: false, vision: true},
	{match: []string{"qwen"}, window: 131072, think: false, vision: false},
	// Kimi / Moonshot
	{match: []string{"kimi-k2-thinking", "kimi-thinking"}, window: 262144, think: true, vision: false},
	{match: []string{"kimi", "moonshot"}, window: 200000, think: false, vision: false},
	// GLM
	{match: []string{"glm-5", "glm-4.6"}, window: 200000, think: true, vision: false},
	{match: []string{"glm"}, window: 128000, think: false, vision: false},
	// MiniMax
	{match: []string{"minimax-m3", "minimax-m2", "minimax-text"}, window: 1000000, think: true, vision: false},
	{match: []string{"minimax"}, window: 1000000, think: false, vision: false},
	// Llama / Mistral / Grok
	{match: []string{"llama-3.3", "llama-3.1"}, window: 131072, think: false, vision: false},
	{match: []string{"llama"}, window: 131072, think: false, vision: false},
	{match: []string{"mistral-large", "mistral-medium"}, window: 128000, think: false, vision: false},
	{match: []string{"grok"}, window: 131072, think: true, vision: true},
}

// ModelCapabilityOf 返回模型的能力画像；认不出来返回带 unknown 标记的缺省值。
func ModelCapabilityOf(model string) ModelCapability {
	name := strings.TrimSpace(model)
	lower := strings.ToLower(name)
	for _, r := range capabilityRules {
		for _, key := range r.match {
			if strings.Contains(lower, key) {
				return ModelCapability{
					ID: name, ContextWindow: r.window, MaxOutput: MaxOutputOf(r.window),
					Thinking: r.think, Vision: r.vision, ToolCall: true, Known: true,
				}
			}
		}
	}
	return ModelCapability{
		ID: name, ContextWindow: DefaultContextWindow, MaxOutput: MaxOutputOf(DefaultContextWindow),
		ToolCall: true, Known: false, Note: "本地没有这个模型的资料，可在模型设置里手填窗口与能力",
	}
}
