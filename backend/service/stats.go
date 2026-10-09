package service

import (
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/repo"
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
	// 起点取本地零点：SQL 侧按 localtime 分组，用 Truncate 截 UTC 网格会让
	// UTC+8 的第一天从本地 08:00 起算，柱高与分组对不上。
	y, m, d := now.Date()
	since := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -days+1)

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
	ctxStat, err := s.env.Repo.ContextUsage(since)
	if err != nil {
		return nil, err
	}

	byDay := make(map[string]repo.UsageStat, len(daily))
	out := &domain.StatsRESP{Days: days, Daily: make([]domain.StatsDailyVO, 0, days)}
	for _, d := range daily {
		byDay[d.Day] = d
		out.Totals.Input += d.Input
		out.Totals.Output += d.Output
		out.Totals.Cached += d.Cached
		out.Totals.Total += d.Total
		out.Totals.Calls += d.Calls
		out.Totals.LatencyMs += d.LatencyMs
	}
	// 补齐没调用的天，否则柱状图的 x 轴会随有无数据跳动。
	for i := 0; i < days; i++ {
		day := since.AddDate(0, 0, i).Format("2006-01-02")
		d := byDay[day]
		out.Daily = append(out.Daily, domain.StatsDailyVO{
			Date: day, Input: d.Input, Output: d.Output, Cached: d.Cached, Total: d.Total,
		})
	}
	if out.Totals.Calls > 0 {
		out.Totals.AvgLatency = out.Totals.LatencyMs / int64(out.Totals.Calls)
	}
	// 命中率按总输入算，不按天平均：某天没命中会把整段的均值拉低，读不出真实水平。
	if out.Totals.Input > 0 {
		rate := float64(out.Totals.Cached) / float64(out.Totals.Input)
		// 归一前落库的行按「未命中部分」记输入，命中量可能大于输入量。命中率越过
		// 100% 只会让用户以为统计坏了，读侧压回 0~1（新行在 llm.Usage.Normalize 已归一）。
		if rate > 1 {
			rate = 1
		}
		out.Totals.CacheHitRate = rate
	}
	out.Totals.AvgContext = ctxStat.Avg
	out.Totals.PeakContext = ctxStat.Peak
	out.Totals.Sessions = int(sessions)

	out.Models = make([]domain.StatsModelVO, 0, len(models))
	for _, m := range models {
		if m.Model == "" {
			continue
		}
		out.Models = append(out.Models, domain.StatsModelVO{
			Model: m.Model, Total: m.Total, Calls: m.Calls,
			Input: m.Input, Output: m.Output, Cached: m.Cached,
		})
	}
	if out.Sessions == nil {
		out.Sessions = []domain.StatsSessionVO{}
	}
	if len(top) > 0 {
		out.Sessions = top
	}
	return out, nil
}
