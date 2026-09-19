package domain

import "WorkBaby/internal/pkg"

// TaskState 后台任务状态（后台任务队列）。
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskCancelled TaskState = "cancelled"
)

// TaskDO 后台任务（内存态，不落库）。
//
// 任务生命周期短且结果已落成会话消息（可回溯），落表是重复真相源；
// 进程重启后任务消失是可接受的语义——与 ExecutionRegistry 一致。
type TaskDO struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Agent       string    `json:"agent"`
	Prompt      string    `json:"prompt"`
	State       TaskState `json:"state"`
	RunID       string    `json:"run_id"`
	Result      string    `json:"result"`
	Error       string    `json:"error"`
	CreatedAt   int64     `json:"created_at"`
	StartedAt   int64     `json:"started_at"`
	FinishedAt  int64     `json:"finished_at"`
	QueueLength int       `json:"queue_length"` // 提交时刻的排队长度（前端可提示等待）
}

// TaskRESP 出参（任务中心列表项）。
type TaskRESP struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Agent      string    `json:"agent"`
	Prompt     string    `json:"prompt"`
	State      TaskState `json:"state"`
	RunID      string    `json:"run_id"`
	Result     string    `json:"result"`
	Error      string    `json:"error"`
	CreatedAt  int64     `json:"created_at"`
	StartedAt  int64     `json:"started_at"`
	FinishedAt int64     `json:"finished_at"`
}

// TaskSubmitREQ 提交后台任务入参。
type TaskSubmitREQ struct {
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Prompt    string `json:"prompt"`
}

// TaskListRESP 任务列表出参。
type TaskListRESP struct {
	Items []TaskRESP `json:"items"`
	Total int        `json:"total"`
}

// 错误变量；段位 5009-5011。
var (
	ErrTaskQueueFull   = pkg.New(5009, "后台任务队列已满，请稍后再试", "")
	ErrTaskNotFound    = pkg.New(5010, "任务不存在", "")
	ErrTaskInvalid     = pkg.New(5011, "任务入参不合法", "")
	ErrTaskNotRunning  = pkg.New(5011, "任务已结束，无法取消", "")
	ErrTaskNoDelegator = pkg.New(5011, "后台任务服务未就绪", "")
)
