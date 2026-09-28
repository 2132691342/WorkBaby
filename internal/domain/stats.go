package domain

import "WorkBaby/internal/pkg"

var ErrInvalidRange = pkg.New(1109, "时间跨度不合理", "")

// StatsReq 统计查询入参；Days 缺省 14 天，上限 90 天。
type StatsREQ struct {
	Days int `json:"days"`
}

// StatsTotalsVO 总量卡片。
type StatsTotalsVO struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	Total      int `json:"total"`
	Calls      int `json:"calls"`
	Sessions   int `json:"sessions"`
	LatencyMs  int64 `json:"latency_ms"`
	AvgLatency int64 `json:"avg_latency_ms"`
}

// StatsDailyVO 每日一柱，date 形如 2026-09-28。
type StatsDailyVO struct {
	Date   string `json:"date"`
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Total  int    `json:"total"`
}

// StatsModelVO 按模型归因。
type StatsModelVO struct {
	Model string `json:"model"`
	Total int    `json:"total"`
	Calls int    `json:"calls"`
}

// StatsSessionVO 用量最高的会话。
type StatsSessionVO struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	Total     int    `json:"total"`
}

// StatsRESP 仪表盘出参。一次返回全部，前端不需要拼多个接口。
type StatsRESP struct {
	Days     int              `json:"days"`
	Totals   StatsTotalsVO    `json:"totals"`
	Daily    []StatsDailyVO   `json:"daily"`
	Models   []StatsModelVO   `json:"models"`
	Sessions []StatsSessionVO `json:"sessions"`
}
