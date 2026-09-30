// 模型能力目录：上下文窗口与「是否支持思考」的判定。
//
// 上游 `/models` 只返回模型 ID，不带窗口大小与思考能力；
// 拉一次列表就"知道"上下文和思考能力是不成立的。这里的表是显式声明：
// 认不出来的模型走保守缺省，并把 unknown 标出来让界面说实话，
// 而不是拿一个猜出来的数字当权威显示给用户。
package domain

import "strings"

// 模型能力缺省值：认不出来时用这一组，并在 ModelCapability.Known 上标 false。
const (
	DefaultContextWindow = 128000
	DefaultMaxOutput     = 8192
)

// ModelCapability 是单个模型的能力画像。
type ModelCapability struct {
	ID            string `json:"id"`
	ContextWindow int    `json:"context_window"`
	MaxOutput     int    `json:"max_output"`
	Thinking      bool   `json:"thinking"`
	Vision        bool   `json:"vision"`
	Known         bool   `json:"known"` // false 表示这是缺省值，不是查到的
	Note          string `json:"note"`  // 认不出来时给界面的一句话说明
}

// capabilityRule 是目录里的一条声明。
type capabilityRule struct {
	match  []string // 模型名包含任一子串即命中，按顺序匹配
	window int
	output int
	think  bool
	vision bool
}

// capabilityRules 是内置目录。顺序即优先级：更具体的写前面。
// 新增模型只改这张表，不需要动任何调用方。
var capabilityRules = []capabilityRule{
	// OpenAI
	{match: []string{"gpt-4.1-mini", "gpt-4o-mini", "gpt-4o"}, window: 128000, output: 16384, think: false, vision: true},
	{match: []string{"gpt-4.1"}, window: 1000000, output: 32768, think: false, vision: true},
	{match: []string{"o4-mini", "o3-mini"}, window: 200000, output: 100000, think: true, vision: true},
	{match: []string{"o3", "o1-mini", "o1"}, window: 200000, output: 100000, think: true, vision: true},
	// Anthropic
	{match: []string{"claude-3-5-haiku"}, window: 200000, output: 8192, think: true, vision: true},
	{match: []string{"claude-3-5-sonnet", "claude-sonnet-4", "claude-opus-4"}, window: 200000, output: 64000, think: true, vision: true},
	{match: []string{"claude-3-opus", "claude-3-haiku"}, window: 200000, output: 4096, think: false, vision: true},
	// Google
	{match: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, window: 1000000, output: 65536, think: true, vision: true},
	{match: []string{"gemini"}, window: 1000000, output: 8192, think: false, vision: true},
	// DeepSeek
	{match: []string{"deepseek-reasoner", "deepseek-r1"}, window: 128000, output: 32768, think: true, vision: false},
	{match: []string{"deepseek"}, window: 128000, output: 8192, think: false, vision: false},
	// Qwen
	{match: []string{"qwen3", "qwq"}, window: 131072, output: 32768, think: true, vision: false},
	{match: []string{"qwen2.5-vl", "qwen-vl"}, window: 131072, output: 8192, think: false, vision: true},
	{match: []string{"qwen"}, window: 131072, output: 8192, think: false, vision: false},
	// Kimi / Moonshot
	{match: []string{"kimi-k2-thinking", "kimi-thinking"}, window: 262144, output: 32768, think: true, vision: false},
	{match: []string{"kimi", "moonshot"}, window: 200000, output: 16384, think: false, vision: false},
	// GLM
	{match: []string{"glm-4.6"}, window: 200000, output: 32768, think: true, vision: false},
	{match: []string{"glm"}, window: 128000, output: 8192, think: false, vision: false},
	// MiniMax
	{match: []string{"minimax-m3", "minimax-m2", "minimax-text"}, window: 1000000, output: 32768, think: true, vision: false},
	{match: []string{"minimax"}, window: 1000000, output: 16384, think: false, vision: false},
	// Llama / Mistral / Groq
	{match: []string{"llama-3.3", "llama-3.1"}, window: 131072, output: 8192, think: false, vision: false},
	{match: []string{"llama"}, window: 131072, output: 4096, think: false, vision: false},
	{match: []string{"mistral-large", "mistral-medium"}, window: 128000, output: 8192, think: false, vision: false},
	{match: []string{"grok"}, window: 131072, output: 32768, think: true, vision: true},
}

// ModelCapabilityOf 返回模型的能力画像。
// 表里认得出来就返回声明值；认不出来返回带 unknown 标记的缺省值，
// 调用方据此决定「显示确切数字」还是「显示未知」。
func ModelCapabilityOf(model string) ModelCapability {
	name := strings.TrimSpace(model)
	lower := strings.ToLower(name)
	for _, r := range capabilityRules {
		for _, key := range r.match {
			if strings.Contains(lower, key) {
				return ModelCapability{
					ID: name, ContextWindow: r.window, MaxOutput: r.output,
					Thinking: r.think, Vision: r.vision, Known: true,
				}
			}
		}
	}
	return ModelCapability{
		ID: name, ContextWindow: DefaultContextWindow, MaxOutput: DefaultMaxOutput,
		Known: false, Note: "本地没有这个模型的资料，上下文按 128K 估算，可在设置里手填",
	}
}
