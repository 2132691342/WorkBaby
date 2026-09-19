package api

import (
	"WorkBaby/internal/domain"
)

// ListAgentProfiles 返回全部自定义子智能体（含 disabled），供设置页展示。
func (h *Handler) ListAgentProfiles() ([]domain.AgentProfileRESP, error) {
	return h.agentSvc.List(h.ctx)
}

// UpsertAgentProfile 创建/更新自定义子智能体（按 name upsert）。
func (h *Handler) UpsertAgentProfile(req domain.AgentProfileREQ) (domain.AgentProfileRESP, error) {
	p, err := h.agentSvc.Upsert(h.ctx, &req)
	if err != nil {
		return domain.AgentProfileRESP{}, err
	}
	return *p, nil
}

// SetAgentProfileEnabled 启停自定义子智能体。
func (h *Handler) SetAgentProfileEnabled(name string, enabled bool) error {
	return h.agentSvc.SetEnabled(h.ctx, name, enabled)
}

// DeleteAgentProfile 删除自定义子智能体。
func (h *Handler) DeleteAgentProfile(name string) error {
	return h.agentSvc.Delete(h.ctx, name)
}
