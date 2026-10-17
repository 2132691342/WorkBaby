// 评估与回归：一次 run 打几分、指标如何，与基线比差在哪（specs/19）。
package service

import (
	"encoding/json"
	"strconv"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// EvalService 管评估记录与基线。评估挂在会话上：一次对话就是一个可比较的对象，
// 再细到 run 级基线没有意义（同一会话里两次 run 的输入本来就不同）。
type EvalService struct{ env *Env }

// NewEvalService 构造评估服务。
func NewEvalService(env *Env) *EvalService { return &EvalService{env: env} }

// Auto 在一次 run 收尾后自动采一次客观指标：失败也采——
// 「一次失败的运行花了多少 token」同样是回归要看的。
func (s *EvalService) Auto(sessionID, runID string) {
	if e, _ := s.env.Repo.EvalByRun(runID); e != nil {
		return
	}
	m := s.metricsOf(sessionID)
	row := &domain.EvalDO{
		ID: pkg.NewID(domain.PrefixEval), RunID: runID,
		Scope: domain.EvalScopeSession, ScopeID: sessionID,
		MetricsJSON: encodeMetrics(m),
	}
	if err := s.env.Repo.CreateEval(row); err != nil {
		pkg.Warnf("eval: 采集失败 session=%s: %v", sessionID, err)
	}
}

// List 评估列表（含与所属会话基线的差值）。
func (s *EvalService) List(scope, scopeID string) ([]domain.EvalVO, error) {
	evals, err := s.env.Repo.ListEvals(scope, scopeID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.EvalVO, 0, len(evals))
	for _, e := range evals {
		vo := evalVO(e)
		if b, _ := s.env.Repo.BaselineByScope(e.Scope, e.ScopeID); b != nil {
			if base, err := s.env.Repo.GetEval(b.EvalID); err == nil {
				vo.ScoreDelta = vo.Score - base.Score
				vo.TotalDelta = totalOf(vo.Metrics) - totalOf(metricsOf(*base))
			}
		}
		out = append(out, vo)
	}
	return out, nil
}

// Register 登记主观分。客观指标已在收尾时自动采集，这里只补人的判断。
func (s *EvalService) Register(req domain.RegisterEvalREQ) (*domain.EvalVO, error) {
	if req.Score < 1 || req.Score > 5 {
		return nil, pkg.New("9221", "评分只能是 1 到 5 分", "")
	}
	e, err := s.env.Repo.EvalByRun(req.RunID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, domain.ErrEvalNotFound
	}
	if err := s.env.Repo.UpdateEvalScore(e.ID, req.Score, req.Note); err != nil {
		return nil, err
	}
	e.Score, e.Note = req.Score, req.Note
	vo := evalVO(*e)
	return &vo, nil
}

// Baselines 基线列表。
func (s *EvalService) Baselines() ([]domain.BaselineVO, error) {
	list, err := s.env.Repo.ListBaselines()
	if err != nil {
		return nil, err
	}
	out := make([]domain.BaselineVO, 0, len(list))
	for _, b := range list {
		vo := domain.BaselineVO{ID: b.ID, EvalID: b.EvalID, Scope: b.Scope, ScopeID: b.ScopeID, CreatedAt: b.CreatedAt}
		if e, err := s.env.Repo.GetEval(b.EvalID); err == nil {
			vo.Score = e.Score
		}
		out = append(out, vo)
	}
	return out, nil
}

// SaveBaseline 把一条评估存为基线。
func (s *EvalService) SaveBaseline(req domain.SaveBaselineREQ) (*domain.BaselineVO, error) {
	e, err := s.env.Repo.GetEval(req.EvalID)
	if err != nil {
		return nil, err
	}
	b := &domain.BaselineDO{
		ID: pkg.NewID(domain.PrefixBaseline), EvalID: e.ID,
		Scope: e.Scope, ScopeID: e.ScopeID,
	}
	if err := s.env.Repo.CreateBaseline(b); err != nil {
		return nil, err
	}
	return &domain.BaselineVO{ID: b.ID, EvalID: b.EvalID, Scope: b.Scope, ScopeID: b.ScopeID, Score: e.Score, CreatedAt: b.CreatedAt}, nil
}

// DeleteBaseline 删基线。
func (s *EvalService) DeleteBaseline(id string) error { return s.env.Repo.DeleteBaseline(id) }

// Compare 一次回归对比：基线 vs 一次 run。
func (s *EvalService) Compare(baselineID, runID string) (*domain.CompareVO, error) {
	if baselineID == "" || runID == "" {
		return nil, domain.ErrCompareHalf
	}
	b, err := s.env.Repo.GetBaseline(baselineID)
	if err != nil {
		return nil, err
	}
	base, err := s.env.Repo.GetEval(b.EvalID)
	if err != nil {
		return nil, err
	}
	run, err := s.env.Repo.EvalByRun(runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, domain.ErrEvalNotFound
	}
	bm, rm := metricsOf(*base), metricsOf(*run)
	return &domain.CompareVO{
		BaselineID: baselineID, RunID: runID,
		ScoreDelta:  run.Score - base.Score,
		TotalDelta:  totalOf(rm) - totalOf(bm),
		BaseSummary: summaryOf(*base), RunSummary: summaryOf(*run),
		BaseMetrics: bm, RunMetrics: rm,
	}, nil
}

// metricsOf 采集一次会话的客观指标。
// 用量从 token_usages 汇总；工具成败由旁听器在事件里累加后落进 runs，
// 不再回查条目链（正文已不在本机库里）。
func (s *EvalService) metricsOf(sessionID string) domain.EvalMetrics {
	in, out, cached, _ := s.env.Repo.UsageOfSession(sessionID)
	calls, failures, _ := s.env.Repo.ToolStatsOfSession(sessionID)
	return domain.EvalMetrics{
		InputTokens: in, OutputTokens: out, CachedTokens: cached,
		ToolCalls: calls, ToolFailures: failures,
	}
}

// summaryOf 取评估对象的结论摘要，供对比页左右并排看。
func summaryOf(e domain.EvalDO) string { return e.Note }

// evalVO 把 DO 转出参。
func evalVO(e domain.EvalDO) domain.EvalVO {
	return domain.EvalVO{
		ID: e.ID, RunID: e.RunID, Scope: e.Scope, ScopeID: e.ScopeID,
		Score: e.Score, Note: e.Note, Metrics: metricsOf(e), CreatedAt: e.CreatedAt,
	}
}

// metricsOf 解出指标 JSON；坏值给零值（少几个数字，而不是整页打不开）。
func metricsOf(e domain.EvalDO) domain.EvalMetrics {
	var m domain.EvalMetrics
	if e.MetricsJSON == "" {
		return m
	}
	if err := json.Unmarshal([]byte(e.MetricsJSON), &m); err != nil {
		return domain.EvalMetrics{}
	}
	return m
}

// encodeMetrics 编码指标列。
func encodeMetrics(m domain.EvalMetrics) string {
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(raw)
}

// totalOf 总 token：回归比较读的是它。
func totalOf(m domain.EvalMetrics) int { return m.InputTokens + m.OutputTokens + m.CachedTokens }

// parseRetention 读保留期设置；写坏的值按缺省处理。
func parseRetention(raw string) int {
	if raw == "" {
		return domain.DefaultRetentionDays
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return domain.DefaultRetentionDays
	}
	if n > domain.MaxRetentionDays {
		return domain.MaxRetentionDays
	}
	return n
}
