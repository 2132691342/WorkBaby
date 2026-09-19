package api

import "WorkBaby/internal/domain"

// ListSessionChanges 会话文件变更清单（最新在前；前端变更面板）。
func (h *Handler) ListSessionChanges(sessionID string, limit int) (domain.FileChangeListRESP, error) {
	return h.changeSvc.List(h.ctx, sessionID, limit)
}

// GetSessionChange 单条变更详情（diff 正文 + 变更前内容）。
func (h *Handler) GetSessionChange(id string) (domain.FileChangeDetailRESP, error) {
	return h.changeSvc.Detail(h.ctx, id)
}

// RollbackSessionChange 回滚到变更前内容（新建的文件删除，修改的文件写回快照）。
func (h *Handler) RollbackSessionChange(id string) error {
	return h.changeSvc.Rollback(h.ctx, id)
}
