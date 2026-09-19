package llm

import (
	"context"

	"WorkBaby/internal/domain"
)

// ProviderKind Provider 协议族；与 domain.ProviderKind 同义（避免循环 import，参数化时直接传这个）。
type ProviderKind = domain.ProviderKind

// Provider 抽象。Stream 返回的 channel 由实现负责 close；流中错误经 StreamChunk{Err} 传递；
// Chat 的 ToolCalls 是一次性完整列表，Stream 模式下以归一化 ToolCall 分散在多个 chunk。
type Provider interface {
	Name() string
	Kind() ProviderKind
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	Models(ctx context.Context) ([]ModelInfo, error)
	Ping(ctx context.Context) error
}

// ChatRequest 单次对话输入；Provider 不感知会话，组合由上游 (harness) 完成。
type ChatRequest struct {
	Model       string           // 模型名（必填）
	Messages    []*Message       // 完整历史（system 在首）
	Tools       []ToolDefinition // 暴露给模型
	Temperature *float64         // nil = 使用合并后的默认
	TopP        *float64
	MaxTokens   *int
	Stop        []string
	Thinking    *ThinkingConfig
	ExtraBody   map[string]any
	User        string
	// SessionID 会话标识：OpenAI 侧用作 prompt_cache_key，让同一会话稳定命中同一缓存分片。
	SessionID string
}

// ChatResponse 单次非流响应。
type ChatResponse struct {
	Message    Message              // 助手完整回复
	ToolCalls  []NormalizedToolCall // 工具调用列表
	Usage      TokenUsage
	StopReason string // end_turn / tool_use / max_tokens / length / stop
}

// StreamChunk 流式增量；harness 据此持续 Emit。
//
// Delta.Content / Delta.Thinking 是字符串片段，不是逐 token 而是事件序列。
// FinalUsage 在最后一片填充（很多 Provider 在末片发 usage）。
type StreamChunk struct {
	Delta        Message
	ToolCall     *NormalizedToolCall // 某个 tool_call 已完整闭合时填一次
	FinalUsage   *TokenUsage
	FinishReason *string
	Err          error
}

// ModelInfo 模型清单项（Ping/Models 用）。
type ModelInfo struct {
	ID    string `json:"id"`
	Owned bool   `json:"owned_by,omitempty"`
}

// ToolDefinition 暴露给模型的工具描述。
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// ThinkingConfig 推理开关；Provider 适配层翻译为各家私有字段。
type ThinkingConfig struct {
	// Type: enabled/disabled/adaptive；各 Provider 翻译：
	//   Anthropic → thinking.type=enabled + budget_tokens
	//   GLM → thinking.type=enabled（在 ExtraBody 注入）
	//   DeepSeek → 不需要（reasoning_content 自动返回）
	Type string `json:"type,omitempty"`
	// BudgetTokens 思维预算（Anthropic 等）。
	BudgetTokens int `json:"budget_tokens,omitempty"`
}
