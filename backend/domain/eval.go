// 评估与基线聚合根：一次 run 打几分，与历史最好成绩比什么（specs/19）。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrEvalNotFound    = pkg.New("9221", "这条评估记录不存在", "")
	ErrEvalDuplicated  = pkg.New("9222", "同一次运行已经打过分了", "改分请走更新")
	ErrEvalNoMetrics   = pkg.New("9223", "这次运行没有可采集的指标", "")
	ErrBaselineMissing = pkg.New("9224", "这条基线不存在", "")
	ErrCompareHalf     = pkg.New("9225", "对比需要基线与一次运行各一份", "")
)

// 评估范围：评估挂在会话上，一次对话就是一个可比较的对象。
const EvalScopeSession = "session"

// EvalDO 一条评估。MetricsJSON 是自动采集的客观指标（只读），
// Score / Note 是人工填写的主观分；两者同表不同源。
type EvalDO struct {
	ID          string `gorm:"primaryKey;size:64" json:"id"`
	RunID       string `gorm:"index;size:64" json:"run_id"`
	Scope       string `gorm:"size:16" json:"scope"`
	ScopeID     string `gorm:"index;size:64" json:"scope_id"`
	Score       int    `json:"score"` // 1-5；0 = 只采了客观指标
	Note        string `gorm:"type:text" json:"note"`
	MetricsJSON string `gorm:"type:text" json:"-"`
	Orphaned    bool   `json:"orphaned"` // 来源会话已删除，评估记录本身保留
	CreatedAt   int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 显式指定表名。
func (EvalDO) TableName() string { return "orchestra_evals" }

// BaselineDO 一条回归基线：引用评估 id 而非拷贝数值——
// 基线要能看出「当时的那次」到底是什么样。
type BaselineDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	EvalID    string `gorm:"index;size:64" json:"eval_id"`
	Scope     string `gorm:"size:16" json:"scope"`
	ScopeID   string `gorm:"index;size:64" json:"scope_id"`
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 显式指定表名。
func (BaselineDO) TableName() string { return "orchestra_baselines" }

// EvalMetrics 客观指标：自动采集，界面与回归对比都读它。
type EvalMetrics struct {
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	CachedTokens int   `json:"cached_tokens"`
	LatencyMS    int64 `json:"latency_ms"`
	ToolCalls    int   `json:"tool_calls"`
	ToolFailures int   `json:"tool_failures"`
	Approvals    int   `json:"approvals"` // 人工干预次数
	Truncated    bool  `json:"truncated"` // 是否撞过输出上限
}

// EvalVO 出参：评估 + 指标 + 与所属范围基线的差值（有基线才非零）。
type EvalVO struct {
	ID         string      `json:"id"`
	RunID      string      `json:"run_id"`
	Scope      string      `json:"scope"`
	ScopeID    string      `json:"scope_id"`
	Score      int         `json:"score"`
	Note       string      `json:"note"`
	Metrics    EvalMetrics `json:"metrics"`
	CreatedAt  int64       `json:"created_at"`
	ScoreDelta int         `json:"score_delta"` // 与基线分之差；无基线为 0
	TotalDelta int         `json:"total_delta"` // 与基线总 token 差
	Orphaned   bool        `json:"orphaned"`    // 来源会话已删除
}

// BaselineVO 基线出参。
type BaselineVO struct {
	ID        string `json:"id"`
	EvalID    string `json:"eval_id"`
	Scope     string `json:"scope"`
	ScopeID   string `json:"scope_id"`
	Score     int    `json:"score"`
	CreatedAt int64  `json:"created_at"`
}

// RegisterEvalREQ 登记主观分（客观指标自动带上）。
type RegisterEvalREQ struct {
	RunID string `json:"run_id"`
	Score int    `json:"score"`
	Note  string `json:"note"`
}

// SaveBaselineREQ 存基线。
type SaveBaselineREQ struct {
	EvalID string `json:"eval_id"`
	Scope  string `json:"scope"`
}

// CompareVO 一次回归对比：指标差 + 双方结果摘要。
type CompareVO struct {
	BaselineID  string      `json:"baseline_id"`
	RunID       string      `json:"run_id"`
	ScoreDelta  int         `json:"score_delta"`
	TotalDelta  int         `json:"total_delta"`
	BaseSummary string      `json:"base_summary"`
	RunSummary  string      `json:"run_summary"`
	BaseMetrics EvalMetrics `json:"base_metrics"`
	RunMetrics  EvalMetrics `json:"run_metrics"`
}
