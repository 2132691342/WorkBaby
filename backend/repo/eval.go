// 评估与用量读取：评估记录、基线，以及按会话汇总用量与工具成败。
package repo

import (
	"encoding/json"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// CreateEval 落一条评估。
func (r *Repo) CreateEval(e *domain.EvalDO) error {
	return wrapDB("evals.create", r.db.Create(e).Error)
}

// UpdateEvalScore 只改人工分与备注：客观指标是采集来的，不能被覆写。
func (r *Repo) UpdateEvalScore(id string, score int, note string) error {
	return wrapDB("evals.score", r.db.Model(&domain.EvalDO{}).Where("id = ?", id).
		Updates(map[string]any{"score": score, "note": note}).Error)
}

// EvalByRun 按运行 id 取评估；没有返回 nil。
func (r *Repo) EvalByRun(runID string) (*domain.EvalDO, error) {
	var row domain.EvalDO
	if err := r.db.Where("run_id = ?", runID).First(&row).Error; err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, wrapDB("evals.byrun", err)
	}
	return &row, nil
}

// ListEvals 按范围列评估（scope / scope_id 为空表示不限）。
func (r *Repo) ListEvals(scope, scopeID string) ([]domain.EvalDO, error) {
	q := r.db.Order("created_at DESC")
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	if scopeID != "" {
		q = q.Where("scope_id = ?", scopeID)
	}
	var rows []domain.EvalDO
	if err := q.Find(&rows).Error; err != nil {
		return nil, wrapDB("evals.list", err)
	}
	return rows, nil
}

// GetEval 取一条评估。
func (r *Repo) GetEval(id string) (*domain.EvalDO, error) {
	var row domain.EvalDO
	if err := r.db.Where("id = ?", id).First(&row).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrEvalNotFound
		}
		return nil, wrapDB("evals.get", err)
	}
	return &row, nil
}

// MarkEvalsOrphaned 把某个来源的评估标成「来源已删除」：账留着，但看得出它没有归属了。
func (r *Repo) MarkEvalsOrphaned(scope, scopeID string) error {
	return wrapDB("evals.orphan", r.db.Model(&domain.EvalDO{}).
		Where("scope = ? AND scope_id = ?", scope, scopeID).
		Update("orphaned", true).Error)
}

// CreateBaseline 存一条基线。
func (r *Repo) CreateBaseline(b *domain.BaselineDO) error {
	return wrapDB("baselines.create", r.db.Create(b).Error)
}

// DeleteBaseline 删基线。
func (r *Repo) DeleteBaseline(id string) error {
	return wrapDB("baselines.delete", r.db.Where("id = ?", id).Delete(&domain.BaselineDO{}).Error)
}

// ListBaselines 按创建时间倒序列基线。
func (r *Repo) ListBaselines() ([]domain.BaselineDO, error) {
	rows := []domain.BaselineDO{}
	if err := r.db.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, wrapDB("baselines.list", err)
	}
	return rows, nil
}

// GetBaseline 取基线。
func (r *Repo) GetBaseline(id string) (*domain.BaselineDO, error) {
	var row domain.BaselineDO
	if err := r.db.Where("id = ?", id).First(&row).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrBaselineMissing
		}
		return nil, wrapDB("baselines.get", err)
	}
	return &row, nil
}

// BaselineByScope 取某个范围当前的基线；没有返回 nil。
func (r *Repo) BaselineByScope(scope, scopeID string) (*domain.BaselineDO, error) {
	var row domain.BaselineDO
	if err := r.db.Where("scope = ? AND scope_id = ?", scope, scopeID).
		Order("created_at DESC").First(&row).Error; err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, wrapDB("baselines.scope", err)
	}
	return &row, nil
}

// UsageOfSession 汇总一个会话的用量（三次求和一次查完）。
func (r *Repo) UsageOfSession(sessionID string) (input, output, cached int, err error) {
	var row struct {
		In     int `gorm:"column:input_sum"`
		Out    int `gorm:"column:output_sum"`
		Cached int `gorm:"column:cached_sum"`
	}
	err = r.db.Model(&domain.TokenUsageDO{}).
		// 别名不能叫 in：它是 SQL 保留字，写成 AS in 会直接语法错误。
		Select("COALESCE(SUM(input),0) AS input_sum, COALESCE(SUM(output),0) AS output_sum, COALESCE(SUM(cached),0) AS cached_sum").
		Where("session_id = ?", sessionID).
		Scan(&row).Error
	if err != nil {
		return 0, 0, 0, wrapDB("usage.session", err)
	}
	return row.In, row.Out, row.Cached, nil
}

// ToolStatsOfSession 从运行记录的时间线汇总一个会话的工具调用与失败次数：
// 观测域只统计自己记的账，不回查 PI 的会话文件。
func (r *Repo) ToolStatsOfSession(sessionID string) (calls, failures int, err error) {
	var rows []domain.RunDO
	if err := r.db.Select("timeline_json").
		Where("session_id = ?", sessionID).
		Find(&rows).Error; err != nil {
		return 0, 0, wrapDB("usage.tools", err)
	}
	for i := range rows {
		timeline := domain.DecodeRunTimeline(rows[i].TimelineJSON)
		if timeline == nil {
			continue
		}
		for _, step := range timeline.Steps {
			if step.Kind != domain.StepTool {
				continue
			}
			calls++
			if !step.OK {
				failures++
			}
		}
	}
	return calls, failures, nil
}

// marshalJSON / unmarshalJSON 是 service 层拼 JSON 列的两个小工具。
func marshalJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func unmarshalJSON(raw string, v any) error { return json.Unmarshal([]byte(raw), v) }

// newForkID 是新分支会话的 id。
func newForkID() string { return pkg.NewID(domain.PrefixSession) }
