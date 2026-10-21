// 统计聚合根：用量按天 / 模型 / 会话的聚合形态，来源是 PI 的会话文件。
package domain

// StatsTotalsVO 区间总量。
// 上游把缓存命中与未命中分两个字段报：提示词量 = Input + Cached + CacheWrite，
// 命中率的分母是它，不是 Input。
type StatsTotalsVO struct {
	// Input 是未命中缓存的提示词；Cached 是命中部分；CacheWrite 是新写入部分。
	Input      int `json:"input"`
	Output     int `json:"output"`
	Cached     int `json:"cached"`
	CacheWrite int `json:"cache_write"`
	// Total 是提示词总量加输出；上游给的总量小于两者之和时以两者之和为准。
	Total int     `json:"total"`
	Cost  float64 `json:"cost"`
	Calls int     `json:"calls"`
	// Sessions / Messages 是区间内有活动的会话数与消息数。
	Sessions int `json:"sessions"`
	Messages int `json:"messages"`
}

// StatsDailyVO 一个聚合桶；天粒度 date 形如 2026-09-28，
// 小时粒度形如 2026-10-19 14:00。
type StatsDailyVO struct {
	Date       string  `json:"date"`
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	Cached     int     `json:"cached"`
	CacheWrite int     `json:"cache_write"`
	Total      int     `json:"total"`
	Cost       float64 `json:"cost"`
	Calls      int     `json:"calls"`
}

// StatsModelVO 按模型归因。
type StatsModelVO struct {
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	Cached     int     `json:"cached"`
	CacheWrite int     `json:"cache_write"`
	Total      int     `json:"total"`
	Cost       float64 `json:"cost"`
	Calls      int     `json:"calls"`
}

// StatsSessionVO 用量最高的会话。
type StatsSessionVO struct {
	SessionID string  `json:"session_id"`
	Title     string  `json:"title"`
	Workspace string  `json:"workspace"`
	Total     int     `json:"total"`
	Cost      float64 `json:"cost"`
	UpdatedAt int64   `json:"updated_at"`
}

// StatsRESP 仪表盘出参。一次返回全部，前端不需要拼多个接口。
type StatsRESP struct {
	// Span 是请求的时间范围：24h / today / 7d / 14d / 30d。
	Span string `json:"span"`
	// Granularity 是 Daily 的聚合粒度：day（按天）或 hour（24h / today 按小时）。
	Granularity string           `json:"granularity"`
	Totals      StatsTotalsVO    `json:"totals"`
	Daily       []StatsDailyVO   `json:"daily"`
	Models      []StatsModelVO   `json:"models"`
	Sessions    []StatsSessionVO `json:"sessions"`
}
