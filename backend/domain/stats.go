// 统计聚合根：用量按天 / 模型 / 会话的聚合形态，来源是 PI 的会话文件。
package domain

// StatsTotalsVO 区间总量。
type StatsTotalsVO struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	// Cached 命中上游缓存的输入量；命中率 = Cached / Input。
	Cached int     `json:"cached"`
	Total  int     `json:"total"`
	Cost   float64 `json:"cost"`
	Calls  int     `json:"calls"`
	// Sessions / Messages 是区间内有活动的会话数与消息数。
	Sessions int `json:"sessions"`
	Messages int `json:"messages"`
}

// StatsDailyVO 每日一柱，date 形如 2026-09-28。
type StatsDailyVO struct {
	Date   string  `json:"date"`
	Input  int     `json:"input"`
	Output int     `json:"output"`
	Cached int     `json:"cached"`
	Total  int     `json:"total"`
	Cost   float64 `json:"cost"`
	Calls  int     `json:"calls"`
}

// StatsModelVO 按模型归因。
type StatsModelVO struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Input    int     `json:"input"`
	Output   int     `json:"output"`
	Cached   int     `json:"cached"`
	Total    int     `json:"total"`
	Cost     float64 `json:"cost"`
	Calls    int     `json:"calls"`
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
	Days     int              `json:"days"`
	Totals   StatsTotalsVO    `json:"totals"`
	Daily    []StatsDailyVO   `json:"daily"`
	Models   []StatsModelVO   `json:"models"`
	Sessions []StatsSessionVO `json:"sessions"`
}
