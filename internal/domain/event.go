package domain

// TokenUsageDO 每次上游调用一行，用于用量归因与会话统计。
type TokenUsageDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	SessionID string `gorm:"size:64;index" json:"session_id"`
	EntryID   string `gorm:"size:64" json:"entry_id"`
	Provider  string `gorm:"size:64" json:"provider"`
	Model     string `gorm:"size:128" json:"model"`
	Input     int    `json:"input"`
	Output    int    `json:"output"`
	// Cached 命中上游缓存的输入量；和 Input 分开看才知道长对话到底省没省钱。
	Cached int `json:"cached"`
	Total  int `json:"total"`
	// Context 这一轮发出去时上下文占用的窗口量，按轮次看占用是涨还是被整理过。
	Context  int   `json:"context"`
	LatencyMs int64 `json:"latency_ms"`
	CreatedAt int64 `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (TokenUsageDO) TableName() string { return "token_usages" }

// 事件名：SSE 的 event 字段取值。
const (
	EventChatStart      = "chat:start"
	EventChatDelta      = "chat:delta"
	EventChatToolStart  = "chat:tool_start"
	EventChatToolEnd    = "chat:tool_end"
	EventChatApproval   = "chat:approval"
	EventChatCompressed = "chat:compressed"
	EventChatUser       = "chat:user"
	EventChatDone       = "chat:done"
	EventChatStopped    = "chat:stopped"
	EventChatError      = "chat:error"
	EventChatContext    = "chat:context"
	EventChatGap        = "chat:gap"
)

// Envelope 是 SSE 帧的统一外壳；Data 按事件名对应到下面各结构体。
type Envelope struct {
	Seq   int64  `json:"seq"`
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// StartData chat:start。
type StartData struct {
	RunID    string `json:"run_id"`
	SessionID string `json:"session_id"`
}

// DeltaData chat:delta。
type DeltaData struct {
	EntryID string `json:"entry_id"`
	Kind    string `json:"kind"`
	Delta   string `json:"delta"`
}

// ToolStartData chat:tool_start。
type ToolStartData struct {
	ToolCallID string         `json:"tool_call_id"`
	Tool       string         `json:"tool"`
	Label      string         `json:"label"`
	Args       map[string]any `json:"args"`
}

// ToolEndData chat:tool_end。
type ToolEndData struct {
	ToolCallID string `json:"tool_call_id"`
	OK         bool   `json:"ok"`
	Title      string `json:"title"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// ApprovalData chat:approval。
type ApprovalData struct {
	ApprovalID string         `json:"approval_id"`
	ToolCallID string         `json:"tool_call_id"`
	Tool       string         `json:"tool"`
	Label      string         `json:"label"`
	Args       map[string]any `json:"args"`
	Risk       string         `json:"risk"`
	Reason     string         `json:"reason"`
}

// CompressedData chat:compressed。
type CompressedData struct {
	TokensBefore int `json:"tokens_before"`
	TokensAfter  int `json:"tokens_after"`
}

// UserData chat:user：插话 / 排队消息已注入上下文并落库，界面据此把它补进时间线。
type UserData struct {
	EntryID string `json:"entry_id"`
	Content string `json:"content"`
}

// ContextData chat:context：每轮广播一次上下文占用，供输入区显示水位。
type ContextData struct {
	Used    int  `json:"used"`
	Window  int  `json:"window"`
	Ratio   int  `json:"ratio"`   // 0-100 的整数百分比，前端不必自己算
	Known   bool `json:"known"`   // false 表示窗口是估算值，界面应显示「未知」而不是具体数字
	Reserve int  `json:"reserve"` // 留给模型输出的余量
}

// DoneData chat:done。
type DoneData struct {
	EntryID    string  `json:"entry_id"`
	StopReason string  `json:"stop_reason"`
	Usage      *UsageVO `json:"usage"`
}

// ErrorData chat:error。
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// StoppedData chat:stopped：用户点了停止。
// 单独发这个事件而不是让前端自己改状态：停止可能来自托盘或快捷键，
// 由后端广播一次权威信号，界面不会出现「后端已停、界面还在转圈」。
type StoppedData struct {
	Reason string `json:"reason"`
}

// GapData chat:gap：重放窗口溢出，前端需拉快照对账。
type GapData struct {
	Reason string `json:"reason"`
}

// BootstrapVO 启动引导数据。
type BootstrapVO struct {
	Version          string            `json:"version"`
	ContractVersion  int               `json:"contract_version"`
	DefaultProviderID string           `json:"default_provider_id"`
	DefaultModel     string            `json:"default_model"`
	Workspace        string            `json:"workspace"`
	Permission       string            `json:"permission"`
	PythonReady      bool              `json:"python_ready"`
	Settings         map[string]string `json:"settings"`
}
