// 模型能力目录：上下文窗口与能力判定。上游模型列表不带这些信息，
// 认不出的模型走保守缺省并标记 unknown，让界面说实话而不是拿猜的数字当权威。
package domain

import "strings"

// DefaultContextWindow 是认不出模型时的缺省窗口：当前主流模型 128K 起步，
// 取 128000 与界面预填的默认值一致；更小的模型靠用户在设置里手填纠正。
const DefaultContextWindow = 128000

// ModelCapability 是单个模型的能力画像。
type ModelCapability struct {
	ID            string `json:"id"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	// MaxOutputLimit 是厂商文档里的输出硬上限（0 = 未知）：窗口 1/8 对 1M 窗口会算出
	// 12.5 万，超过厂商上限就是整轮 400。
	MaxOutputLimit int `json:"max_output_limit"`
	// WindowOverride 表示窗口来自用户手填或服务声明，不是目录查出来的。
	// 此时目录里的硬上限描述的是另一个端点（同名不同服务），不参与钳制。
	WindowOverride bool   `json:"window_override"`
	Thinking       bool   `json:"thinking"`
	Vision         bool   `json:"vision"`
	ToolCall       bool   `json:"tool_call"`
	Known          bool   `json:"known"` // false 表示这是缺省值，不是查到的
	Note           string `json:"note"`  // 认不出来时给界面的一句话说明
}

// OutputBudget 返回该模型应下发的输出预算：窗口 1/8；窗口来自目录时再与厂商硬上限取小。
// 1/8 保证长回答不被随手截断，也是用户手填窗口后的唯一口径（硬上限来自另一个端点）。
func (c ModelCapability) OutputBudget() int {
	if c.ContextWindow <= 0 {
		return 0
	}
	out := c.ContextWindow / 8
	if !c.WindowOverride && c.MaxOutputLimit > 0 && out > c.MaxOutputLimit {
		out = c.MaxOutputLimit
	}
	return out
}

// WithWindow 返回按用户声明的窗口覆盖后的画像：窗口成了确切值，
// 目录口径的硬上限随之失效（它描述的是同名模型的另一个服务）。
func (c ModelCapability) WithWindow(window int) ModelCapability {
	c.ContextWindow = window
	c.WindowOverride = true
	c.Known = true
	c.Note = ""
	return c
}

// capabilityRule 是目录里的一条声明。
type capabilityRule struct {
	match  []string // 模型名包含任一子串即命中，按顺序匹配
	window int
	// maxOut 是厂商文档里的最大输出（0 = 不知道，按窗口 1/8 走）；数值宁保守勿激进——
	// 低一点只是回答短些（界面能「继续」），高一点是整轮被上游拒绝。手填窗口后不生效。
	maxOut int
	think  bool
	vision bool
}

// capabilityRules 是内置目录。顺序即优先级：更具体的写前面。
// 新增模型只改这张表，不需要动任何调用方。
var capabilityRules = []capabilityRule{
	// OpenAI
	{match: []string{"gpt-4.1-mini"}, window: 1000000, maxOut: 32768, vision: true},
	{match: []string{"gpt-4.1"}, window: 1000000, maxOut: 32768, vision: true},
	{match: []string{"gpt-4o-mini", "gpt-4o"}, window: 128000, maxOut: 16384, vision: true},
	{match: []string{"o4-mini", "o3-mini"}, window: 200000, maxOut: 100000, think: true, vision: true},
	{match: []string{"o3", "o1-mini", "o1"}, window: 200000, maxOut: 100000, think: true, vision: true},
	// GPT-5 系走 max_completion_tokens 协议，能力口径与 o 系一致
	{match: []string{"gpt-5"}, window: 400000, maxOut: 128000, think: true, vision: true},
	// Anthropic：3.x 的输出上限只有 4k/8k，按窗口 1/8 下发会被 400 拒掉
	{match: []string{"claude-3-opus", "claude-3-haiku"}, window: 200000, maxOut: 4096, vision: true},
	{match: []string{"claude-3-5-sonnet", "claude-3-5-haiku", "claude-3-7"}, window: 200000, maxOut: 8192, think: true, vision: true},
	{match: []string{"claude-opus-4"}, window: 200000, maxOut: 32000, think: true, vision: true},
	{match: []string{"claude-sonnet-4"}, window: 200000, maxOut: 64000, think: true, vision: true},
	// Google
	{match: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, window: 1000000, maxOut: 65536, think: true, vision: true},
	{match: []string{"gemini"}, window: 1000000, maxOut: 8192, vision: true},
	// DeepSeek
	{match: []string{"deepseek-reasoner", "deepseek-r1"}, window: 128000, maxOut: 65536, think: true, vision: false},
	// v4 / v3.2 起的混合推理模型：同一端点既能思考也能带视觉
	{match: []string{"deepseek-v4", "deepseek-v3.2", "deepseek-v3.1"}, window: 128000, maxOut: 32768, think: true, vision: true},
	{match: []string{"deepseek"}, window: 128000, maxOut: 8192, vision: false},
	// Qwen
	{match: []string{"qwen3", "qwq"}, window: 131072, maxOut: 32768, think: true, vision: false},
	{match: []string{"qwen2.5-vl", "qwen-vl"}, window: 131072, maxOut: 8192, vision: true},
	{match: []string{"qwen"}, window: 131072, maxOut: 8192, vision: false},
	// Kimi / Moonshot
	{match: []string{"kimi-k2-thinking", "kimi-thinking"}, window: 262144, maxOut: 32768, think: true, vision: false},
	{match: []string{"kimi", "moonshot"}, window: 200000, maxOut: 16384, vision: false},
	// GLM
	{match: []string{"glm-5", "glm-4.6"}, window: 200000, maxOut: 32768, think: true, vision: false},
	{match: []string{"glm"}, window: 128000, maxOut: 16384, vision: false},
	// MiniMax
	{match: []string{"minimax-m3", "minimax-m2", "minimax-text"}, window: 1000000, maxOut: 32768, think: true, vision: false},
	{match: []string{"minimax"}, window: 1000000, maxOut: 16384, vision: false},
	// Llama / Mistral / Grok
	{match: []string{"llama-3.3", "llama-3.1"}, window: 131072, maxOut: 8192, vision: false},
	{match: []string{"llama"}, window: 131072, maxOut: 8192, vision: false},
	{match: []string{"mistral-large", "mistral-medium"}, window: 128000, maxOut: 8192, vision: false},
	{match: []string{"grok"}, window: 131072, maxOut: 32768, think: true, vision: true},
}

// ModelCapabilityOf 返回模型的能力画像；认不出来返回带 unknown 标记的缺省值。
func ModelCapabilityOf(model string) ModelCapability {
	name := strings.TrimSpace(model)
	lower := strings.ToLower(name)
	for _, r := range capabilityRules {
		for _, key := range r.match {
			if strings.Contains(lower, key) {
				cap := ModelCapability{
					ID: name, ContextWindow: r.window,
					MaxOutputLimit: r.maxOut,
					Thinking:       r.think, Vision: r.vision, ToolCall: true, Known: true,
				}
				cap.MaxOutput = cap.OutputBudget()
				return cap
			}
		}
	}
	cap := ModelCapability{
		ID: name, ContextWindow: DefaultContextWindow,
		ToolCall: true, Known: false, Note: "本地没有这个模型的资料，可在模型设置里手填窗口与能力",
	}
	cap.MaxOutput = cap.OutputBudget()
	return cap
}
