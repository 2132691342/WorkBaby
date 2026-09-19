package api

import "WorkBaby/internal/domain"

// ListSessions 当前用户的会话列表（分页）。
func (h *Handler) ListSessions(page, pageSize int) (*domain.SessionListRESP, error) {
	return h.chatSvc.ListSessions(h.ctx, page, pageSize)
}

// CreateSession 新建会话。
func (h *Handler) CreateSession(req domain.ChatSessionREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.CreateSession(h.ctx, &req)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// GetSession 单条会话。
func (h *Handler) GetSession(id string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.GetSession(h.ctx, id)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// RenameSession 重命名。
func (h *Handler) RenameSession(id string, name string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.RenameSession(h.ctx, id, name)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// EffectiveParams 当前会话生效参数快照（输入框与设置页共用）。
func (h *Handler) EffectiveParams(id string) (*domain.EffectiveParamsRESP, error) {
	return h.chatSvc.EffectiveParams(h.ctx, id)
}

// SetSessionPermission 切换会话工具权限模式。
func (h *Handler) SetSessionPermission(id string, req domain.ChatSessionPermissionREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionPermission(h.ctx, id, req.Mode)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionPinned 置顶/取消置顶。
func (h *Handler) SetSessionPinned(id string, req domain.SessionPinREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionPinned(h.ctx, id, req.Pinned)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionArchived 归档/取消归档。
func (h *Handler) SetSessionArchived(id string, req domain.SessionArchiveREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionArchived(h.ctx, id, req.Archived)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionModel 切换会话使用的 Provider/模型（参数展示与后续 run 跟随所选模型）。
func (h *Handler) SetSessionModel(id string, req domain.ChatSessionModelREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.UpdateSessionModel(h.ctx, id, &req)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// DeleteSession 软删单条。
func (h *Handler) DeleteSession(id string) error {
	return h.chatSvc.DeleteSession(h.ctx, id)
}

// DeleteSessions 批量软删（OK / Failed 列表）。
func (h *Handler) DeleteSessions(ids []string) (domain.BatchDeleteResult, error) {
	ok, failed, err := h.chatSvc.DeleteSessions(h.ctx, ids)
	if err != nil {
		return domain.BatchDeleteResult{}, err
	}
	return domain.BatchDeleteResult{Ok: ok, Failed: failed}, nil
}

// UpdateSessionWorkspace 绑定/解绑会话外部工作目录（绝对路径；空 = 回默认工作区）。
func (h *Handler) UpdateSessionWorkspace(id string, workspacePath string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.UpdateWorkspace(h.ctx, id, workspacePath)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}
