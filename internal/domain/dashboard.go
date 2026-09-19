package domain

// DashboardStatsRESP 仪表盘统计聚合（GET /api/v1/dashboard/stats）。
// 只读快照：累计计数 + 今日活动 + 最近活动列表 + Go 运行时信息。
type DashboardStatsRESP struct {
	AiToolsTotal   int64               `json:"ai_tools_total"`
	TodaySessions  int64               `json:"today_sessions"`
	TodayMessages  int64               `json:"today_messages"`
	TodayTokens    int64               `json:"today_tokens"`
	RecentMessages []RecentMessageRESP `json:"recent_messages"`
	System         *SystemInfoRESP     `json:"system"`
}

// RecentMessageRESP 最近消息（活动流；content 由 service 截断）。
type RecentMessageRESP struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

// SystemInfoRESP Go 运行时信息（替代旧 Java/JVM 字段）。
type SystemInfoRESP struct {
	OSName      string `json:"os_name"`
	OSArch      string `json:"os_arch"`
	GoVersion   string `json:"go_version"`
	UserHome    string `json:"user_home"`
	NumCPU      int    `json:"num_cpu"`
	Goroutines  int    `json:"goroutines"`
	HeapAllocMB int64  `json:"heap_alloc_mb"`
	HeapSysMB   int64  `json:"heap_sys_mb"`
}

// DashboardTrendRESP 最近 N 天趋势（GET /api/v1/dashboard/trend?range=N）。
// Days 与三个计数切片等长对齐，日期格式 "MM-DD"。
type DashboardTrendRESP struct {
	Days     []string `json:"days"`
	Sessions []int64  `json:"sessions"`
	Messages []int64  `json:"messages"`
	Tokens   []int64  `json:"tokens"`
}
