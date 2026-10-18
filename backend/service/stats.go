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

// Compute 计算近 days 天的用量。
func (s *StatsService) Compute(days int) (*domain.StatsRESP, error) {
	if days <= 0 {
		days = 14
	}
	if days > 90 {
		days = 90
	}
	cutoff := time.Now().AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour).UnixMilli()

	resp := &domain.StatsRESP{
		Days:     days,
		Daily:    []domain.StatsDailyVO{},
		Models:   []domain.StatsModelVO{},
		Sessions: []domain.StatsSessionVO{},
	}
	metas, err := pi.ListSessions(s.env.PiEnv.SessionsDir())
	if err != nil {
		return nil, err
	}

	daily := map[string]*domain.StatsDailyVO{}
	models := map[string]*domain.StatsModelVO{}
	totalCalls := 0
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
			day := time.UnixMilli(at).Format("2006-01-02")
			d := daily[day]
			if d == nil {
				d = &domain.StatsDailyVO{Date: day}
				daily[day] = d
			}
			d.Input += usage.Input
			d.Output += usage.Output
			d.Cached += usage.CacheRead
			d.Total += usage.TotalTokens
			d.Cost += usage.Cost.Total
			d.Calls++

			key := msg.Provider + "/" + msg.Model
			m := models[key]
			if m == nil {
				m = &domain.StatsModelVO{Provider: msg.Provider, Model: msg.Model}
				models[key] = m
			}
			m.Input += usage.Input
			m.Output += usage.Output
			m.Total += usage.TotalTokens
			m.Cost += usage.Cost.Total
			m.Calls++

			resp.Totals.Input += usage.Input
			resp.Totals.Output += usage.Output
			resp.Totals.Cached += usage.CacheRead
			resp.Totals.Total += usage.TotalTokens
			resp.Totals.Cost += usage.Cost.Total
			resp.Totals.Calls++
			sessionTotal += usage.TotalTokens
			sessionCost += usage.Cost.Total
			totalCalls++
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

	// 补齐区间里的每一天：缺的那天要画 0 而不是跳过，趋势图才连续。
	start := time.UnixMilli(cutoff)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		if d, ok := daily[day]; ok {
			resp.Daily = append(resp.Daily, *d)
			continue
		}
		resp.Daily = append(resp.Daily, domain.StatsDailyVO{Date: day})
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
	_ = totalCalls
	return resp, nil
}
