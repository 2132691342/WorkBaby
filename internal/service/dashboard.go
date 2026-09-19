package service

import (
	"context"
	"os"
	"runtime"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// DashboardService 仪表盘只读聚合编排；纯 Go 可单测（不引 gin/Wails）。
type DashboardService struct {
	repo   *repo.DashboardRepo
	usages *repo.TokenUsageRepo
	tools  *ToolService
}

// NewDashboardService 注入统计仓储、token 明细仓储与工具服务。
func NewDashboardService(r *repo.DashboardRepo, usages *repo.TokenUsageRepo, tools *ToolService) *DashboardService {
	return &DashboardService{repo: r, usages: usages, tools: tools}
}

// Stats 组装统计快照（累计 + 今日 + 最近活动 + 运行时）。
func (s *DashboardService) Stats(ctx context.Context) (*domain.DashboardStatsRESP, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()

	var (
		todaySessions, todayMessages, todayTokens int64
	)
	var err error

	if todaySessions, err = s.repo.CountTodaySessions(ctx, todayStart); err != nil {
		return nil, err
	}
	if todayMessages, err = s.repo.CountTodayMessages(ctx, todayStart); err != nil {
		return nil, err
	}
	if todayTokens, err = s.repo.SumTodayTokens(ctx, todayStart); err != nil {
		return nil, err
	}

	recentMessages, err := s.recentMessages(ctx)
	if err != nil {
		return nil, err
	}
	var toolsTotal int64
	if ts, lerr := s.tools.ListTools(ctx); lerr == nil {
		toolsTotal = int64(len(ts))
	}

	return &domain.DashboardStatsRESP{
		AiToolsTotal:   toolsTotal,
		TodaySessions:  todaySessions,
		TodayMessages:  todayMessages,
		TodayTokens:    todayTokens,
		RecentMessages: recentMessages,
		System:         systemInfo(),
	}, nil
}

// Trend 最近 N 天趋势。
func (s *DashboardService) Trend(ctx context.Context, days int) (*domain.DashboardTrendRESP, error) {
	return s.repo.Trend(ctx, days)
}

// maxTrendSpanDays 自定义范围最大跨度（防止前端误传导致内存爆掉）。
const maxTrendSpanDays = 366

// TokenTrend token 消耗趋势：today 按 24 小时桶（未来时段为 0），week/month/custom 按自然日桶。
func (s *DashboardService) TokenTrend(ctx context.Context, req domain.TokenTrendREQ) (*domain.TokenTrendRESP, error) {
	scope := req.Scope
	if scope == "" {
		scope = domain.TrendScopeDefault
	}
	now := time.Now()
	start, end, gran := tokenTrendWindow(now, scope, req)
	if end <= start || end-start > int64(maxTrendSpanDays)*24*int64(time.Hour.Milliseconds()) {
		return nil, domain.ErrTokenTrendRange
	}

	// SQLite 存 UTC epoch；按本地日界切桶需补偿时区偏移
	_, offsetSec := now.Zone()
	offsetMs := int64(offsetSec) * 1000

	buckets, err := s.usages.Aggregate(ctx, start, end, gran, offsetMs)
	if err != nil {
		return nil, err
	}
	summary, err := s.usages.Summarize(ctx, start, end)
	if err != nil {
		return nil, err
	}

	labels, stepMs := tokenTrendAxis(start, end, gran, offsetMs)
	out := &domain.TokenTrendRESP{
		Scope:       scope,
		Granularity: gran,
		StartAt:     start,
		EndAt:       end,
		Labels:      labels,
		Input:       make([]int64, len(labels)),
		Output:      make([]int64, len(labels)),
		CacheRead:   make([]int64, len(labels)),
		Total:       summary.TotalTokens,
		CostUSD:     summary.CostUSD,
	}
	// 对齐：桶起点与聚合 SQL 用同一套取整规则（(ts+offset)/step*step），故可直接索引换算
	base := (start + offsetMs) / stepMs * stepMs
	idx := make(map[int64]int, len(labels))
	for i := range labels {
		idx[base+int64(i)*stepMs] = i
	}
	for _, b := range buckets {
		i, ok := idx[b.BucketMs]
		if !ok {
			continue
		}
		out.Input[i] = b.InputTokens
		out.Output[i] = b.OutputTokens
		out.CacheRead[i] = b.CacheReadTokens
	}
	pkg.L.Info("dashboard token trend", "scope", scope, "granularity", gran,
		"buckets", len(labels), "calls", summary.Calls, "total", summary.TotalTokens)
	return out, nil
}

// TokenSummary 区间 token 汇总（配合折线图的合计卡片）。
func (s *DashboardService) TokenSummary(ctx context.Context, req domain.TokenTrendREQ) (domain.TokenSummaryRESP, error) {
	now := time.Now()
	start, end, _ := tokenTrendWindow(now, req.Scope, req)
	if end <= start {
		return domain.TokenSummaryRESP{}, domain.ErrTokenTrendRange
	}
	return s.usages.Summarize(ctx, start, end)
}

// tokenTrendWindow 解析时间窗 → [start, end) 毫秒 + 聚合粒度。
func tokenTrendWindow(now time.Time, scope domain.TokenTrendScope, req domain.TokenTrendREQ) (int64, int64, domain.TokenTrendBucket) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch scope {
	case domain.TrendScopeWeek:
		return today.AddDate(0, 0, -6).UnixMilli(), now.UnixMilli() + 1, domain.TrendBucketDay
	case domain.TrendScopeMonth:
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return monthStart.UnixMilli(), now.UnixMilli() + 1, domain.TrendBucketDay
	case domain.TrendScopeCustom:
		start := req.StartAt
		end := req.EndAt
		if end > 0 && end < start {
			start, end = end, start
		}
		if start <= 0 {
			start = today.UnixMilli()
		}
		if end <= 0 {
			end = start + 24*int64(time.Hour.Milliseconds())
		}
		// 对齐到自然日：起始日 00:00 → 结束日次日 00:00（前端只精确到天）
		s := time.UnixMilli(start).In(now.Location())
		e := time.UnixMilli(end).In(now.Location())
		start = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
		end = time.Date(e.Year(), e.Month(), e.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1).UnixMilli()
		return start, end, domain.TrendBucketDay
	default:
		return today.UnixMilli(), today.AddDate(0, 0, 1).UnixMilli(), domain.TrendBucketHour
	}
}

