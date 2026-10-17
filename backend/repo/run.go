// 运行观测与检查点的持久层：run 记账、上下文快照、检查点与隐藏会话回收。
package repo

import (
	"WorkBaby/backend/domain"
)

// CreateRun 落一行运行记录。
func (r *Repo) CreateRun(row *domain.RunDO) error {
	return wrapDB("runs.create", r.db.Create(row).Error)
}

// UpdateRunColumns 按列覆盖写：收尾与轮末读数都是部分字段，不整行覆盖。
func (r *Repo) UpdateRunColumns(id string, cols map[string]any) error {
	return wrapDB("runs.update", r.db.Model(&domain.RunDO{}).Where("id = ?", id).Updates(cols).Error)
}

// GetRun 取一次运行记录。
func (r *Repo) GetRun(id string) (*domain.RunDO, error) {
	var row domain.RunDO
	if err := r.db.Where("id = ?", id).First(&row).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrRunNotFound
		}
		return nil, wrapDB("runs.get", err)
	}
	return &row, nil
}

// ListRuns 按会话（可空 = 全局）列最近的运行记录。
func (r *Repo) ListRuns(sessionID string, limit int) ([]domain.RunDO, error) {
	q := r.db.Order("started_at DESC").Limit(limit)
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	var rows []domain.RunDO
	if err := q.Find(&rows).Error; err != nil {
		return nil, wrapDB("runs.list", err)
	}
	return rows, nil
}

// ReapStale 收口上次进程遗留的 running 行：执行句柄是内存态，重启后没人办得完。
func (r *Repo) ReapStale() error {
	return wrapDB("runs.reap", r.db.Model(&domain.RunDO{}).
		Where("status = ?", domain.RunRunning).
		Updates(map[string]any{"status": domain.RunAborted, "stop_reason": "interrupted"}).Error)
}

// CreateSnapshot 落一次上下文快照。
func (r *Repo) CreateSnapshot(row *domain.ContextSnapshotDO) error {
	return wrapDB("snapshots.create", r.db.Create(row).Error)
}

// ListSnapshots 按轮次升序列出一个 run 的快照。
func (r *Repo) ListSnapshots(runID string) ([]domain.ContextSnapshotDO, error) {
	rows := []domain.ContextSnapshotDO{}
	if err := r.db.Where("run_id = ?", runID).Order("turn ASC").Find(&rows).Error; err != nil {
		return nil, wrapDB("snapshots.list", err)
	}
	return rows, nil
}

// CreateCheckpoint 打一个检查点（同轮次重复打点直接跳过）。
func (r *Repo) CreateCheckpoint(row *domain.CheckpointDO) error {
	var n int64
	if err := r.db.Model(&domain.CheckpointDO{}).
		Where("session_id = ? AND turn = ?", row.SessionID, row.Turn).
		Count(&n).Error; err != nil {
		return wrapDB("checkpoints.count", err)
	}
	if n > 0 {
		return nil
	}
	return wrapDB("checkpoints.create", r.db.Create(row).Error)
}

// GetCheckpoint 取一个检查点。
func (r *Repo) GetCheckpoint(id string) (*domain.CheckpointDO, error) {
	var row domain.CheckpointDO
	if err := r.db.Where("id = ?", id).First(&row).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrCheckpointNotFound
		}
		return nil, wrapDB("checkpoints.get", err)
	}
	return &row, nil
}

// ListCheckpoints 按轮次升序列出一个会话的检查点。
func (r *Repo) ListCheckpoints(sessionID string) ([]domain.CheckpointVO, error) {
	rows := []domain.CheckpointDO{}
	if err := r.db.Where("session_id = ?", sessionID).Order("turn ASC").Find(&rows).Error; err != nil {
		return nil, wrapDB("checkpoints.list", err)
	}
	out := make([]domain.CheckpointVO, 0, len(rows))
	for _, c := range rows {
		out = append(out, domain.CheckpointVO{
			ID: c.ID, SessionID: c.SessionID, RunID: c.RunID, PiEntryID: c.PiEntryID,
			Turn: c.Turn, Prompt: c.Prompt, CreatedAt: c.CreatedAt,
		})
	}
	return out, nil
}

// ForkSessionAt 建一条分叉会话的记录行：真正的 JSONL 由上层调 PI 的 fork 生成后回写。
// PI 的 leaf 是内部状态，不能在持久层改。
func (r *Repo) ForkSessionAt(src *domain.SessionDO, title string) (*domain.SessionDO, error) {
	forked := &domain.SessionDO{
		ID:         newForkID(),
		Title:      title,
		Workspace:  src.Workspace,
		ProviderID: src.ProviderID,
		Model:      src.Model,
		Permission: src.Permission,
	}
	if err := r.CreateSession(forked); err != nil {
		return nil, err
	}
	return forked, nil
}

// ListExpiredHiddenSessions 列出一批过期的隐藏会话 id（updated_at 早于 cutoff）。
func (r *Repo) ListExpiredHiddenSessions(cutoff int64, limit int) ([]string, error) {
	q := r.db.Model(&domain.SessionDO{}).
		Where("hidden = ? AND updated_at < ?", true, cutoff).
		Order("updated_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var ids []string
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, wrapDB("sessions.expired", err)
	}
	return ids, nil
}

// ArchiveSessions 删掉若干隐藏会话的逐字对话链与会话行：用量账与运行记录不动。
func (r *Repo) ArchiveSessions(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	err := r.WithTx(func(tx *Repo) error {
		if err := tx.db.Where("session_id IN ?", ids).Delete(&domain.ApprovalDO{}).Error; err != nil {
			return wrapDB("approvals.archive", err)
		}
		res := tx.db.Where("id IN ?", ids).Delete(&domain.SessionDO{})
		if res.Error != nil {
			return wrapDB("sessions.archive", res.Error)
		}
		n = res.RowsAffected
		return nil
	})
	return n, err
}

// DetachRunSessions 把运行记录的会话引用摘空：入口消失，而不是留着让用户点了拿 404。
func (r *Repo) DetachRunSessions(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.Model(&domain.RunDO{}).Where("session_id IN ?", ids).Update("session_id", "")
	if res.Error != nil {
		return 0, wrapDB("runs.detach", res.Error)
	}
	return res.RowsAffected, nil
}
