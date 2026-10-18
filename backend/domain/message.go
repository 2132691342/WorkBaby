// 消息投影形态：PI 的条目链在界面上的视图，不含任何上下文重建语义。
package domain

// UsageVO 一次上游调用的用量。
type UsageVO struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	// Cached 是 Input 里命中上游缓存的部分（恒 ≤ Input）。
	Cached  int     `json:"cached"`
	Total   int     `json:"total"`
	Context int     `json:"context"`
	Cost    float64 `json:"cost"`
}

// ToolCall 模型发起的一次工具调用。
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Label string         `json:"label,omitempty"`
	Args  map[string]any `json:"args,omitempty"`
}

// MessageImage 消息里的图片。
type MessageImage struct {
	MIME   string `json:"mime"`
	Base64 string `json:"base64"`
}

// MessageVO 一条消息。
type MessageVO struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"` // user / assistant / tool / notice
	Type       string         `json:"type"` // text / tool / notice
	Content    string         `json:"content,omitempty"`
	Thinking   string         `json:"thinking,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	IsError    bool           `json:"is_error,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
	Images     []MessageImage `json:"images,omitempty"`
	Usage      *UsageVO       `json:"usage,omitempty"`
	CreatedAt  int64          `json:"created_at"`
	// Hidden 是扩展注入的隐藏内容：不渲染成气泡。
	Hidden bool `json:"hidden,omitempty"`
	// Notice 非空时渲染成分隔提示条（已压缩 / 换模型）而不是气泡。
	Notice string `json:"notice,omitempty"`
}

// 角色取值。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleNotice    = "notice"
)
