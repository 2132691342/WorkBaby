// Package llm 提供归一化消息模型与三个上游协议的流式适配。
package llm

import (
	"context"
	"strings"

	"WorkBaby/backend/pkg"
)

// 消息角色。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// 停止原因。
const (
	StopStop    = "stop"
	StopLength  = "length"
	StopToolUse = "tool_use"
	StopError   = "error"
	StopAborted = "aborted"
)

// 事件类型。
const (
	EventDelta    = "delta"
	EventThinking = "thinking"
	EventToolCall = "tool_call"
	EventDone     = "done"
	EventError    = "error"
)

// DefaultMaxTokens 是输出预算的最后兜底，走到这里说明上层漏了。
// 0 不能下发：openai 按 omitempty 丢掉整个字段，anthropic 的 max_tokens 是必填。
const DefaultMaxTokens = 16384

var (
	ErrNoAPIKey  = pkg.New(3001, "缺少 API Key", "")
	ErrBadStream = pkg.New(3105, "上游返回了无法解析的流", "")
)

// Image 是 user 消息附带的图片；Base64 不含 data: 前缀。
// 历史回传时原样保留，识图模型才能在多轮对话里持续看到图。
type Image struct {
	MIME   string
	Base64 string
}

// Message 是归一化消息：assistant 的正文与思考严格分离，工具调用与结果各自独立。
type Message struct {
	Role       string
	Content    string
	Thinking   string
	ToolCalls  []ToolCall
	ToolCallID string
	IsError    bool
	// Images 只在 user 消息上出现；不支持识图的模型由 service 层提前拦截。
	Images []Image
}

// ToolCall 是一次工具调用；Args 保留原始 JSON，由工具侧自行解析。
type ToolCall struct {
	ID    string
	Name  string
	Label string
	Args  map[string]any
}

// ToolDef 是给上游的工具声明。
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Usage 是一次调用的输入 / 输出与耗时。
type Usage struct {
	Input  int
	Output int
	Total  int
	// Cached 是这次输入里命中上游缓存的部分。缓存命中才是长对话真正的成本项：
	// 同一段前缀重复计费的话，多轮对话的花费会随轮数线性膨胀。
	Cached    int
	LatencyMs int64
}

// CacheHitRate 缓存命中率；没有输入量时返回 0。
func (u Usage) CacheHitRate() float64 {
	if u.Input <= 0 {
		return 0
	}
	return float64(u.Cached) / float64(u.Input)
}

// Request 是一次上游请求。Temperature / TopP 用指针表达「用户没设置就不下发」，
// 让 0 值也是合法输入（比如刻意要确定性输出时设 0）。
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature *float64
	TopP        *float64
}

// FriendlyUpstreamError 把常见上游错误转成人话：直接展示上游原始 JSON
// 对用户没有可行动性——尤其是模型名写错这种本地就能说清楚的问题。
func FriendlyUpstreamError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "model not found"), strings.Contains(low, "invalid_model_error"),
		strings.Contains(low, "unknown model"), strings.Contains(low, "does not exist"):
		return "模型在这个服务上不存在，请在对话底部换个模型，或到设置里检查模型名。上游信息：" + msg
	}
	return msg
}

// Event 是流式事件。Go 侧按值传递，UI 需累积快照时自行拼接，不共享可变状态。
type Event struct {
	Type       string
	Delta      string
	ToolCall   *ToolCall
	Usage      *Usage
	StopReason string
	Err        error
}

// Streamer 是上游适配器的统一入口。
type Streamer interface {
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

// ClientConfig 是构造适配器所需的连接信息。
type ClientConfig struct {
	API     string
	BaseURL string
	APIKey  string
	Model   string
}
