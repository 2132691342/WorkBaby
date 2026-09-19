package domain

// ChatTaskDO 后台任务：持久化的独立 Agent run（不进聊天消息流）。
//
// 与委派（delegate_task）的分工：委派是父 run 内的同步子调用；后台任务是用户
// 显式提交的异步作业，有独立生命周期、跨重启可见、可取消与查看结果。
type ChatTaskDO struct {
	ID        string `gorm:"primaryKey;size:64"             json:"id"`
	SessionID string `gorm:"size:64;index:idx_task_session" json:"session_id"` // 承载工作区与 Provider 的宿主会话
	Agent     string `gorm:"size:32"                        json:"agent"`      // 执行 Agent 名（default / explore / 自定义）
	Prompt    string `gorm:"type:text"                      json:"prompt"`     // 自包含任务描述
	State     string `gorm:"size:16;index:idx_task_state"   json:"state"`      // pending/running/completed/failed/cancelled
	RunID     string `gorm:"size:64"                        json:"run_id"`     // 实际执行 run（执行期写入）
	Result    string `gorm:"type:text"                      json:"result"`     // 完成产物（Agent 最终答复摘要）
	Error     string `gorm:"type:text"                      json:"error"`      // 失败原因
	CreatedAt int64  `gorm:"autoCreateTime:milli"           json:"created_at"`
	StartedAt int64  `gorm:"default:0"                      json:"started_at"`
	FinishedAt int64 `gorm:"default:0"                      json:"finished_at"`
}

// TableName 固定表名。
func (ChatTaskDO) TableName() string { return "chat_tasks" }

// 任务状态机：pending → running → completed/failed/cancelled。
// pending 阶段在队列里等待 worker；cancelled 可发生在 pending 与 running 两态。
const (
	TaskStatePending   = "pending"
	TaskStateRunning   = "running"
	TaskStateCompleted = "completed"
	TaskStateFailed    = "failed"
	TaskStateCancelled = "cancelled"
)

// TaskTerminal 判断是否终态。
func TaskTerminal(state string) bool {
	return state == TaskStateCompleted || state == TaskStateFailed || state == TaskStateCancelled
}

// ChatTaskREQ 提交后台任务入参。
type ChatTaskREQ struct {
	SessionID string `json:"session_id"` // 宿主会话（工作区 + Provider 来源）
	Agent     string `json:"agent"`      // 空 = default
	Prompt    string `json:"prompt"`     // 必填：自包含任务描述
}

// ChatTaskListRESP 任务列表出参。
type ChatTaskListRESP struct {
	Items []ChatTaskDO `json:"items"`
	Total int          `json:"total"`
}
