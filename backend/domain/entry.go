// 会话条目聚合根：消息的各形态（Payload / VO / REQ / RESP）与附件入参。
// WorkBaby 不再持有条目链（消息真身在 PI 的 JSONL 里），这里只保留跨边界要用到的形状。
package domain

// HealthRESP 存活探测出参。
type HealthRESP struct {
	OK bool `json:"ok"`
}

// MessageImage 是消息附带的图片（识图输入）；Base64 不含 data: 前缀。
type MessageImage struct {
	MIME   string `json:"mime"`
	Base64 string `json:"base64"`
}

// ToolCall 是一次工具调用；Args 保留原始 JSON 便于回放与展示。
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Label string         `json:"label,omitempty"`
	Args  map[string]any `json:"args,omitempty"`
}

// MessageVO 前端渲染用的扁平消息（从 PI 的 JSONL 投影而来）。
type MessageVO struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Type       string         `json:"type"`
	Thinking   string         `json:"thinking,omitempty"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
	Images     []MessageImage `json:"images,omitempty"`
	Usage      *UsageVO       `json:"usage,omitempty"`
	CreatedAt  int64          `json:"created_at"`
	LatencyMs  int64          `json:"latency_ms,omitempty"`
	// Hidden 标记这条消息不在对话流里显示（扩展注入的隐藏内容）。
	Hidden bool `json:"hidden,omitempty"`
	// Notice 非空时这条渲染成一条分隔提示（已压缩 / 分支摘要 / 换模型）而不是气泡。
	Notice string `json:"notice,omitempty"`
}

// AttachmentREQ 引用文件：只带路径，正文由后端现读，避免前端传一大坨字符串。
// 粘贴的图片没有路径，直接带 Base64 数据。
type AttachmentREQ struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Image string `json:"image_base64,omitempty"` // 粘贴图片时必填，path 为空
}

// SendMessageREQ 发消息入参。
type SendMessageREQ struct {
	SessionID   string          `json:"session_id"`
	Content     string          `json:"content"`
	Attachments []AttachmentREQ `json:"attachments,omitempty"`
}

// QueueMessageREQ 插话与排队入参。
type QueueMessageREQ struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// StopRunREQ 停止当前 run 入参。
type StopRunREQ struct {
	SessionID string `json:"session_id"`
}

// SendMessageRESP 发消息出参。
type SendMessageRESP struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
}
