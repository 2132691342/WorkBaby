// Package agent 是 WorkBaby 的原生智能体引擎：模型调用、工具执行、会话落盘。
// 本包只依赖标准库与 pkg，不知道 HTTP、界面与上层编排的存在。
package agent

import (
	"context"
	"encoding/json"
)

// Role 是消息角色。
type Role string

const (
	// RoleSystem 系统提示（含工具声明）
	RoleSystem Role = "system"
	// RoleUser 用户消息
	RoleUser Role = "user"
	// RoleAssistant 模型回复
	RoleAssistant Role = "assistant"
	// RoleTool 工具结果
	RoleTool Role = "tool"
)

// BlockKind 是内容块种类。
type BlockKind string

const (
	// KindText 正文
	KindText BlockKind = "text"
	// KindThinking 思考过程
	KindThinking BlockKind = "thinking"
	// KindImage 图片（唯一的二进制通道）
	KindImage BlockKind = "image"
	// KindToolCall 模型要求的工具调用
	KindToolCall BlockKind = "tool_call"
)

// Block 是消息里的一个内容块。一个块只填自己那一族的字段，
// 不用 interface：引擎侧所有消费点都要跨块种类取字段，接口会把它们拆散。
type Block struct {
	// Kind 块种类
	Kind BlockKind `json:"kind"`
	// Text 正文或思考文本
	Text string `json:"text,omitempty"`
	// ImageMime 图片 MIME
	ImageMime string `json:"image_mime,omitempty"`
	// ImageBase64 图片内容（base64）
	ImageBase64 string `json:"image_base64,omitempty"`
	// CallId 工具调用 ID
	CallId string `json:"call_id,omitempty"`
	// ToolName 工具名
	ToolName string `json:"tool_name,omitempty"`
	// ToolArgs 工具入参（JSON 原文）
	ToolArgs string `json:"tool_args,omitempty"`
}

// Message 是一条对话消息。
type Message struct {
	// Role 角色
	Role Role `json:"role"`
	// Blocks 内容块，按模型吐出的原始顺序
	Blocks []Block `json:"blocks"`
	// ToolCallId 工具结果消息对应的那次调用
	ToolCallId string `json:"tool_call_id,omitempty"`
	// ToolName 工具结果对应的工具名
	ToolName string `json:"tool_name,omitempty"`
	// IsError 工具结果是否失败
	IsError bool `json:"is_error,omitempty"`
	// Details 工具结果的附加信息（待办清单、差异行都在这里）
	Details map[string]any `json:"details,omitempty"`
	// Usage 这条助手消息的用量
	Usage *Usage `json:"usage,omitempty"`
	// StopReason stop / tool_calls / length / error / aborted
	StopReason string `json:"stop_reason,omitempty"`
	// ErrorMessage 上游失败原话
	ErrorMessage string `json:"error_message,omitempty"`
	// ProviderName 产生这条的服务商
	ProviderName string `json:"provider_name,omitempty"`
	// ModelId 产生这条的模型
	ModelId string `json:"model_id,omitempty"`
	// CreatedAt 产生时刻（毫秒）
	CreatedAt int64 `json:"created_at"`
}

// Text 取消息的纯文本：用户与工具的正文就是 text 块拼起来。
func (m *Message) Text() string {
	out := ""
	for _, block := range m.Blocks {
		if block.Kind == KindText {
			out += block.Text
		}
	}
	return out
}

// ToolCalls 取消息里的工具调用块。
func (m *Message) ToolCalls() []Block {
	out := make([]Block, 0, 4)
	for _, block := range m.Blocks {
		if block.Kind == KindToolCall {
			out = append(out, block)
		}
	}
	return out
}

