// Package llm 提供 Provider 抽象、流式协议适配、tool call 归一化与 thinking 分离。
// 不 import harness / api / service / wails；Provider 实例由 registry 按配置构建。
package llm

import "strings"

// RoleType 消息角色。
type RoleType string

const (
	RoleSystem    RoleType = "system"
	RoleUser      RoleType = "user"
	RoleAssistant RoleType = "assistant"
	RoleTool      RoleType = "tool"
)

// ContentPart 多模态内容片段；Parts 非空时 Content 仅作降级文本（不支持多模态的上游）。
type ContentPart struct {
	Type     string    `json:"type"` // text / image_url
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片内容（data URI 或 http 地址）。
type ImageURL struct {
	URL string `json:"url"`
}

// Message 跨边界统一消息。
type Message struct {
	Role       RoleType      `json:"role"`
	Content    string        `json:"content"`              // 模型可见正文
	Parts      []ContentPart `json:"parts,omitempty"`      // 多模态片段；非空时各上游按自身协议展开
	Thinking   string        `json:"thinking,omitempty"`   // 推理文本，不混入 Content
	ToolCalls  []ToolCall    `json:"toolCalls,omitempty"`  // 助手消息的工具调用
	ToolCallID string        `json:"toolCallID,omitempty"` // 仅 role==tool
	ToolName   string        `json:"toolName,omitempty"`   // 仅 role==tool
	Name       string        `json:"name,omitempty"`       // 可选显示名
}

// ImageDataURL 是否图片 data URI（data:image/...）。
func (p ContentPart) ImageDataURL() string {
	if p.Type != "image_url" || p.ImageURL == nil {
		return ""
	}
	return p.ImageURL.URL
}

// ToolCall 模型返回的工具调用；只在 assistant 消息出现。
type ToolCall struct {
	ID       string       `json:"id"` // tool call 唯一 ID（OpenAI 形态）；Anthropic 用同字段
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 工具调用的元数据 + 原始 JSON 参数（不展开，便于前端展示）。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// TokenUsage token 统计。
type TokenUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
	TotalTokens      int `json:"totalTokens"`
}

// SplitDataURL 拆分 data URI（data:image/png;base64,xxx）为 mime 与 base64 载荷；非 data URI 返回空串。
func SplitDataURL(u string) (string, string) {
	const prefix = "data:"
	const marker = ";base64,"
	if len(u) <= len(prefix) || u[:len(prefix)] != prefix {
		return "", ""
	}
	rest := u[len(prefix):]
	i := strings.Index(rest, marker)
	if i < 0 {
		return "", ""
	}
	return rest[:i], rest[i+len(marker):]
}

// 构造器。
func SystemMessage(content string) *Message { return &Message{Role: RoleSystem, Content: content} }
func UserMessage(content string) *Message   { return &Message{Role: RoleUser, Content: content} }

// UserMessageWithParts 带多模态片段的用户消息（Content 保留纯文本降级口径）。
func UserMessageWithParts(content string, parts []ContentPart) *Message {
	return &Message{Role: RoleUser, Content: content, Parts: parts}
}
func AssistantMessage(content string, calls []ToolCall) *Message {
	return &Message{Role: RoleAssistant, Content: content, ToolCalls: calls}
}
func ToolMessage(toolCallID, name, content string) *Message {
	return &Message{Role: RoleTool, Content: content, ToolCallID: toolCallID, ToolName: name}
}
