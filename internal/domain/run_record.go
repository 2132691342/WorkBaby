package domain

// RunRecordDO 单次 Agent run 的索引记录。
//
// 事件明细仍走 JSONL（{home}/runs/{runID}.jsonl，由 event.RunEventLog 的 file sink 落盘，
// 回放走 GET /api/v1/chat/runs/:id/events）；本表只存「能列表 / 能筛选 / 能跳转回放」的元数据。
type RunRecordDO struct {
	RunID        string `gorm:"primaryKey;size:64"            json:"run_id"`
	SessionID    string `gorm:"size:64;index:idx_run_session"  json:"session_id"`
	Model        string `gorm:"size:128"                       json:"model"`
	Status       string `gorm:"size:24"                        json:"status"` // running | done | error
	Reason       string `gorm:"size:32"                        json:"reason"` // end_turn | max_turns | error | cancelled ...
	Turns        int    `json:"turns"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	CacheRead    int    `json:"cache_read_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	// 分段耗时归因（毫秒）：等模型 / 跑工具 / 压上下文三段互斥且近似穷举，
	// 剩余差额是本机调度与落库开销（低到不值得单独归因）。
	LLMMs      int64 `json:"llm_ms"`
	ToolsMs    int64 `json:"tools_ms"`
	CompressMs int64 `json:"compress_ms"`
	StartedAt  int64 `gorm:"index:idx_run_session"          json:"started_at"`
	EndedAt    int64 `json:"ended_at"`
}

// TableName 固定表名。
func (RunRecordDO) TableName() string { return "run_records" }

// run 状态枚举。
const (
	RunStatusRunning = "running"
	RunStatusDone    = "done"
	RunStatusError   = "error"
)

// RunRecordRESP 运行历史列表出参。
type RunRecordRESP struct {
	RunID        string `json:"run_id"`
	SessionID    string `json:"session_id"`
	Model        string `json:"model"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	Turns        int    `json:"turns"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	CacheRead    int    `json:"cache_read_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	LLMMs        int64  `json:"llm_ms"`
	ToolsMs      int64  `json:"tools_ms"`
	CompressMs   int64  `json:"compress_ms"`
	StartedAt    int64  `json:"started_at"`
	EndedAt      int64  `json:"ended_at"`
}

// RunRecordListREQ 运行历史列表入参。
type RunRecordListREQ struct {
	SessionID string `form:"session_id" json:"session_id"` // 空 = 全部会话
	Limit     int    `form:"limit" json:"limit"`
	Offset    int    `form:"offset" json:"offset"`
}

// RunRecordListRESP 运行历史分页出参。
type RunRecordListRESP struct {
	Items []RunRecordRESP `json:"items"`
	Total int64           `json:"total"`
}
