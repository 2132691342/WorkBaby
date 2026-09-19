package api

import "WorkBaby/internal/domain"

// GetVersion 返回前端需要的版本/phase/env 元信息（meta 域第一个上线路口）。
func (h *Handler) GetVersion() domain.VersionInfo {
	return h.metaSvc.GetVersion(h.ctx)
}

// GetContractVersion 返回前后端契约版本（前端启动时比对）。
func (h *Handler) GetContractVersion() int { return domain.ContractVersion }

// GetHealth 简化的健康检查（前端 readiness probe）。
// GetRuntimeStatus 返回内置运行时（node / python / powershell）状态，供设置页「关于」排障。
func (h *Handler) GetRuntimeStatus() domain.RuntimeStatusRESP {
	status := domain.RuntimeStatusRESP{Assets: []domain.RuntimeAssetStatus{}}
	if h.paths != nil {
		status.Home = h.paths.Home
	}
	if h.runtimeMgr == nil {
		status.Error = "runtime manager not ready"
		return status
	}
	return h.runtimeMgr.Status()
}

func (h *Handler) GetHealth() domain.HealthInfo {
	pc := 0
	if h.app.ProvRepo != nil {
		if rows, err := h.app.ProvRepo.List(h.ctx); err == nil {
			pc = len(rows)
		}
	}
	return h.metaSvc.GetHealth(h.ctx, pc, h.cfg != nil)
}
