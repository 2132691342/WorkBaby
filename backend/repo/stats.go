package repo

import (
	"time"

	"WorkBaby/backend/domain"
)

// sumInputSQL 按「Input 含缓存」的口径汇总输入量。老数据只记了未命中缓存的部分
// （缓存量因此大于输入量）：读侧按同一条规则补回总量，否则仪表盘输入量偏小、
// 命中率越过 100%（`llm.Usage.Normalize` 管新行，这里管存量）。
const sumInputSQL = "SUM(CASE WHEN COALESCE(cached, 0) > input THEN input + cached ELSE input END)"

// DailyUsage 汇总 days 天内每天的 token 用量；没调用的天不返回，由服务层补零。
func (r *Repo) DailyUsage(since time.Time) ([]domain.UsageStatVO, error) {
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
			sumInputSQL+" AS input, SUM(output) AS output, SUM(total) AS total,"+
			"COALESCE(SUM(cached), 0) AS cached,"+
			"COUNT(*) AS calls, COALESCE(SUM(latency_ms), 0) AS latency_ms").
		Where("created_at >= ?", since.UnixMilli()).
		Group("day").Order("day ASC").Scan(&rows).Error
	if err != nil {
		return nil, wrapDB("汇总每日用量", err)
	}
	out := make([]domain.UsageStatVO, 0, len(rows))
	for _, v := range rows {
		out = append(out, domain.UsageStatVO{
			Day: v.Day, Input: v.Input, Output: v.Output, Cached: v.Cached,
			Total: v.Total, Calls: v.Calls, LatencyMs: v.LatencyMs,
		})
	}
	return out, nil
}

// ModelUsage 汇总 days 天内按模型的 token 用量，按消耗从多到少。
func (r *Repo) ModelUsage(since time.Time, limit int) ([]domain.UsageStatVO, error) {
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
			sumInputSQL+" AS input, SUM(output) AS output, COALESCE(SUM(cached), 0) AS cached").
		Where("created_at >= ?", since.UnixMilli()).
		Group("model").Order("total DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, wrapDB("按模型汇总用量", err)
	}
	out := make([]domain.UsageStatVO, 0, len(rows))
	for _, v := range rows {
		out = append(out, domain.UsageStatVO{Model: v.Model, Total: v.Total, Calls: v.Calls,
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
	return out, wrapDB("汇总会话用量", err)
}

// CountSessions 返回会话总数。
func (r *Repo) CountSessions() (int64, error) {
	var n int64
	err := r.db.Model(&domain.SessionDO{}).Count(&n).Error
	return n, wrapDB("统计会话数", err)
}

// ContextUsage 汇总每次调用发出时的上下文占用。context 为 0 的历史行（旧数据）不参与。
func (r *Repo) ContextUsage(since time.Time) (domain.ContextStatVO, error) {
	var rows []struct {
		Avg  int
		Peak int
	}
	err := r.db.Model(&domain.TokenUsageDO{}).
		Select("COALESCE(CAST(AVG(context) AS INTEGER), 0) AS avg, COALESCE(MAX(context), 0) AS peak").
		Where("created_at >= ? AND context > 0", since.UnixMilli()).
		Scan(&rows).Error
	if err != nil {
		return domain.ContextStatVO{}, wrapDB("汇总上下文水位", err)
	}
	if len(rows) == 0 {
		return domain.ContextStatVO{}, nil
	}
	return domain.ContextStatVO{Avg: rows[0].Avg, Peak: rows[0].Peak}, nil
}