// tokenTrendAxis 生成横坐标标签与桶步长（毫秒）。
func tokenTrendAxis(startMs, endMs int64, gran domain.TokenTrendBucket, offsetMs int64) ([]string, int64) {
	stepMs := int64(time.Hour.Milliseconds())
	layout := "15"
	if gran == domain.TrendBucketDay {
		stepMs = int64(24 * time.Hour.Milliseconds())
		layout = "01-02"
	}
	// 向上取整：today 的 24h 闭开区间恰好 24 桶；week 的 [6.x 天] 需补成 7 桶
	n := int((endMs - startMs + stepMs - 1) / stepMs)
	if n < 1 {
		n = 1
	}
	if n > 512 { // 与 maxTrendSpanDays 双保险
		n = 512
	}
	loc := time.FixedZone("local", int(offsetMs/1000))
	labels := make([]string, 0, n)
	for i := 0; i < n; i++ {
		labels = append(labels, time.UnixMilli(startMs+int64(i)*stepMs).In(loc).Format(layout))
	}
	return labels, stepMs
}

// Overview 管理后台概览：版本/环境 + 各领域计数。
func (s *DashboardService) Overview(ctx context.Context, version, phase, home string, dbOK bool) (*domain.AdminOverviewRESP, error) {
	sessions, err := s.repo.CountSessions(ctx)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.CountMessages(ctx)
	if err != nil {
		return nil, err
	}
	knowledge, err := s.repo.CountKnowledgeDocs(ctx)
	if err != nil {
		return nil, err
	}
	providers, err := s.repo.CountProviders(ctx)
	if err != nil {
		return nil, err
	}

	var toolsTotal int
	if ts, lerr := s.tools.ListTools(ctx); lerr == nil {
		toolsTotal = len(ts)
	}

	return &domain.AdminOverviewRESP{
		Version:       version,
		Phase:         phase,
		Home:          home,
		DBEnabled:     dbOK,
		ToolCount:     toolsTotal,
		Providers:     int(providers),
		Sessions:      sessions,
		Messages:      messages,
		KnowledgeDocs: knowledge,
	}, nil
}

func (s *DashboardService) recentMessages(ctx context.Context) ([]domain.RecentMessageRESP, error) {
	rows, err := s.repo.ListRecentMessages(ctx, 8)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecentMessageRESP, 0, len(rows))
	for _, m := range rows {
		content := m.Content
		if r := []rune(content); len(r) > 120 {
			content = string(r[:120])
		}
		out = append(out, domain.RecentMessageRESP{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      string(m.Role),
			Content:   content,
			CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

// systemInfo 采集 Go 运行时信息（替代旧 Java/JVM 字段）。
func systemInfo() *domain.SystemInfoRESP {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	home, _ := os.UserHomeDir()
	return &domain.SystemInfoRESP{
		OSName:      runtime.GOOS,
		OSArch:      runtime.GOARCH,
		GoVersion:   runtime.Version(),
		UserHome:    home,
		NumCPU:      runtime.NumCPU(),
		Goroutines:  runtime.NumGoroutine(),
		HeapAllocMB: int64(ms.HeapAlloc / (1024 * 1024)),
		HeapSysMB:   int64(ms.HeapSys / (1024 * 1024)),
	}
}
