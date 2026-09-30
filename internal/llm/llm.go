// Package llm 提供归一化消息模型与三个上游协议的流式适配。
package llm

import (
	"context"

	"WorkBaby/internal/pkg"
)

// 消息角色。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// 停止原因。
const (
	StopStop     = "stop"
	StopLength   = "length"
	StopToolUse  = "tool_use"
	StopError    = "error"
	StopAborted  = "aborted"
)

// 事件类型。
const (
	EventDelta    = "delta"
	EventThinking = "thinking"
	EventToolCall = "tool_call"
	EventDone     = "done"
	EventError    = "error"
)

var (
	ErrNoAPIKey  = pkg.New(3001, "缺少 API Key", "")
	ErrBadStream = pkg.New(3105, "上游返回了无法解析的流", "")
)

// Message 是归一化消息：assistant 的正文与思考严格分离，工具调用与结果各自独立。
type Message struct {
	Role       string
	Content    string
	Thinking   string
	ToolCalls  []ToolCall
	ToolCallID string
	IsError    bool
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
	Cached   int
	LatencyMs int64
}

// CacheHitRate 缓存命中率；没有输入量时返回 0。
func (u Usage) CacheHitRate() float64 {
	if u.Input <= 0 {
		return 0
	}
	return float64(u.Cached) / float64(u.Input)
}

// Request 是一次上游请求。
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolDef
	MaxTokens   int
	Temperature float64
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
