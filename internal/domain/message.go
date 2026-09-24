package domain

import (
	"encoding/json"

	"WorkBaby/internal/pkg"
)

// MessageRole 角色枚举。
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSystem    MessageRole = "system"
)

// MessageStatus 消息状态（落库后流转）。
type MessageStatus string

const (
	MessageStatusPending   MessageStatus = "pending"   // 已排队，未落库 assistant 占位
	MessageStatusStreaming MessageStatus = "streaming" // 流式中
	MessageStatusCompleted MessageStatus = "completed" // 终止：成功
	MessageStatusFailed    MessageStatus = "failed"    // 终止：失败
	MessageStatusCancelled MessageStatus = "cancelled" // 终止：用户取消
	MessageStatusArchived  MessageStatus = "archived"  // 已归档：/compact 剔出上下文（UI仍可见）
)

// MessageContextScope 消息是否进入 LLM 上下文。UI 专属消息（界面提示、genui 包装、
// 系统通知）可完整落库并回放但不参与装配；零值按 MessageScopeLLM 处理。
type MessageContextScope string

const (
	MessageScopeLLM MessageContextScope = "llm" // 进入 LLM 上下文（默认）
	MessageScopeUI  MessageContextScope = "ui"  // 仅界面可见
)

// MessageStopReason 流式停止原因。
type MessageStopReason string

const (
	StopReasonCompleted MessageStopReason = "completed"
	StopReasonCancelled MessageStopReason = "cancelled"
	StopReasonMaxTurns  MessageStopReason = "max_turns"
	StopReasonToolErr   MessageStopReason = "tool_error_limit"
	StopReasonToken     MessageStopReason = "token_budget"
	StopReasonStagnant  MessageStopReason = "stagnation"
	StopReasonError     MessageStopReason = "error"
)

// Severity 终止原因的展示级别（前端据此差异化收尾横幅）。
type Severity string

const (
	SeverityInfo    Severity = "info"    // 正常收尾
	SeverityWarning Severity = "warning" // 触发护栏，可恢复
	SeverityError   Severity = "error"   // 失败
)

// StopReasonSeverity 终止原因 → 展示级别（终态差异化；前后端契约一致）。
func StopReasonSeverity(r MessageStopReason) Severity {
	switch r {
	case StopReasonCompleted:
		return SeverityInfo
	case StopReasonCancelled, StopReasonMaxTurns, StopReasonToolErr, StopReasonToken, StopReasonStagnant:
		return SeverityWarning
	default: // error 与未知值
		return SeverityError
	}
}

// MapHarnessReason 内核终止原因 → MessageStopReason（service 落库与 chat:done 统一口径）。
// harness 侧 end_turn/max_tokens/budget_exceeded/stagnation 与领域枚举不同名，逐值映射。
func MapHarnessReason(reason string) MessageStopReason {
	switch reason {
	case "end_turn":
		return StopReasonCompleted
	case "cancelled":
		return StopReasonCancelled
	case "max_turns":
		return StopReasonMaxTurns
	case "stagnation":
		return StopReasonStagnant
	case "max_tokens", "budget_exceeded":
		return StopReasonToken
	default:
		return StopReasonError
	}
}

// AttachmentKind 附件类别；只有 image 会随消息发给多模态模型。
type AttachmentKind string

const (
	AttachmentImage AttachmentKind = "image"
	AttachmentFile  AttachmentKind = "file"
)

// MessageAttachment 消息附件（落库为 JSON；图片可转成 LLM 多模态 part）。
type MessageAttachment struct {
	ID   string         `json:"id"` // 受管文件 ID
	Name string         `json:"name"`
	MIME string         `json:"mime"`
	Size int64          `json:"size"`
	Kind AttachmentKind `json:"kind"`
	URL  string         `json:"url"` // 前端直读地址（/files/files/{id}）
}

