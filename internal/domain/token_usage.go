package domain

import "WorkBaby/internal/pkg"

// TokenUsageSource token 消耗来源（用于仪表盘按场景拆分与对账）。
type TokenUsageSource string

const (
	UsageSourceChat   TokenUsageSource = "chat"
	UsageSourceMemory TokenUsageSource = "memory"
	// UsageSourceDelegate 子 Agent 委派消耗：必须与父 run 的 chat 明细分开，
	// 否则委派（可达 12 轮 + 数十次工具）的消耗无法归因，也污染父 run 的对话成本。
	UsageSourceDelegate TokenUsageSource = "delegate"
	// UsageSourceTask 后台任务消耗：与委派同理由——任务独立成 run，成本单独归因。
	UsageSourceTask TokenUsageSource = "task"
)

// TokenUsageDO 单次 LLM 调用的 token 明细。
//
// 与 chat_messages 的 token 字段区别：消息只保留最终助手消息的汇总值，
// 本表按「每一次上游调用」落一行，多轮工具循环因此可被完整统计与回溯。
type TokenUsageDO struct {
	ID               string           `gorm:"primaryKey;size:64" json:"id"`
	SessionID        string           `gorm:"size:64;index:idx_usage_time" json:"session_id"`
	RunID            string           `gorm:"size:64;index"                 json:"run_id"`
	MessageID        string           `gorm:"size:64;index"                 json:"message_id"`
	ProviderID       string           `gorm:"size:64"                       json:"provider_id"`
	Model            string           `gorm:"size:128"                      json:"model"`
	Source           TokenUsageSource `gorm:"size:16;index:idx_usage_time"  json:"source"`
	Agent            string           `gorm:"size:64"                       json:"agent"` // 委派来源（source=delegate 时为子 Agent 名）
	Turn             int              `gorm:"default:0"                     json:"turn"`
	InputTokens      int              `gorm:"default:0"                     json:"input_tokens"`
	OutputTokens     int              `gorm:"default:0"                     json:"output_tokens"`
	CacheReadTokens  int              `gorm:"default:0"                     json:"cache_read_tokens"`
	CacheWriteTokens int              `gorm:"default:0"                     json:"cache_write_tokens"`
	TotalTokens      int              `gorm:"default:0"                     json:"total_tokens"`
	CostUSD          float64          `gorm:"default:0"                     json:"cost_usd"`
	LatencyMs        int              `gorm:"default:0"                     json:"latency_ms"`
	CreatedAt        int64            `gorm:"autoCreateTime:milli;index:idx_usage_time" json:"created_at"`
}

// TableName 固定表名。
func (TokenUsageDO) TableName() string { return "token_usages" }

// TokenTrendScope 仪表盘 token 趋势的时间范围枚举。
type TokenTrendScope string

const (
	TrendScopeToday   TokenTrendScope = "today"
	TrendScopeWeek    TokenTrendScope = "week"
	TrendScopeMonth   TokenTrendScope = "month"
	TrendScopeCustom  TokenTrendScope = "custom"
	TrendScopeDefault TokenTrendScope = TrendScopeToday
)

// TokenTrendBucket token 趋势的聚合粒度：今日按小时，其余按天。
type TokenTrendBucket string

const (
	TrendBucketHour TokenTrendBucket = "hour"
	TrendBucketDay  TokenTrendBucket = "day"
)

// TokenTrendRESP 仪表盘 token 折线图数据（GET /api/v1/dashboard/token-trend）。
// Labels 与三条折线等长对齐；Granularity 决定横坐标语义（24 小时 / 按天递增）。
type TokenTrendRESP struct {
	Scope       TokenTrendScope  `json:"scope"`
	Granularity TokenTrendBucket `json:"granularity"`
	StartAt     int64            `json:"start_at"`
	EndAt       int64            `json:"end_at"`
	Labels      []string         `json:"labels"`
	Input       []int64          `json:"input"`
	Output      []int64          `json:"output"`
	CacheRead   []int64          `json:"cache_read"`
	Total       int64            `json:"total"`
	CostUSD     float64          `json:"cost_usd"`
}

// TokenBucketDTO 单个时间桶的聚合结果（repo → service 传输）。
// BucketMs 为桶起点（含时区偏移后的对齐值），service 据此补齐空桶。
type TokenBucketDTO struct {
	BucketMs        int64 `json:"bucket_ms"`
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	CacheReadTokens int64 `json:"cache_read_tokens"`
}

// TokenSummaryRESP 区间 token 汇总（折线图右上角的合计卡片）。
type TokenSummaryRESP struct {
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CacheReadTokens int64   `json:"cache_read_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	CostUSD         float64 `json:"cost_usd"`
	Calls           int64   `json:"calls"`
}

// SettingKeyPricingPrefix 模型单价的 system_settings KV 键前缀（key = 前缀 + model 名）。
const SettingKeyPricingPrefix = "pricing."

// ModelPricing 模型单价（USD / 每百万 token）；经 KV（pricing.<model>）配置，缺省 0 = 不计费。
type ModelPricing struct {
	InputPerM     float64 `json:"input_per_m"`
	OutputPerM    float64 `json:"output_per_m"`
	CacheReadPerM float64 `json:"cache_read_per_m"`
}

// CostUSD 按用量计费：缓存命中部分单独计价（单价未配置时按输入全价兜底）。
// 未配置任何单价返回 0（不计费）。
func (p ModelPricing) CostUSD(input, output, cacheRead int64) float64 {
	if p.InputPerM <= 0 && p.OutputPerM <= 0 {
		return 0
	}
	cachePrice := p.CacheReadPerM
	if cachePrice <= 0 {
		cachePrice = p.InputPerM
	}
	nonCached := float64(input) - float64(cacheRead)
	if nonCached < 0 {
		nonCached = 0
	}
	return nonCached/1e6*p.InputPerM + float64(cacheRead)/1e6*cachePrice + float64(output)/1e6*p.OutputPerM
}

// TokenTrendREQ 趋势查询入参；StartAt/EndAt 毫秒时间戳（scope=custom 时必填）。
type TokenTrendREQ struct {
	Scope   TokenTrendScope `json:"scope"`
	StartAt int64           `json:"start_at"`
	EndAt   int64           `json:"end_at"`
}

// 包级错误变量；段位 2000（持久化）。
var ErrTokenTrendRange = pkg.New(2014, "时间范围不合法（start_at 必须小于 end_at 且跨度 ≤ 366 天）", "")
