// 统计聚合根：token 用量按天 / 模型 / 会话的聚合形态。
package domain

// StatsReq 统计查询入参；Days 缺省 14 天，上限 90 天。
type StatsREQ struct {
	Days int `json:"days"`
}

// StatsTotalsVO 总量卡片。
type StatsTotalsVO struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	// Cached 命中上游缓存的输入量。缓存命中率 = Cached / Input，
	// 长对话里这一项决定了成本是线性涨还是几乎不涨。
	Cached     int   `json:"cached"`
	Total      int   `json:"total"`
	Calls      int   `json:"calls"`
	Sessions   int   `json:"sessions"`
	LatencyMs  int64 `json:"latency_ms"`
	AvgLatency int64 `json:"avg_latency_ms"`
	// CacheHitRate 是 0~1 的命中率，百分比由前端算。
	CacheHitRate float64 `json:"cache_hit_rate"`
	// AvgContext 每次调用发出时的上下文占用均值；PeakContext 是区间内最大那一轮。
	AvgContext  int `json:"avg_context"`
	PeakContext int `json:"peak_context"`
}

// StatsDailyVO 每日一柱，date 形如 2026-09-28。
type StatsDailyVO struct {
	Date   string `json:"date"`
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Cached int    `json:"cached"`
	Total  int    `json:"total"`
}

// StatsModelVO 按模型归因。
type StatsModelVO struct {
	Model  string `json:"model"`
	Total  int    `json:"total"`
	Calls  int    `json:"calls"`
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Cached int    `json:"cached"`
}

// StatsSessionVO 用量最高的会话。
type StatsSessionVO struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	Total     int    `json:"total"`
}

// UsageStatVO 一次按天 / 按模型的聚合结果；repo 扫描行，服务层再加工成出参。
type UsageStatVO struct {
	Model     string
	Day       string
	Input     int
	Output    int
	Cached    int
	Total     int
	Calls     int
	LatencyMs int64
}

// ContextStatVO 是区间内上下文占用的均值与峰值。
type ContextStatVO struct {
	Avg  int
	Peak int
}

// StatsRESP 仪表盘出参。一次返回全部，前端不需要拼多个接口。
type StatsRESP struct {
	Days     int              `json:"days"`
	Totals   StatsTotalsVO    `json:"totals"`
	Daily    []StatsDailyVO   `json:"daily"`
	Models   []StatsModelVO   `json:"models"`
	Sessions []StatsSessionVO `json:"sessions"`
}
