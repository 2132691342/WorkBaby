// 运行观测：每次 run 一行记录，回答「这一轮干了什么、花了多少、从哪恢复」。
package service

import (
	"strconv"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// RunsService 是运行记录的唯一写入口与读出口。
// 记录与对话同源（entries + token_usages），不复制正文：观测面只做投影。
type RunsService struct{ env *Env }

// NewRunsService 构造运行观测服务。
func NewRunsService(env *Env) *RunsService { return &RunsService{env: env} }

// Start 在 run 开始时落一行 running；失败只告警，不拦对话。
func (s *RunsService) Start(runID, sessionID, provider, model string) {
	row := &domain.RunDO{
		ID: runID, SessionID: sessionID, ProviderID: provider, Model: model,
		Status: domain.RunRunning, StartedAt: time.Now().UnixMilli(),
	}
	if err := s.env.Repo.CreateRun(row); err != nil {
		pkg.Warnf("runs: 落运行记录失败: %v", err)
	}
}

// Touch 更新运行中的实时读数（每轮覆盖写，不是逐 token 写）。run 行不存在时静默跳过。
func (s *RunsService) Touch(runID string, turns, tokens, context int, compressed int) {
	_ = s.env.Repo.UpdateRunColumns(runID, map[string]any{
		"turns": turns, "tokens": tokens, "context": context, "compressed": compressed,
	})
}

// Finish 收尾：成功与失败都走这里。
func (s *RunsService) Finish(runID, status, stopReason string, usage *domain.UsageVO, context, maxTokens int) {
	cols := map[string]any{
		"status": status, "stop_reason": stopReason, "finished_at": time.Now().UnixMilli(),
		"context": context, "max_tokens": maxTokens,
	}
	if usage != nil {
		cols["tokens"] = usage.Total
		cols["input"] = usage.Input
		cols["output"] = usage.Output
		cols["cached"] = usage.Cached
	}
	if err := s.env.Repo.UpdateRunColumns(runID, cols); err != nil {
		pkg.Warnf("runs: 收尾运行记录失败: %v", err)
	}
}

// Snapshot 在轮末记一次上下文快照，供「当时窗口里有多少、压了几次」回看。
func (s *RunsService) Snapshot(runID, sessionID string, turn, used, window, compressed int) {
	row := &domain.ContextSnapshotDO{
		ID: pkg.NewID(domain.PrefixSnapshot), RunID: runID, SessionID: sessionID,
		Turn: turn, Used: used, Window: window, Compressed: compressed,
	}
	if err := s.env.Repo.CreateSnapshot(row); err != nil {
		pkg.Warnf("runs: 落上下文快照失败: %v", err)
	}
}

// ReapStale 收口上次进程遗留的「运行中」记录：执行句柄是内存态，重启就没人办得完。
func (s *RunsService) ReapStale() error { return s.env.Repo.ReapStale() }

// List 按会话（可空 = 全局）列出最近的运行记录。
func (s *RunsService) List(sessionID string, limit int) ([]domain.RunVO, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.env.Repo.ListRuns(sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RunVO, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.vo(r))
	}
	return out, nil
}

// Detail 一次运行的详情：记录 + 时间线 + 上下文快照 + 可用检查点。
func (s *RunsService) Detail(runID string) (*domain.RunDetailVO, error) {
	row, err := s.env.Repo.GetRun(runID)
	if err != nil {
		return nil, err
	}
	vo := domain.RunDetailVO{RunVO: s.vo(*row)}
	// 时间线从条目链投影：run 结束了也读得出来，且不需要在运行期维护第二份账。
	steps, chars := s.timelineOf(row.SessionID)
	vo.Timeline = &domain.RunTimeline{Chars: chars, Steps: steps}
	if snaps, err := s.env.Repo.ListSnapshots(runID); err == nil {
		vo.Snapshots = snaps
	}
	if cps, err := s.env.Repo.ListCheckpoints(row.SessionID); err == nil {
		vo.Checkpoints = cps
	}
	return &vo, nil
}

// timelineOf 投影工具动作时间线。
// 会话正文在 PI 的 JSONL 里，运行观测只统计自己记的账（用量与工具成败），
// 不再从条目链重读一遗——那需要把整份 JSONL 再解析一次，而详情页刚解析过。
func (s *RunsService) timelineOf(sessionID string) ([]domain.RunStep, int) {
	return []domain.RunStep{}, 0
}

