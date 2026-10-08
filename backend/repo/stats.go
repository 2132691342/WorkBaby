package repo

import (
	"time"

	"WorkBaby/backend/domain"
)

// UsageStat 一次按天 / 按模型的聚合结果。
type UsageStat struct {
	Model     string
	Day       string
	Input     int
	Output    int
	Cached    int
	Total     int
	Calls     int
	LatencyMs int64
}

// DailyUsage 汇总 days 天内每天的 token 用量；没调用的天不返回，由服务层补零。
func (r *Repo) DailyUsage(since time.Time) ([]UsageStat, error) {
	var rows []struct {
		Day       string
		Input     int
		Output    int
		Cached    int
		Total     int
		Calls     int
		LatencyMs int64
	}
	err := r.db.Model(&domain.TokenUsageDO{}).
		Select("strftime('%Y-%m-%d', created_at / 1000, 'unixepoch', 'localtime') AS day,"+
			"SUM(input) AS input, SUM(output) AS output, SUM(total) AS total,"+
			"COALESCE(SUM(cached), 0) AS cached,"+
			"COUNT(*) AS calls, COALESCE(SUM(latency_ms), 0) AS latency_ms").
		Where("created_at >= ?", since.UnixMilli()).
		Group("day").Order("day ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]UsageStat, 0, len(rows))
	for _, v := range rows {
		out = append(out, UsageStat{
			Day: v.Day, Input: v.Input, Output: v.Output, Cached: v.Cached,
			Total: v.Total, Calls: v.Calls, LatencyMs: v.LatencyMs,
		})
	}
	return out, nil
}

// ModelUsage 汇总 days 天内按模型的 token 用量，按消耗从多到少。
func (r *Repo) ModelUsage(since time.Time, limit int) ([]UsageStat, error) {
	var rows []struct {
		Model  string
		Input  int
		Output int
		Cached int
		Total  int
		Calls  int
	}
	err := r.db.Model(&domain.TokenUsageDO{}).
		Select("model AS model, SUM(total) AS total, COUNT(*) AS calls,"+
			"SUM(input) AS input, SUM(output) AS output, COALESCE(SUM(cached), 0) AS cached").
		Where("created_at >= ?", since.UnixMilli()).
		Group("model").Order("total DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]UsageStat, 0, len(rows))
	for _, v := range rows {
		out = append(out, UsageStat{Model: v.Model, Total: v.Total, Calls: v.Calls,
			Input: v.Input, Output: v.Output, Cached: v.Cached})
	}
	return out, nil
}

// TopSessions 取用量最高的会话，用量取自 sessions 上的冗余计数。
func (r *Repo) TopSessions(limit int) ([]domain.StatsSessionVO, error) {
	var out []domain.StatsSessionVO
	err := r.db.Model(&domain.SessionDO{}).
		Select("id AS session_id, title AS title, total_tokens AS total").
		Where("total_tokens > 0").
		Order("total_tokens DESC").Limit(limit).Scan(&out).Error
	return out, err
}

// CountSessions 返回会话总数。
func (r *Repo) CountSessions() (int64, error) {
	var n int64
	err := r.db.Model(&domain.SessionDO{}).Count(&n).Error
	return n, err
}

// ContextStat 是区间内上下文占用的均值与峰值。
type ContextStat struct {
	Avg  int
	Peak int
}

// ContextUsage 汇总每次调用发出时的上下文占用。context 为 0 的历史行（旧数据）不参与。
func (r *Repo) ContextUsage(since time.Time) (ContextStat, error) {
	var rows []struct {
		Avg  int
		Peak int
	}
	err := r.db.Model(&domain.TokenUsageDO{}).
		Select("COALESCE(CAST(AVG(context) AS INTEGER), 0) AS avg, COALESCE(MAX(context), 0) AS peak").
		Where("created_at >= ? AND context > 0", since.UnixMilli()).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return ContextStat{}, err
	}
	return ContextStat{Avg: rows[0].Avg, Peak: rows[0].Peak}, nil
}
