package api

import "WorkBaby/internal/domain"

// ListTools 返回全部工具（含启停状态），供设置页展示。
func (h *Handler) ListTools() ([]domain.ToolMeta, error) {
	return h.toolSvc.ListTools(h.ctx)
}

// SetToolEnabled 持久化工具启停状态。
func (h *Handler) SetToolEnabled(name string, enabled bool) error {
	return h.toolSvc.SetToolEnabled(h.ctx, name, enabled)
}
