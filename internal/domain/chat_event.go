package domain

// 聊天事件载荷契约（chat:*）：字段即前后端契约。
// 只定义「形状容易被写歪」的事件；流式增量等极简载荷仍用 map 直传（高频，零转换开销）。

// ChatDoneEvent run 终态载荷（chat:done）。正常收尾与失败收尾共用同一形状——
// 字段缺失一律零值，前端不必再为「同一事件两种形状」写防御式兜底。
type ChatDoneEvent struct {
	Status     string         `json:"status"`                // 完成状态：completed | failed
	Reason     string         `json:"reason"`                // 领域停止原因（MessageStopReason）
	StopReason string         `json:"stop_reason,omitempty"` // 上游归一化 stop 原因
	MessageID  string         `json:"message_id"`
	Usage      *ChatDoneUsage `json:"usage,omitempty"`
}

// ChatDoneUsage 终态用量（与 token_usages 明细同口径）。
type ChatDoneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_tokens"`
	CacheWrite   int `json:"cache_creation_tokens"`
	Total        int `json:"total_tokens"`
}
