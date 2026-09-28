package service

import (
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/repo"
)

// 仪表盘统计的天数缺省与上限。
const (
	defaultStatsDays = 14
	maxStatsDays     = 90
)

// Stats 汇总 token 用量，供仪表盘一次取全。
func (s *SettingsService) Stats(days int) (*domain.StatsRESP, error) {
	if days <= 0 {
		days = defaultStatsDays
	}
	if days > maxStatsDays {
		days = maxStatsDays
	}
	now := time.Now()
	since := now.AddDate(0, 0, -days+1).Truncate(24 * time.Hour)

	daily, err := s.env.Repo.DailyUsage(since)
	if err != nil {
		return nil, err
	}
	models, err := s.env.Repo.ModelUsage(since, 8)
	if err != nil {
		return nil, err
	}
	top, err := s.env.Repo.TopSessions(5)
	if err != nil {
		return nil, err
	}
	sessions, err := s.env.Repo.CountSessions()
	if err != nil {
		return nil, err
	}

	byDay := make(map[string]repo.UsageStat, len(daily))
	out := &domain.StatsRESP{Days: days, Daily: make([]domain.StatsDailyVO, 0, days)}
	for _, d := range daily {
		byDay[d.Day] = d
		out.Totals.Input += d.Input
		out.Totals.Output += d.Output
		out.Totals.Total += d.Total
		out.Totals.Calls += d.Calls
		out.Totals.LatencyMs += d.LatencyMs
	}
	// 补齐没调用的天，否则柱状图的 x 轴会随有无数据跳动。
	for i := 0; i < days; i++ {
		day := since.AddDate(0, 0, i).Format("2006-01-02")
		d := byDay[day]
		out.Daily = append(out.Daily, domain.StatsDailyVO{
			Date: day, Input: d.Input, Output: d.Output, Total: d.Total,
		})
	}
	if out.Totals.Calls > 0 {
		out.Totals.AvgLatency = out.Totals.LatencyMs / int64(out.Totals.Calls)
	}
	out.Totals.Sessions = int(sessions)

	out.Models = make([]domain.StatsModelVO, 0, len(models))
	for _, m := range models {
		if m.Model == "" {
			continue
		}
		out.Models = append(out.Models, domain.StatsModelVO{Model: m.Model, Total: m.Total, Calls: m.Calls})
	}
	if out.Sessions == nil {
		out.Sessions = []domain.StatsSessionVO{}
	}
	if len(top) > 0 {
		out.Sessions = top
	}
	return out, nil
}