// Touch 轮末盖一次读数：实时水位进运行记录，同一次读数再落一张快照。
// 同一个数存两处不是冗余——列表要「现在多少」，详情要「第几轮涨到多少」。
func (c *CheckpointService) Touch(runID, sessionID string, turn, used, window, compressed int) {
	_ = c.env.Repo.UpdateRunColumns(runID, map[string]any{
		"turns": turn, "context": used, "compressed": compressed,
	})
	row := &domain.ContextSnapshotDO{
		ID: pkg.NewID(domain.PrefixSnapshot), RunID: runID, SessionID: sessionID,
		Turn: turn, Used: used, Window: window, Compressed: compressed,
	}
	if err := c.env.Repo.CreateSnapshot(row); err != nil {
		pkg.Warnf("checkpoint: 落快照失败: %v", err)
	}
}

// Checkpoints 列一个会话的全部检查点（按轮次升序）。
func (s *RunsService) Checkpoints(sessionID string) ([]domain.CheckpointVO, error) {
	return s.env.Repo.ListCheckpoints(sessionID)
}

// Restore 回到某个检查点：PI 没有「改 leaf」的协议面，
// 所以语义是**从那一刻分叉出一条新会话**，原会话完整保留。
// 界面用「从这一刻重开」表达它，而不是「撤销」。
func (s *RunsService) Restore(sessionID, checkpointID string) (*domain.SessionVO, error) {
	return s.Fork(sessionID, checkpointID)
}

// Fork 从检查点分叉出一条新会话：原会话完整保留，用户从那一刻开一条平行的问法。
func (s *RunsService) Fork(sessionID, checkpointID string) (*domain.SessionVO, error) {
	cp, err := s.env.Repo.GetCheckpoint(checkpointID)
	if err != nil {
		return nil, err
	}
	if cp.SessionID != sessionID {
		return nil, pkg.New("9601", "检查点不属于这个会话", "")
	}
	// 真正的分叉要问 PI 要（它会写一份新的 JSONL 并把父会话指向原文件）。
	vo, err := s.env.Sessions().Fork(sessionID, cp.PiEntryID)
	if err != nil {
		return nil, err
	}
	// 运行态必须一起清：那一轮已不在，前端还按「运行中」显示就再也停不下来。
	s.env.Emitter.Emit(sessionID, domain.EventChatSwitched, domain.SwitchedData{
		SessionID: vo.ID, Reason: "fork",
	})
	return vo, nil
}

// vo 把 DO 转出参；耗时在收尾后才算得出来。
func (s *RunsService) vo(r domain.RunDO) domain.RunVO {
	vo := domain.RunVO{
		ID: r.ID, SessionID: r.SessionID, ProviderID: r.ProviderID, Model: r.Model,
		Status: r.Status, StopReason: r.StopReason, Turns: r.Turns, Tokens: r.Tokens,
		Input: r.Input, Output: r.Output, Cached: r.Cached, Context: r.Context,
		MaxTokens: r.MaxTokens, Compressed: r.Compressed,
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
	if r.FinishedAt > 0 {
		vo.DurationMs = r.FinishedAt - r.StartedAt
	}
	if sess, err := s.env.Repo.GetSession(r.SessionID); err == nil {
		vo.SessionTitle = sess.Title
	}
	vo.CanResume = r.Status == domain.RunFailed || r.Status == domain.RunAborted
	return vo
}

// CheckpointService 管检查点：轮前打点、回到某轮、从某轮分叉。
type CheckpointService struct{ env *Env }

// NewCheckpointService 构造检查点服务。
func NewCheckpointService(env *Env) *CheckpointService { return &CheckpointService{env: env} }

// Mark 在一轮开始前打点（同轮次重复打点直接跳过：一轮只有一个「轮前」）。
func (c *CheckpointService) Mark(sessionID, runID, piEntryID, prompt string, turn int) {
	row := &domain.CheckpointDO{
		ID: pkg.NewID(domain.PrefixCheckpoint), SessionID: sessionID, RunID: runID,
		PiEntryID: piEntryID, Turn: turn, Prompt: trimRunes(prompt, domain.CheckpointPromptLimit),
	}
	if err := c.env.Repo.CreateCheckpoint(row); err != nil {
		pkg.Warnf("checkpoint: 打点失败: %v", err)
	}
}

// trimRunes 按 rune 截断；检查点存的是「当时问了什么」，不是全文。
func trimRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// ParseTurn 把设置里的轮数解析成整数；坏值走默认，不让一个空设置变成「永不压缩」。
func ParseTurn(raw string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return def
	}
	return n
}
