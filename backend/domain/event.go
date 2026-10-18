// 事件与引导出参：SSE 信封、各事件 data 形态、启动引导数据。
package domain

// 事件名：SSE 的 event 字段取值。
const (
	EventChatStart      = "chat:start"
	EventChatDelta      = "chat:delta"
	EventChatToolStart  = "chat:tool_start"
	EventChatToolEnd    = "chat:tool_end"
	EventChatUser       = "chat:user"
	EventChatDone       = "chat:done"
	EventChatStopped    = "chat:stopped"
	EventChatError      = "chat:error"
	EventChatContext    = "chat:context"
	EventChatGap        = "chat:gap"
	EventChatCompressed = "chat:compressed"
	// EventChatRetry 是 PI 正在自动重试上游：没有它，界面在重试期间看上去就是卡死。
	EventChatRetry = "chat:retry"
	// EventChatSwitched 是会话被切走了（换工作区 / 引擎重启），前端必须拉新快照。
	EventChatSwitched = "chat:switched"
	// EventChatSetup 是环境安装进度（npm 装 PI 可能几十秒）。
	EventChatSetup = "chat:setup"
	// EventChatAsk 是引擎里的扩展在问用户（confirm / select / input）。
	// 问题来自 PI 进程，工作区级通道，所以 session_id 可能为空。
	EventChatAsk = "chat:ask"
	// EventChatAskClosed 是那条问题已经不需要再答了（已答 / 会话中止 / 进程退出）。
	EventChatAskClosed = "chat:ask_closed"
	// EventChatNotify 是引擎扩展的即时通知（fire-and-forget，无需应答）。
	EventChatNotify = "chat:notify"
	// EventChatStatus 是引擎扩展设置的状态条文本；空 text 表示清除。
	EventChatStatus = "chat:status"
	// EventChatWidget 是引擎扩展推送的小部件文本行。
	EventChatWidget = "chat:widget"
	// EventChatTitle 是引擎请求改标题。
	EventChatTitle = "chat:title"
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

// AskData chat:ask：引擎（或其扩展）在等用户回答。
type AskData struct {
	ID      string   `json:"id"`
	Method  string   `json:"method"` // confirm / select / input
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Options []string `json:"options"`
}

// AskClosedData chat:ask_closed。
type AskClosedData struct {
	ID     string `json:"id"`
	Reason string `json:"reason"` // answered / cancelled / gone
}

// CompressedData chat:compressed。
type CompressedData struct {
	TokensBefore int `json:"tokens_before"`
	TokensAfter  int `json:"tokens_after"`
}

// UserData chat:user：插话 / 排队消息已注入上下文，界面据此把它补进时间线。
type UserData struct {
	EntryID string `json:"entry_id"`
	Content string `json:"content"`
}

// ContextData chat:context：每轮广播一次上下文占用，供输入区显示水位。
type ContextData struct {
	Used    int  `json:"used"`
	Window  int  `json:"window"`
	Ratio   int  `json:"ratio"`
	Known   bool `json:"known"`
	Reserve int  `json:"reserve"`
}

// DoneData chat:done。
type DoneData struct {
	EntryID    string   `json:"entry_id"`
	StopReason string   `json:"stop_reason"`
	Usage      *UsageVO `json:"usage"`
	MaxTokens  int      `json:"max_tokens"`
}

// ErrorData chat:error。code 是数字字符串，与 HTTP 状态码的 int 分开。
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StoppedData chat:stopped：停止可能来自托盘或快捷键，由后端广播权威信号。
type StoppedData struct {
	Reason string `json:"reason"`
}

// GapData chat:gap：重放窗口溢出，前端需拉快照对账。
type GapData struct {
	Reason string `json:"reason"`
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
	Reason    string `json:"reason"`
}

// SetupData chat:setup：环境安装进度。
type SetupData struct {
	Phase    string `json:"phase"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

// NotifyData chat:notify：扩展的即时通知。Level 是 info / warning / error。
type NotifyData struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// StatusData chat:status：扩展设置的状态条；空 Text 表示清除那个键。
type StatusData struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// WidgetData chat:widget：扩展推送的小部件文本行；空 Lines 表示清除。
type WidgetData struct {
	Key       string   `json:"key"`
	Lines     []string `json:"lines"`
	Placement string   `json:"placement"`
}

// TitleData chat:title：引擎请求改标题（展示层自定是否采纳）。
type TitleData struct {
	Title string `json:"title"`
}

// BootstrapVO 启动引导数据：一次请求把前端起步要读的东西全给到，省掉一串往返。
type BootstrapVO struct {
	Version         string `json:"version"`
	ContractVersion int    `json:"contract_version"`
	// DefaultProvider / DefaultModel 来自 PI 的 settings.json。
	DefaultProvider string `json:"default_provider"`
	DefaultModel    string `json:"default_model"`
	Workspace       string `json:"workspace"`
	// PiEnv 是环境闸门：ready=false 时前端进向导且不 spawn PI。
	PiEnv    *PiEnvVO          `json:"pi_env"`
	Settings map[string]string `json:"settings"`
}

// WarmupRESP 首屏预热聚合：设置页各面板要读的清单一次拉齐，
// 前端不再逐面板手点刷新。全部只读磁盘与配置，不拉起 PI 进程。
type WarmupRESP struct {
	Skills     []SkillVO          `json:"skills"`
	Providers  *ProviderListRESP  `json:"providers"`
	Tools      *ToolListRESP      `json:"tools"`
	Extensions *ExtensionListRESP `json:"extensions"`
	MCP        *MCPListRESP       `json:"mcp"`
}

// PiEnvVO 是 PI 运行环境的判定结果（形态由 pi/env 产出，这里避免 domain 反向依赖）。
type PiEnvVO struct {
	NodeReady   bool   `json:"node_ready"`
	NodePath    string `json:"node_path"`
	NodeVersion string `json:"node_version"`
	NodeSource  string `json:"node_source"`
	NodeError   string `json:"node_error"`

	PythonReady   bool   `json:"python_ready"`
	PythonPath    string `json:"python_path"`
	PythonVersion string `json:"python_version"`
	PythonSource  string `json:"python_source"`

	PiReady   bool   `json:"pi_ready"`
	PiPath    string `json:"pi_path"`
	PiVersion string `json:"pi_version"`
	PiSource  string `json:"pi_source"`
	PiError   string `json:"pi_error"`

	AgentDir   string `json:"agent_dir"`
	ModelReady bool   `json:"model_ready"`
	Ready      bool   `json:"ready"`
	Blocking   string `json:"blocking"`
}
