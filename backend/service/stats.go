// 用量统计：全部数字来自 PI 的会话文件（每条助手消息带 usage 与模型名）。
// WorkBaby 不再单独落一张用量表——会话文件里已经有一份更权威的。
package service

import (
	"encoding/json"
	"sort"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
)

// StatsService 聚合用量并组织成仪表盘出参。
type StatsService struct {
	env *Env
}

// NewStatsService 构造统计服务。
func NewStatsService(env *Env) *StatsService { return &StatsService{env: env} }

// bucketPlan 把 span 归一成「粒度 + 起点 + 桶数 + 桶键」。
// 天粒度从本地零点起算，小时粒度按整点切：两种都要把缺的桶补成 0，趋势图才连续。
func bucketPlan(span string) (gran string, start time.Time, count int, keyOf func(time.Time) string) {
	now := time.Now()
	switch span {
	case "24h":
		s := now.Add(-24 * time.Hour).Truncate(time.Hour)
		return "hour", s, 24, func(t time.Time) string { return t.Format("2006-01-02 15:04") }
	case "today":
		s := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		hours := int(now.Sub(s)/time.Hour) + 1
		if hours > 24 {
			hours = 24
		}
		return "hour", s, hours, func(t time.Time) string { return t.Format("2006-01-02 15:04") }
	case "7d", "30d":
		days := 7
		if span == "30d" {
			days = 30
		}
		s := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
		return "day", s, days, func(t time.Time) string { return t.Format("2006-01-02") }
	default: // 14d，也是未识别 span 的兜底
		s := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -13)
		return "day", s, 14, func(t time.Time) string { return t.Format("2006-01-02") }
	}
}

// Compute 计算一段时间的用量。span：24h / today / 7d / 14d / 30d，空与未知按 14d。
func (s *StatsService) Compute(span string) (*domain.StatsRESP, error) {
	gran, start, count, keyOf := bucketPlan(span)
	if span != "24h" && span != "today" && span != "7d" && span != "30d" {
		span = "14d"
	}
	cutoff := start.UnixMilli()

	resp := &domain.StatsRESP{
		Span:        span,
		Granularity: gran,
		Daily:       []domain.StatsDailyVO{},
		Models:      []domain.StatsModelVO{},
		Sessions:    []domain.StatsSessionVO{},
	}
	metas, err := pi.ListSessions(s.env.PiEnv.SessionsDir())
	if err != nil {
		return nil, err
	}

	daily := map[string]*domain.StatsDailyVO{}
	models := map[string]*domain.StatsModelVO{}
	active := map[string]bool{}

	for _, meta := range metas {
		data, err := pi.LoadSession(meta.File)
		if err != nil {
			continue
		}
		sessionTotal := 0
		sessionCost := 0.0
		hasActivity := false
		for i := range data.Entries {
			entry := &data.Entries[i]
			if entry.Type != "message" {
				continue
			}
			var msg pi.SessionMessage
			if json.Unmarshal(entry.Message, &msg) != nil || msg.Usage == nil {
				continue
			}
			at := msg.Timestamp
			if at == 0 {
				at = pi.IsoMillis(entry.Timestamp)
			}
			if at < cutoff {
				continue
			}
			hasActivity = true
			usage := msg.Usage
			key := keyOf(time.UnixMilli(at))
			d := daily[key]
			if d == nil {
				d = &domain.StatsDailyVO{Date: key}
				daily[key] = d
			}
			d.Input += usage.Input
			d.Output += usage.Output
			d.Cached += usage.CacheRead
			d.CacheWrite += usage.CacheWrite
			d.Total += usage.TotalTokens
			d.Cost += usage.Cost.Total
			d.Calls++

			mkey := msg.Provider + "/" + msg.Model
			m := models[mkey]
			if m == nil {
				m = &domain.StatsModelVO{Provider: msg.Provider, Model: msg.Model}
				models[mkey] = m
			}
			m.Input += usage.Input
			m.Output += usage.Output
			m.Cached += usage.CacheRead
			m.CacheWrite += usage.CacheWrite
			m.Total += usage.TotalTokens
			m.Cost += usage.Cost.Total
			m.Calls++

			resp.Totals.Input += usage.Input
			resp.Totals.Output += usage.Output
			resp.Totals.Cached += usage.CacheRead
			resp.Totals.CacheWrite += usage.CacheWrite
			resp.Totals.Total += usage.TotalTokens
			resp.Totals.Cost += usage.Cost.Total
			resp.Totals.Calls++
			resp.Totals.Messages++
			sessionTotal += usage.TotalTokens
			sessionCost += usage.Cost.Total
		}
		if hasActivity {
			active[meta.ID] = true
			resp.Sessions = append(resp.Sessions, domain.StatsSessionVO{
				SessionID: meta.ID, Title: meta.Title(), Workspace: meta.Cwd,
				Total: sessionTotal, Cost: sessionCost, UpdatedAt: meta.UpdatedAt,
			})
		}
	}
	resp.Totals.Sessions = len(active)

	// 补齐区间里的每个桶：缺的画 0 而不是跳过，趋势图才连续。
	step := 24 * time.Hour
	if gran == "hour" {
		step = time.Hour
	}
	for i := 0; i < count; i++ {
		key := keyOf(start.Add(step * time.Duration(i)))
		if d, ok := daily[key]; ok {
			resp.Daily = append(resp.Daily, *d)
			continue
		}
		resp.Daily = append(resp.Daily, domain.StatsDailyVO{Date: key})
	}
	for _, m := range models {
		resp.Models = append(resp.Models, *m)
	}
	sort.Slice(resp.Models, func(i, j int) bool { return resp.Models[i].Total > resp.Models[j].Total })
	if len(resp.Models) > 6 {
		resp.Models = resp.Models[:6]
	}
	sort.Slice(resp.Sessions, func(i, j int) bool { return resp.Sessions[i].Total > resp.Sessions[j].Total })
	if len(resp.Sessions) > 6 {
		resp.Sessions = resp.Sessions[:6]
	}
	return resp, nil
}
