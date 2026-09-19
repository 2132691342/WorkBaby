package api

import "WorkBaby/internal/domain"

// GetDashboardStats 返回仪表盘统计快照。
func (h *Handler) GetDashboardStats() (*domain.DashboardStatsRESP, error) {
	return h.dashSvc.Stats(h.ctx)
}

// GetDashboardTrend 返回最近 N 天趋势（range 校验在 service/repo 兜底）。
func (h *Handler) GetDashboardTrend(days int) (*domain.DashboardTrendRESP, error) {
	return h.dashSvc.Trend(h.ctx, days)
}

// GetTokenTrend 返回 token 消耗折线图（scope 决定粒度：today→24 小时，其余→按天）。
func (h *Handler) GetTokenTrend(req domain.TokenTrendREQ) (*domain.TokenTrendRESP, error) {
	return h.dashSvc.TokenTrend(h.ctx, req)
}