// Usage 是一次调用的 token 用量。
type Usage struct {
	// InputTokens 输入 token
	InputTokens int `json:"input_tokens"`
	// OutputTokens 输出 token
	OutputTokens int `json:"output_tokens"`
	// CacheReadTokens 命中缓存的输入 token
	CacheReadTokens int `json:"cache_read_tokens"`
	// CacheWriteTokens 写入缓存的输入 token
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// TotalTokens 用量合计。
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// ToolSpec 是交给模型的工具声明。
type ToolSpec struct {
	// Name 工具名
	Name string `json:"name"`
	// Description 什么时候该用它
	Description string `json:"description"`
	// Schema 入参 JSON Schema
	Schema map[string]any `json:"schema"`
}

// EventKind 是流式事件种类。
type EventKind string

const (
	// EventText 正文增量
	EventText EventKind = "text"
	// EventThinking 思考增量
	EventThinking EventKind = "thinking"
	// EventToolStart 一个工具调用开始（名字已知）
	EventToolStart EventKind = "tool_start"
	// EventToolDelta 工具入参增量
	EventToolDelta EventKind = "tool_delta"
	// EventStop 本次回复结束
	EventStop EventKind = "stop"
	// EventUsage 用量读数（可能出现在任何一个分片上）
	EventUsage EventKind = "usage"
)

// StreamEvent 是上游流式分片归一之后的事件。
type StreamEvent struct {
	// Kind 事件种类
	Kind EventKind `json:"kind"`
	// Index 内容块下标（工具调用靠它归并）
	Index int `json:"index"`
	// Text 增量文本
	Text string `json:"text,omitempty"`
	// CallId 工具调用 ID
	CallId string `json:"call_id,omitempty"`
	// ToolName 工具名
	ToolName string `json:"tool_name,omitempty"`
	// Usage 累计用量
	Usage *Usage `json:"usage,omitempty"`
	// StopReason 结束原因
	StopReason string `json:"stop_reason,omitempty"`
}

// ModelRef 是一次调用要用到的模型与凭据。
type ModelRef struct {
	// ProviderName 服务商名
	ProviderName string
	// ModelId 模型 ID
	ModelId string
	// BaseUrl 接口根地址
	BaseUrl string
	// ApiKey 密钥
	ApiKey string
	// ContextWindow 上下文窗口 token 数
	ContextWindow int
	// MaxTokens 单次输出上限
	MaxTokens int
	// SupportsReasoning 是否支持思考
	SupportsReasoning bool
}

// Request 是一次模型调用的入参。
type Request struct {
	// Model 模型与凭据
	Model ModelRef
	// System 系统提示
	System string
	// Messages 上下文消息（不含系统提示）
	Messages []Message
	// Tools 可用工具声明
	Tools []ToolSpec
	// MaxTokens 本次输出上限；0 表示用模型的默认值
	MaxTokens int
	// Temperature 采样温度；nil 表示用上游默认值
	Temperature *float64
}

// Streamer 把一次请求变成一串归一事件。
// 实现方负责把上游协议翻译成 StreamEvent，并在流结束时关闭通道；
// 失败必须以一条带 ErrorMessage 的结束事件收场，不向上抛。
type Streamer interface {
	// Stream 发起一次流式调用
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

// AssistantFrom 把一串流式事件累积成一条助手消息。
// 工具入参是分片到达的：按 Index 归并，最后拼成完整 JSON 原文。
type assistantBuilder struct {
	// blocks 正文与思考块，按到达顺序
	blocks []Block
	// calls 工具调用块，按上游 index 归并
	calls map[int]*Block
	// callOrder 工具调用的到达顺序
	callOrder []int
	usage     *Usage
	stop      string
	errMsg    string
}

// fail 记下上游的失败原话：结束时它同时充当结束原因与错误文案。
func (b *assistantBuilder) fail(message string) {
	b.errMsg = message
	b.stop = "error"
}

// errorMessage 取失败原话。
func (b *assistantBuilder) errorMessage() string { return b.errMsg }

// apply 吃进一个事件。
func (b *assistantBuilder) apply(ev StreamEvent) {
	switch ev.Kind {
	case EventText:
		b.appendText(KindText, ev.Text)
	case EventThinking:
		b.appendText(KindThinking, ev.Text)
	case EventToolStart:
		b.startCall(ev.Index, ev.CallId, ev.ToolName)
	case EventToolDelta:
		b.appendCallArgs(ev.Index, ev.Text)
	case EventUsage:
		if ev.Usage != nil {
			b.usage = ev.Usage
		}
	case EventStop:
		b.stop = ev.StopReason
	}
}

// appendText 追加一段文本：与上一个块同种就续写，换种就另开一块。
// 槽位分配放这里而不是在解析流的地方：只有块表自己知道
// 「上一段是什么」，猜错的代价是整段文字被重置掉。
func (b *assistantBuilder) appendText(kind BlockKind, text string) {
	if text == "" {
		return
	}
	if n := len(b.blocks); n > 0 && b.blocks[n-1].Kind == kind {
		b.blocks[n-1].Text += text
		return
	}
	b.blocks = append(b.blocks, Block{Kind: kind, Text: text})
}

// startCall 登记一个工具调用。按上游的 index 归并而不是按到达顺序：
// 并行工具调用的分片理论上可以交错，靠「最后一个块」拼入参会串到一起。
func (b *assistantBuilder) startCall(index int, callId, name string) {
	if b.calls == nil {
		b.calls = map[int]*Block{}
	}
	if _, seen := b.calls[index]; !seen {
		b.callOrder = append(b.callOrder, index)
	}
	b.calls[index] = &Block{Kind: KindToolCall, CallId: callId, ToolName: name}
}

// appendCallArgs 把入参增量拼到对应调用上。
func (b *assistantBuilder) appendCallArgs(index int, text string) {
	if text == "" {
		return
	}
	if call, ok := b.calls[index]; ok {
		call.ToolArgs += text
	}
}

// message 收尾成一条助手消息（流式期间只用来取块表，落盘那条由引擎重新组装）。
func (b *assistantBuilder) message(model ModelRef) Message {
	blocks := make([]Block, 0, len(b.blocks)+len(b.callOrder))
	for _, block := range b.blocks {
		if block.Kind == "" || block.Text == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	for _, index := range b.callOrder {
		call := b.calls[index]
		if call == nil || call.ToolName == "" {
			continue
		}
		blocks = append(blocks, *call)
	}
	return Message{
		Role:         RoleAssistant,
		Blocks:       blocks,
		Usage:        b.usage,
		StopReason:   b.stop,
		ProviderName: model.ProviderName,
		ModelId:      model.ModelId,
	}
}

// schemaOf 把结构体形状的入参声明转成 JSON Schema 片段，省得每个工具手拼 map。
func schemaOf(properties map[string]any, required ...string) map[string]any {
	props := map[string]any{}
	for name, desc := range properties {
		props[name] = map[string]any{"type": "string", "description": desc}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// decodeArgs 把工具入参原文解成通用形状；解不开就返回原文，让工具自己报错。
func decodeToolArgs(raw string) any {
	if raw == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}
