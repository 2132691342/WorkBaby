// 协议记录解析：把 PI 的 JSONL 会话事件与扩展 UI 记录解析成统一的事件形态。
// json tag 忠实 PI 的协议字段名（camelCase），走出这个文件就换上 WorkBaby 的形态。
package pi

import "encoding/json"

// EventKind 是 PI 会话事件的种类。
type EventKind string

const (
	// agent_start / agent_end / agent_settled：一次 run 的起止。
	KindAgentStart   EventKind = "agent_start"
	KindAgentEnd     EventKind = "agent_end"
	KindAgentSettled EventKind = "agent_settled"

	KindTurnStart EventKind = "turn_start"
	KindTurnEnd   EventKind = "turn_end"

	KindMessageStart  EventKind = "message_start"
	KindMessageUpdate EventKind = "message_update"
	KindMessageEnd    EventKind = "message_end"

	KindToolStart  EventKind = "tool_execution_start"
	KindToolUpdate EventKind = "tool_execution_update"
	KindToolEnd    EventKind = "tool_execution_end"

	KindCompactionStart EventKind = "compaction_start"
	KindCompactionEnd   EventKind = "compaction_end"

	KindRetryStart EventKind = "auto_retry_start"
	KindRetryEnd   EventKind = "auto_retry_end"

	KindQueueUpdate EventKind = "queue_update"

	// KindExtensionUI 是扩展 UI 请求（审批闸门与通知都走它）。
	KindExtensionUI EventKind = "extension_ui_request"
)

// Event 是一条解析后的 PI 事件。各字段按 Kind 取用，未用到的保持零值。
type Event struct {
	Kind EventKind

	// —— 扩展 UI ——
	ID      string   `json:"id"`
	Method  string   `json:"method"`
	Title   string   `json:"title"`
	Body    string   `json:"-"`
	Text    string   `json:"text"`
	Options []string `json:"options"`
	// NotifyType 是 notify 的级别：info / warning / error。
	NotifyType string `json:"notifyType"`
	// StatusKey / StatusText 是 setStatus 的键值对；空 Text 表示清除。
	StatusKey  string `json:"statusKey"`
	StatusText string `json:"statusText"`
	// WidgetLines 是 setWidget 的文本行；空表示清除。
	WidgetLines     []string `json:"widgetLines"`
	WidgetPlacement string   `json:"widgetPlacement"`

	// —— 消息 ——
	// 同一个 json 字段两种形态：扩展 UI 的 message 是字符串，消息事件的 message 是对象。
	MessageRaw json.RawMessage `json:"message"`
	// Usage 是流式期间的最新累计用量（message_update 才有）。
	Usage *Usage `json:"usage"`
	// Delta 是 assistantMessageEvent 里的增量。
	Delta        string    `json:"delta"`
	DeltaType    string    `json:"-"`
	ContentIndex int       `json:"contentIndex"`
	ToolCallID   string    `json:"toolCallId"`
	ToolName     string    `json:"toolName"`
	ToolCall     *ToolCall `json:"toolCall"`

	// —— 工具执行 ——
	Args       map[string]any `json:"args"`
	Result     *ToolResult    `json:"-"`
	Partial    *ToolResult    `json:"partialResult"`
	IsError    bool           `json:"isError"`
	DurationMs int64          `json:"durationMs"`

	// —— 回合 / 运行 ——
	ToolResults []json.RawMessage `json:"toolResults"`
	WillRetry   bool              `json:"willRetry"`
	Aborted     bool              `json:"aborted"`
	// StopReason 与 Role 从 message_end 的 message 对象里提出来。
	StopReason string `json:"-"`
	Role       string `json:"-"`

	// —— 压缩 ——
	CompactionReason string            `json:"reason"`
	CompactResult    *CompactionResult `json:"-"`
	ErrorMessage     string            `json:"errorMessage"`

	// —— 重试 ——
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"maxAttempts"`
	DelayMs     int64  `json:"delayMs"`
	Success     bool   `json:"success"`
	FinalError  string `json:"finalError"`

	// 原始记录保留一份，排障时按需取。
	Raw json.RawMessage `json:"-"`
}

// Usage 是 PI 的用量口径（snake 转换由上层做）。
type Usage struct {
	Input       int `json:"input"`
	Output      int `json:"output"`
	CacheRead   int `json:"cacheRead"`
	CacheWrite  int `json:"cacheWrite"`
	Reasoning   int `json:"reasoning"`
	TotalTokens int `json:"totalTokens"`
	Cost        struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cacheRead"`
		CacheWrite float64 `json:"cacheWrite"`
		Total      float64 `json:"total"`
	} `json:"cost"`
}

