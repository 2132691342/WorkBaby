// 运行观测聚合根：一次 run 的账、上下文快照与检查点（specs/23）。
package domain

import (
	"WorkBaby/backend/pkg"
)

var (
	ErrRunNotFound        = pkg.New("9602", "这次运行不存在", "回列表刷新")
	ErrCheckpointNotFound = pkg.New("9603", "这个检查点不存在", "回列表刷新")
)

// 运行状态：与内核 stop_reason 分开——前者是「这一轮整体的结局」，后者是「模型为什么停下」。
const (
	RunRunning = "running"
	RunDone    = "done"
	RunFailed  = "failed"
	RunAborted = "aborted" // 用户停止
)

// RunDO 一次 run 一行。正文不在这里：条目链是唯一真相源，这张表只记账与位点。
type RunDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	SessionID  string `gorm:"index;size:64" json:"session_id"`
	ProviderID string `gorm:"size:64" json:"provider_id"`
	Model      string `gorm:"size:128" json:"model"`
	Status     string `gorm:"size:16" json:"status"`
	// StopReason 是 PI 的收尾原因：stop / length / toolUse / aborted / error。
	StopReason string `gorm:"size:16" json:"stop_reason"`
	Turns      int    `json:"turns"`
	Tokens     int    `json:"tokens"`
	Input      int    `json:"input"`
	Output     int    `json:"output"`
	Cached     int    `json:"cached"`
	// Cost 是上游按模型单价算出的美元成本；模型未声明价格时为 0。
	Cost float64 `json:"cost"`
	// Context 是收尾时上下文占用，MaxTokens 是模型声明的输出上限。
	Context   int `json:"context"`
	MaxTokens int `json:"max_tokens"`
	// Compressed 是这次 run 里压缩发生过的次数。
	Compressed int `json:"compressed"`
	// TimelineJSON 存这次 run 的工具动作时间线（由旁听器采）。
	// 正文在 PI 的 JSONL 里，这里只存「干了哪些动作、成没成、花了多久」。
	TimelineJSON string `gorm:"type:text" json:"-"`
	StartedAt    int64  `gorm:"index" json:"started_at"`
	FinishedAt   int64  `json:"finished_at"` // 0 = 还在跑
}

// TableName 显式指定表名。
func (RunDO) TableName() string { return "runs" }

// ContextSnapshotDO 轮末的上下文快照：回答「第几轮时窗口里有多少」。
type ContextSnapshotDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	RunID      string `gorm:"index;size:64" json:"run_id"`
	SessionID  string `gorm:"index;size:64" json:"session_id"`
	Turn       int    `json:"turn"`
	Used       int    `json:"used"`
	Window     int    `json:"window"`
	Compressed int    `json:"compressed"`
	CreatedAt  int64  `gorm:"autoCreateTime:milli;index" json:"created_at"`
}

// TableName 显式指定表名。
func (ContextSnapshotDO) TableName() string { return "context_snapshots" }

// CheckpointDO 一轮开始前的位点：LeafEntryID 是那一刻的会话叶子。
// 回到它就是「把这个会话的当前位点移回去」，不删任何条目。
type CheckpointDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	SessionID string `gorm:"index;size:64" json:"session_id"`
	RunID     string `gorm:"size:64" json:"run_id"`
	// PiEntryID 是那一轮起点的 PI 条目 id（分叉时传给 PI 的 fork）。
	PiEntryID string `gorm:"size:64" json:"pi_entry_id"`
	Turn      int    `json:"turn"`
	// Prompt 是那一轮开始时用户说的话（截断），列表里靠它认出「从哪一轮重开」。
	Prompt    string `gorm:"type:text" json:"prompt"`
	CreatedAt int64  `gorm:"autoCreateTime:milli;index" json:"created_at"`
}

// TableName 显式指定表名。
func (CheckpointDO) TableName() string { return "checkpoints" }

// CheckpointPromptLimit 检查点里那句需求保留的字符数。
const CheckpointPromptLimit = 200

// RunVO 运行记录出参。
type RunVO struct {
	ID           string `json:"id"`
	SessionID    string `json:"session_id"`
	SessionTitle string `json:"session_title"`
	ProviderID   string `json:"provider_id"`
	Model        string `json:"model"`
	Status       string `json:"status"`      // Run* 取值
	StopReason   string `json:"stop_reason"` // 内核收尾原因
	Turns        int    `json:"turns"`
	Tokens       int    `json:"tokens"`
	Input        int    `json:"input"`
	Output       int    `json:"output"`
	Cached       int    `json:"cached"`
	Context      int    `json:"context"`
	MaxTokens    int    `json:"max_tokens"`
	Compressed   int    `json:"compressed"`
	StartedAt    int64  `json:"started_at"`
	FinishedAt   int64  `json:"finished_at"` // 0 = 还在跑
	DurationMs   int64  `json:"duration_ms"`
	// CanResume 表示这一轮异常收尾，界面上给「继续」入口。
	CanResume bool `json:"can_resume"`
}

// RunDetailVO 一次运行的详情：记录 + 时间线 + 快照 + 可回退的检查点。
type RunDetailVO struct {
	RunVO
	Timeline    *RunTimeline        `json:"timeline,omitempty"`
	Snapshots   []ContextSnapshotDO `json:"snapshots"`
	Checkpoints []CheckpointVO      `json:"checkpoints"`
}

// CheckpointVO 检查点出参。
type CheckpointVO struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
	// PiEntryID 是那一轮起点的 PI 条目 id：分叉时用它。
	PiEntryID string `json:"pi_entry_id"`
	Turn      int    `json:"turn"`
	Prompt    string `json:"prompt"`
	CreatedAt int64  `json:"created_at"`
}

// RestoreCheckpointREQ 回到 / 分叉自某个检查点：会话 id 必需（检查点不全局唯一）。
type RestoreCheckpointREQ struct {
	SessionID string `json:"session_id"`
}

// ArchiveVO 一次按保留期归档的结果。
// 归档 = 丢逐字对话链、留结论与时间线：运行记录、用量账、评估指标一条不动。
type ArchiveVO struct {
	RetentionDays int `json:"retention_days"` // 本次生效的保留期；0 = 永久保留（不做任何归档）
	// Sessions 是被清掉消息链的隐藏会话数。
	Sessions int `json:"sessions"`
	// DetachedRuns 是因此失去「原始对话」入口、但结论与时间线仍在的运行记录数。
	DetachedRuns int `json:"detached_runs"`
}

// runPayload 是时间线投影需要的那几个字段，与面向前端的消息载荷解耦。
type runPayload struct {
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ToolError  bool   `json:"tool_error"`
	ToolCalls  []struct {
		ID string `json:"id"`
	} `json:"tool_calls"`
}
