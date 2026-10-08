// 内核事件定义：内核对外的唯一可观测面。
package agent

import "WorkBaby/backend/llm"

// EventKind 是内核事件的种类。
type EventKind string

// 内核事件全集：agent / turn / message / tool 四组。
const (
	EventAgentStart EventKind = "agent_start"
	EventTurnStart  EventKind = "turn_start"
	EventDelta      EventKind = "message_delta"
	EventToolStart  EventKind = "tool_execution_start"
	EventToolEnd    EventKind = "tool_execution_end"
	EventSteering   EventKind = "steering"
	EventTurnEnd    EventKind = "turn_end"
	EventAgentEnd   EventKind = "agent_end"
	EventCompressed EventKind = "compressed"
	EventContext    EventKind = "context_usage"
)

// 增量种类：正文与思考严格分离。
const (
	DeltaText     = "text"
	DeltaThinking = "thinking"
)

// Event 是内核向外发出的事件；调用方据此落库、推送与渲染。
type Event struct {
	Kind          EventKind
	Turn          int
	Turns         int
	DeltaKind     string
	Delta         string
	ToolCall      *llm.ToolCall
	ToolOK        bool
	ToolTitle     string
	ToolOutput    string
	ToolBlocked   bool
	// UserContent 是随 steering 事件注入上下文的插话正文，调用方据此补落 user 条目。
	UserContent   string
	DurationMs    int64
	StopReason    string
	Usage         *llm.Usage
	Err           error
	TokensBefore  int
	TokensAfter   int
	TokensUsed    int
	TokensWindow  int
	TokensKnown   bool // 窗口是否为确切值（false = 估算，界面须显示「未知」）
	TokensReserve int
	// ContextTokens 是这一轮发出去时上下文占用的窗口量。落库是 append-only，
	// 消息底部的「上下文」只能在这一轮结束时就已经带着它。
	ContextTokens int
	Messages      int
}
