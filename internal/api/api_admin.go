package api

import (
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// GetAdminOverview 返回管理后台概览（版本/环境/计数聚合）。
func (h *Handler) GetAdminOverview() (*domain.AdminOverviewRESP, error) {
	version := h.metaSvc.GetVersion(h.ctx)
	home := ""
	if h.paths != nil {
		home = h.paths.Home
	}
	return h.dashSvc.Overview(h.ctx, version.Version, version.Phase, home, h.cfg != nil)
}

// CleanupMisreportedTokenUsage 清理上游误报的缓存 token：把 cache_read_tokens > input_tokens
// 的存量行 cache_read / cache_write 清零（新数据已在解析时 clamp）。不可逆、幂等。
func (h *Handler) CleanupMisreportedTokenUsage() (map[string]any, error) {
	if h.app.UsageRepo == nil {
		return nil, pkg.New(2012, "token usage repo not ready", "")
	}
	n, err := h.app.UsageRepo.CleanupMisreported(h.ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"cleaned": n}, nil
}
