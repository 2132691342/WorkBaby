// 消息投影形态：PI 的条目链在界面上的视图，不含任何上下文重建语义。
package domain

// UsageVO 一次上游调用的用量。
// 上游把缓存命中量与未命中量分开报：提示词总量 = Input + Cached + CacheWrite。
type UsageVO struct {
	// Input 是未命中上游缓存的提示词；Cached 是命中部分；CacheWrite 是新写入部分。
	Input      int `json:"input"`
	Output     int `json:"output"`
	Cached     int `json:"cached"`
	CacheWrite int `json:"cache_write,omitempty"`
	// Total 是提示词总量加输出；Context 只是提示词总量（一次调用送上去的全部）。
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

// MessageBlock 助手消息里一段有顺序的内容：thinking / text / tool。
// Kind 为 tool 时只带工具标识与参数，结果在紧随其后的 tool 消息里。
type MessageBlock struct {
	Kind       string         `json:"kind"`
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	Label      string         `json:"label,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
}

// MessageVO 一条消息。
type MessageVO struct {
	ID       string `json:"id"`
	Role     string `json:"role"` // user / assistant / tool / notice
	Type     string `json:"type"` // text / tool / notice
	Content  string `json:"content,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	// Blocks 是正文 / 思考 / 工具调用的原始顺序；Content 与 Thinking 只是它的拼接结果。
	Blocks     []MessageBlock `json:"blocks,omitempty"`
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

// 内容块取值：界面按这个顺序内联渲染正文 / 思考 / 工具卡。
const (
	BlockText     = "text"
	BlockThinking = "thinking"
	BlockTool     = "tool"
)