// ToolCall 是模型发起的一次工具调用。
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResult 是工具执行的结果。
type ToolResult struct {
	Content []ContentBlock  `json:"content"`
	Details json.RawMessage `json:"details"`
}

// ContentBlock 是消息与工具结果的通用内容块。
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// CompactionResult 是一次压缩的结果。
type CompactionResult struct {
	Summary              string `json:"summary"`
	FirstKeptEntryID     string `json:"firstKeptEntryId"`
	TokensBefore         int    `json:"tokensBefore"`
	EstimatedTokensAfter int    `json:"estimatedTokensAfter"`
	Usage                *Usage `json:"usage"`
}

// Message 是一条完整消息（message_start / message_end 用）。
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Timestamp  int64           `json:"timestamp"`
	API        string          `json:"api"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Usage      *Usage          `json:"usage"`
	StopReason string          `json:"stopReason"`
	IsError    bool            `json:"isError"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
}

// assistantUpdate 是 message_update 里嵌套的增量事件。
type assistantUpdate struct {
	Type         string    `json:"type"`
	ContentIndex int       `json:"contentIndex"`
	Delta        string    `json:"delta"`
	ID           string    `json:"id"`
	ToolName     string    `json:"toolName"`
	ToolCall     *ToolCall `json:"toolCall"`
	Content      string    `json:"content"`
}

// parseEvent 把一条原始记录解析成 Event；未知类型返回 ok=false。
func parseEvent(raw []byte) (Event, bool) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return Event{}, false
	}
	ev := Event{Kind: EventKind(head.Type), Raw: raw}

	switch head.Type {
	case "agent_start", "agent_end", "agent_settled", "turn_start", "turn_end",
		"compaction_start", "compaction_end", "auto_retry_start", "auto_retry_end",
		"queue_update", "message_start", "message_end", "tool_execution_start",
		"tool_execution_update", "tool_execution_end", "extension_ui_request":
		if err := json.Unmarshal(raw, &ev); err != nil {
			return Event{}, false
		}
		// result 在工具事件与压缩事件里是两种类型，收起一个 json 字段就得分开解一遍。
		switch head.Type {
		case "tool_execution_end":
			var wrap struct {
				Result *ToolResult `json:"result"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil {
				ev.Result = wrap.Result
			}
		case "compaction_end":
			var wrap struct {
				Result *CompactionResult `json:"result"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil {
				ev.CompactResult = wrap.Result
			}
		}
		// 扩展 UI 的 message 是给人读的正文，消息事件的 message 是消息对象。
		if head.Type == "extension_ui_request" {
			var wrap struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil {
				ev.Body = wrap.Message
			}
		}
		// fire-and-forget 的字段不在 wrap 里，直接按 Event 的 tag 解：
		// notify（notifyType）、setStatus（statusKey/statusText）、setWidget（widgetLines）。
		if head.Type == "extension_ui_request" {
			var extra struct {
				NotifyType      string   `json:"notifyType"`
				StatusKey       string   `json:"statusKey"`
				StatusText      string   `json:"statusText"`
				WidgetLines     []string `json:"widgetLines"`
				WidgetPlacement string   `json:"widgetPlacement"`
			}
			if err := json.Unmarshal(raw, &extra); err == nil {
				ev.NotifyType = extra.NotifyType
				ev.StatusKey = extra.StatusKey
				ev.StatusText = extra.StatusText
				ev.WidgetLines = extra.WidgetLines
				ev.WidgetPlacement = extra.WidgetPlacement
			}
		}
		// message_end / message_start 的 usage 与 stopReason 在 message 对象里，不在顶层。
		if head.Type == "message_end" || head.Type == "message_start" {
			var wrap struct {
				Message struct {
					Usage      *Usage `json:"usage"`
					StopReason string `json:"stopReason"`
					Role       string `json:"role"`
				} `json:"message"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil {
				if ev.Usage == nil {
					ev.Usage = wrap.Message.Usage
				}
				ev.StopReason = wrap.Message.StopReason
				ev.Role = wrap.Message.Role
			}
		}
		return ev, true
	case "message_update":
		if err := json.Unmarshal(raw, &ev); err != nil {
			return Event{}, false
		}
		var wrap struct {
			Assistant assistantUpdate `json:"assistantMessageEvent"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return Event{}, false
		}
		ev.DeltaType = wrap.Assistant.Type
		if ev.Delta == "" {
			ev.Delta = wrap.Assistant.Delta
		}
		if ev.ToolCallID == "" {
			ev.ToolCallID = wrap.Assistant.ID
		}
		if ev.ToolName == "" {
			ev.ToolName = wrap.Assistant.ToolName
		}
		if ev.ToolCall == nil {
			ev.ToolCall = wrap.Assistant.ToolCall
		}
		return ev, true
	default:
		return Event{}, false
	}
}
