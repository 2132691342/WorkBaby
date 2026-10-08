package domain

// EntryDO 会话条目：append-only，parent_id 串成树，不做更新与删除。
type EntryDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	SessionID  string `gorm:"size:64;index:idx_entry_session,priority:1" json:"session_id"`
	ParentID   string `gorm:"size:64" json:"parent_id"`
	Seq        int    `gorm:"index:idx_entry_session,priority:2" json:"seq"`
	Type       string `gorm:"size:16" json:"type"`
	Role       string `gorm:"size:16" json:"role"`
	PayloadJSON string `gorm:"type:text" json:"payload_json"`
	UsageJSON  string `gorm:"type:text" json:"usage_json"`
	CreatedAt  int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (EntryDO) TableName() string { return "entries" }

// HealthRESP 存活探测出参。
type HealthRESP struct {
	OK bool `json:"ok"`
}

// MessageImage 是消息附带的图片（识图输入）；Base64 不含 data: 前缀。
type MessageImage struct {
	MIME   string `json:"mime"`
	Base64 string `json:"base64"`
}

// MessagePayload 是 message 类条目的 payload 结构。
type MessagePayload struct {
	Thinking   string         `json:"thinking,omitempty"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
	LatencyMs  int64          `json:"latency_ms,omitempty"`
	Images     []MessageImage `json:"images,omitempty"`
}

// ToolCall 是一次工具调用；Args 保留原始 JSON 便于回放与展示。
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Label string         `json:"label,omitempty"`
	Args  map[string]any `json:"args,omitempty"`
}

// UsageVO token 与耗时。
type UsageVO struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	// Cached 是这次输入里命中上游缓存的部分，缓存命中率由它除以 Input 得出。
	Cached int `json:"cached"`
	Total  int `json:"total"`
	// Context 是这一轮发出去时上下文占了多少窗口（系统提示 + 历史消息）。
	// 用户判断「还能聊多久」靠的是它，不是这一轮本身花了多少。
	Context   int   `json:"context"`
	LatencyMs int64 `json:"latency_ms"`
}

// MessageVO 前端渲染用的扁平消息。
type MessageVO struct {
	ID         string     `json:"id"`
	Role       string     `json:"role"`
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
	RunID    string `json:"run_id"`
	EntryID  string `json:"entry_id"`
	SessionID string `json:"session_id"`
}
