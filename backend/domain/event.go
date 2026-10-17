// 事件与计量：SSE 信封、各事件 data 形态、每次调用的用量记录与启动引导出参。
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
	// Cached 是 Input 里命中上游缓存的部分（恒 ≤ Input）；和 Input 分开看才知道长对话省没省钱。
	Cached int `json:"cached"`
	Total  int `json:"total"`
	// Context 这一轮发出去时上下文占用的窗口量，按轮次看占用是涨还是被整理过。
	Context   int   `json:"context"`
	LatencyMs int64 `json:"latency_ms"`
	// CreatedAt 带索引：仪表盘三个聚合全按它过滤，没索引就是每次打开全表扫。
	CreatedAt int64 `gorm:"autoCreateTime:milli;index" json:"created_at"`
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
	// EventChatRetry 是 PI 正在自动重试上游：没有它，界面在重试期间看上去就是卡死。
	EventChatRetry = "chat:retry"
	// EventChatSwitched 是会话被切走了（回退分叉 / 工作区重建进程），前端必须拉新快照。
	EventChatSwitched = "chat:switched"
	// EventChatSetup 是环境安装进度（npm 装 PI 可能几十秒）。
	EventChatSetup = "chat:setup"
)

// Envelope 是 SSE 帧的统一外壳；Data 按事件名对应到下面各结构体。
type Envelope struct {
	Seq   int64  `json:"seq"`
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// StartData chat:start。
type StartData struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
}

// DeltaData chat:delta。
type DeltaData struct {
	EntryID string `json:"entry_id"`
	Kind    string `json:"kind"` // text / thinking
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
	Risk       string         `json:"risk"` // Risk* 取值
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
	Known   bool `json:"known"`   // false 表示窗口是估算值，界面读数加「约」前缀
	Reserve int  `json:"reserve"` // 留给模型输出的余量
}

// DoneData chat:done。
type DoneData struct {
	EntryID    string   `json:"entry_id"`
	StopReason string   `json:"stop_reason"` // completed / length / max_turns / aborted / error
	Usage      *UsageVO `json:"usage"`
	// MaxTokens 是本轮实际下发的输出预算：撞 length 时界面靠它说清是哪个上限用尽了。
	MaxTokens int `json:"max_tokens"`
}

// ErrorData chat:error。code 是数字字符串，与 HTTP 状态码的 int 分开。
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StoppedData chat:stopped：停止可能来自托盘或快捷键，由后端广播权威信号。
type StoppedData struct {
	Reason string `json:"reason"` // 恒为 "user"：托盘与快捷键也归为用户主动停止
}

// GapData chat:gap：重放窗口溢出，前端需拉快照对账。
type GapData struct {
	Reason string `json:"reason"` // replay_overflow：缓冲里已没有 Last-Event-ID 之后的事件
}

// RetryData chat:retry：PI 正在自动重试上游。
type RetryData struct {
	Attempt int    `json:"attempt"`
	Max     int    `json:"max"`
	Message string `json:"message"`
}

// SwitchedData chat:switched：会话被切走。
type SwitchedData struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason"` // fork / workspace_restart
}

// SetupData chat:setup：环境安装进度。
type SetupData struct {
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

// BootstrapVO 启动引导数据：一次请求把前端起步要读的东西全给到，省掉一串往返。
type BootstrapVO struct {
	Version           string            `json:"version"`
	ContractVersion   int               `json:"contract_version"` // 与前端 types/api.ts 同键
	DefaultProviderID string            `json:"default_provider_id"`
	DefaultModel      string            `json:"default_model"`
	Workspace         string            `json:"workspace"`
	Permission        string            `json:"permission"` // Permission* 取值
	Settings          map[string]string `json:"settings"`   // 全量 KV 设置
	// PiEnv 是环境闸门：ready=false 时前端进向导且不 spawn PI。
	PiEnv *PiEnvVO `json:"pi_env"`
	// ExpiredApprovals 是启动时按拒绝收口的上次残留确认。
	ExpiredApprovals []ApprovalVO `json:"expired_approvals"`
}

// PiEnvVO 是 PI 运行环境的判定结果（形态由 pi/env 产出，这里避免 domain 反向依赖）。
type PiEnvVO struct {
	NodeReady   bool   `json:"node_ready"`
	NodePath    string `json:"node_path"`
	NodeVersion string `json:"node_version"`
	NodeSource  string `json:"node_source"`
	NodeError   string `json:"node_error"`
	PiReady     bool   `json:"pi_ready"`
	PiPath      string `json:"pi_path"`
	PiVersion   string `json:"pi_version"`
	PiSource    string `json:"pi_source"`
	PiError     string `json:"pi_error"`
	ModelReady  bool   `json:"model_ready"`
	Ready       bool   `json:"ready"`
	Blocking    string `json:"blocking"`
}
