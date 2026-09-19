package api

import (
	"WorkBaby/internal/domain"
)

// SubmitTask 提交后台任务（POST /tasks）：立即返回 pending 任务，执行在后台进行。
func (h *Handler) SubmitTask(req domain.ChatTaskREQ) (*domain.ChatTaskDO, error) {
	return h.taskSvc.Submit(h.ctx, req.SessionID, req.Agent, req.Prompt)
}

// ListTasks 任务列表（GET /tasks?limit=50，最新在前）。
func (h *Handler) ListTasks(limit int) (*domain.ChatTaskListRESP, error) {
	return h.taskSvc.List(h.ctx, limit)
}

// CancelTask 取消任务（POST /tasks/:id/cancel）；幂等：已终态无事发生。
func (h *Handler) CancelTask(id string) error {
	return h.taskSvc.Cancel(h.ctx, id)
}
