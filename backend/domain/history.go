// 通用会话历史的出参：扫描结果与会话详情（specs/12）。
package domain

// HistoryEntryVO 一段扫到的外部会话；列表与详情同构，详情才带 messages。
type HistoryEntryVO struct {
	ID           string             `json:"id"`
	Source       string             `json:"source"`
	Project      string             `json:"project"`
	Title        string             `json:"title"`
	Path         string             `json:"path"`
	StartedAt    int64              `json:"started_at"`
	MessageCount int                `json:"message_count"`
	Messages     []HistoryMessageVO `json:"messages,omitempty"`
}

// HistoryMessageVO 统一消息形态：role 归一为 user / assistant / tool。
type HistoryMessageVO struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

// HistoryScanRESP 一次扫描的结果。
// Entries 必须为空数组而不是 nil：契约说是数组就永远发 []，前端不会读到 null。
type HistoryScanRESP struct {
	Entries  []HistoryEntryVO `json:"entries"`
	Skipped  int              `json:"skipped"`
	TimedOut bool             `json:"timed_out"`
}