// MessageDO 单条消息持久化实体。
type MessageDO struct {
	ID         string        `gorm:"primaryKey;size:64"  json:"id"`
	SessionID  string        `gorm:"size:64;index:idx_msg_session_seq" json:"session_id"`
	Seq        int64         `gorm:"index:idx_msg_session_seq"          json:"seq"`
	RunID      string        `gorm:"size:64;index"        json:"run_id"`
	Role       MessageRole   `gorm:"size:16"              json:"role"`
	Content    string        `gorm:"type:text"            json:"content"`
	Thinking   string        `gorm:"type:text"            json:"thinking"`
	ToolCallID string        `gorm:"size:64"              json:"tool_call_id"`    // role=tool 时关联的调用 ID
	ToolCalls  string        `gorm:"type:text"            json:"tool_calls_json"` // role=assistant 时序列化的工具调用
	Status     MessageStatus `gorm:"size:16"              json:"status"`
	// ContextScope 消息分层：ui = 仅界面可见（不进 LLM 上下文）；空/llm = 正常参与。
	ContextScope MessageContextScope `gorm:"size:16"            json:"context_scope"`
	StopReason   MessageStopReason   `gorm:"size:32"          json:"stop_reason"`
	Model        string              `gorm:"size:128"             json:"model"`
	InputTokens  int                 `gorm:"default:0"            json:"input_tokens"`
	OutputTokens int                 `gorm:"default:0"            json:"output_tokens"`
	CacheRead    int                 `gorm:"default:0"            json:"cache_read_tokens"`
	TotalTokens  int                 `gorm:"default:0"            json:"total_tokens"`
	LatencyMs    int                 `gorm:"default:0"            json:"latency_ms"`
	Cost         string              `gorm:"size:32"              json:"cost"`
	// AttachmentsJSON 附件清单（MessageAttachment 数组序列化；正文与附件分离，正文保持纯文本）。
	AttachmentsJSON string `gorm:"type:text"            json:"-"`
	CreatedAt       int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt       int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (MessageDO) TableName() string { return "chat_messages" }

// Attachments 解析附件 JSON；非法值按空处理（不阻断消息渲染）。
func (m MessageDO) Attachments() []MessageAttachment {
	if m.AttachmentsJSON == "" {
		return nil
	}
	var out []MessageAttachment
	if err := json.Unmarshal([]byte(m.AttachmentsJSON), &out); err != nil {
		return nil
	}
	return out
}

// SetAttachments 写入附件 JSON。
func (m *MessageDO) SetAttachments(as []MessageAttachment) {
	if len(as) == 0 {
		m.AttachmentsJSON = ""
		return
	}
	b, err := json.Marshal(as)
	if err != nil {
		m.AttachmentsJSON = ""
		return
	}
	m.AttachmentsJSON = string(b)
}

// MessageREQ 创建消息（前端发消息用）。
type MessageREQ struct {
	Content        string   `json:"content"`
	ProviderID     string   `json:"provider_id"`
	Model          string   `json:"model"`
	Temperature    *float64 `json:"temperature,omitempty"`
	ThinkingEffort *string  `json:"thinking_effort,omitempty"`
}

// MessageRESP 出参。
type MessageRESP struct {
	ID         string        `json:"id"`
	SessionID  string        `json:"session_id"`
	RunID      string        `json:"run_id"`
	Role       MessageRole   `json:"role"`
	Content    string        `json:"content"`
	Thinking   string        `json:"thinking"`
	ToolCallID string        `json:"tool_call_id"`
	ToolCalls  string        `json:"tool_calls_json"`
	Status     MessageStatus `json:"status"`
	// ContextScope 消息分层：ui = 仅界面可见（不进 LLM 上下文）。
	ContextScope MessageContextScope `json:"context_scope"`
	StopReason   MessageStopReason   `json:"stop_reason"`
	Model        string              `json:"model"`
	InputTokens  int                 `json:"input_tokens"`
	OutputTokens int                 `json:"output_tokens"`
	CacheRead    int                 `json:"cache_read_tokens"`
	TotalTokens  int                 `json:"total_tokens"`
	LatencyMs    int                 `json:"latency_ms"`
	Cost         string              `json:"cost"`
	CreatedAt    int64               `json:"created_at"`
	UpdatedAt    int64               `json:"updated_at"`
	Blocks       []MessageBlockRESP  `json:"blocks,omitempty"`      // assistant 消息的持久化过程块（工具调用/结果/产物）
	Attachments  []MessageAttachment `json:"attachments,omitempty"` // user 消息的附件（图片/文件）
}

// MessageListRESP 分页。
type MessageListRESP struct {
	Items   []MessageRESP `json:"items"`
	Total   int           `json:"total"`
	NextSeq int64         `json:"next_seq"` // 增量查询游标（<0 表示已到末尾）
}

// TruncateMessagesREQ 从指定消息起（含）截断的入参（「从此处重新生成」）。
type TruncateMessagesREQ struct {
	MessageID string `json:"message_id"`
}

// DecideApprovalREQ 审批决策回填入参。Scope 决定放行有效期：once（默认）= 只放行这一次，
// session = 本会话内同命令免审（不可逆风险忽略该字段，由 service 侧强制）。
type DecideApprovalREQ struct {
	Approved bool   `json:"approved"`
	Scope    string `json:"scope,omitempty"`
}

// 包级错误变量；段位 5000。
var (
	ErrMessageNotFound = pkg.New(5004, "message not found", "")
	ErrMessageInvalid  = pkg.New(5005, "message invalid (empty content)", "")
	// 5006 归 harness 上下文压缩，5007 归 Resume；消息-会话归属校验顺延到 5008。
	ErrMessageNotInSession = pkg.New(5008, "message does not belong to session", "")
)
