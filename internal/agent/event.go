package agent

// EventKind 事件命名空间：agent.{run|turn|tool}.{phase}。上层按前缀映射到前端通道。
//
// 全集固定 14 种——每新增一种都要前端同步解码，因此宁可复用 Payload 字段也不加新 Kind。
type EventKind string

const (
	EventRunStart     EventKind = "agent.run.start"
	EventTurnStart    EventKind = "agent.turn.start"
	EventTurnDelta    EventKind = "agent.turn.delta"    // 正文增量
	EventTurnThinking EventKind = "agent.turn.thinking" // 推理增量
	EventTurnEnd      EventKind = "agent.turn.end"
	EventToolCall     EventKind = "agent.tool.call"   // 模型决定调用（参数已闭合）
	EventToolStart    EventKind = "agent.tool.start"  // 进入执行器
	EventToolResult   EventKind = "agent.tool.result" // 执行完成（含拒绝）
	EventCheckpoint   EventKind = "agent.checkpoint"
	EventCompressed   EventKind = "agent.compressed"
	EventError        EventKind = "agent.error"
	EventRunDone      EventKind = "agent.run.done"
	// EventRetry 建流瞬时错误自动重试：前端 StreamingBubble 据此显示「正在重试 N/3」。
	EventRetry EventKind = "agent.retry"
	// EventQueueDrained（PI Phase 3）steering / follow-up 队列一次 drain 取出 ≥1 条消息。
	EventQueueDrained EventKind = "agent.queue.drained"
)

// Event 跨边界事件载荷；JSON 字段一律 snake_case（AGENTS.md）。
type Event struct {
	Kind        EventKind `json:"kind"`
	RunID       string    `json:"run_id"`
	ParentRunID string    `json:"parent_run_id,omitempty"` // 子 Agent 委派：指向发起方 run
	SessionID   string    `json:"session_id"`
	Turn        int       `json:"turn"`
	Agent       string    `json:"agent,omitempty"` // 产生事件的 Agent 名（子 Agent 用于区分来源）
	Payload     any       `json:"payload,omitempty"`
}

// Sink 事件出口；Loop 只依赖此接口，便于单测注入与上层桥接 SSE / 落库。
type Sink interface {
	Emit(Event)
}

// FuncSink 函数适配器。
type FuncSink func(Event)

// Emit 实现 Sink。
func (f FuncSink) Emit(e Event) { f(e) }

// NopSink 空实现，单测静默。
type NopSink struct{}

// Emit 实现 Sink，丢弃事件。
func (NopSink) Emit(Event) {}

// RunStartPayload run 起始。
type RunStartPayload struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// DeltaPayload 增量文本；Kind 取 content / thinking。
type DeltaPayload struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// ToolCallPayload 工具调用。
type ToolCallPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`          // JSON 字符串
	Activity  string `json:"activity,omitempty"` // 面向用户的一行动作描述
}

// ToolResultPayload 工具结果。Refused 表示护栏拒绝（非故障），前端展示「已拒绝」而非错误。
type ToolResultPayload struct {
	ToolCallID    string            `json:"tool_call_id"`
	Name          string            `json:"name"`
	Content       string            `json:"content"`
	Err           string            `json:"error,omitempty"`
	DurationMs    int64             `json:"duration_ms"`
	Meta          map[string]string `json:"meta,omitempty"`
	Data          map[string]any    `json:"data,omitempty"`
	Refused       bool              `json:"refused,omitempty"`
	RefusedReason string            `json:"refused_reason,omitempty"`
	// UIHint 结果分型（tool.ToolMeta.UIHint：diff / memory…）：前端差异化渲染。
	// 显式声明优先于内容嗅探——嗅探会在流式期把普通正文误判成 diff。
	UIHint string `json:"ui_hint,omitempty"`
}

// UsagePayload token 用量。CacheRead/CacheWrite 是 Input 的拆解维度，不叠加。
type UsagePayload struct {
	Input      int `json:"input_tokens"`
	Output     int `json:"output_tokens"`
	CacheRead  int `json:"cache_read_tokens,omitempty"`
	CacheWrite int `json:"cache_creation_tokens,omitempty"`
	Total      int `json:"total_tokens"`
}

// TurnEndPayload 单轮结束：带本轮用量与是否带工具调用。
type TurnEndPayload struct {
	Usage      UsagePayload `json:"usage"`
	ToolCalls  int          `json:"tool_calls"`
	StopReason string       `json:"stop_reason,omitempty"`
	Truncated  bool         `json:"truncated,omitempty"`   // 输出被 max_tokens 截断
	RetryAfter int          `json:"retry_after,omitempty"` // 建流重试次数
	LatencyMs  int64        `json:"latency_ms"`
}

// RunDonePayload run 终态。
type RunDonePayload struct {
	Reason     string       `json:"reason"`
	StopReason string       `json:"stop_reason,omitempty"`
	Usage      UsagePayload `json:"usage"`
	Turns      int          `json:"turns"`
}

// CompressedPayload 上下文压缩证据。
type CompressedPayload struct {
	Removed   int    `json:"removed"`             // 被折叠的历史条数
	Summary   string `json:"summary,omitempty"`   // 交接摘要（确定性折叠为空）
	Truncated bool   `json:"truncated,omitempty"` // 是否按轮截断
	CutoffAt  int64  `json:"cutoff_at,omitempty"` // 压缩发生时间（毫秒）
}

// CheckpointPayload 检查点写入位点。
type CheckpointPayload struct {
	Turn int `json:"turn"`
}

// RetryPayload 建流瞬时错误重试。Attempt 从 1 起（首次重试为 1），DelayMs 是本次退避时长。
type RetryPayload struct {
	Attempt int   `json:"attempt"`
	DelayMs int64 `json:"delay_ms"`
}

// ErrorPayload 失败事件。Kind 为机器可读类别（timeout/rate_limited/auth/context_length/connection/upstream）。
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Kind    string `json:"kind,omitempty"`
}

// QueueDrainedPayload 队列取出可见性事件：让前端 / 审计感知「消息已入队并被消费」。
type QueueDrainedPayload struct {
	Queue string `json:"queue"`             // "steering" | "follow_up"
	Count int    `json:"count"`             // 本次 drain 取出的消息数
	Mode  string `json:"mode,omitempty"`    // QueueMode（one-at-a-time / all）
	Turn  int    `json:"turn"`              // 关联轮次
}
