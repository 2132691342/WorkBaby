package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// RunRecordRepo 运行历史索引持久化。
type RunRecordRepo struct{ db *gorm.DB }

// NewRunRecordRepo 构造。
func NewRunRecordRepo(db *gorm.DB) *RunRecordRepo { return &RunRecordRepo{db: db} }

// Start 记录 run 启动；同 runID 重复调用（续跑）保留首条，不覆盖启动时间。
func (r *RunRecordRepo) Start(ctx context.Context, row domain.RunRecordDO) error {
	if row.RunID == "" {
		return nil
	}
	if err := r.db.WithContext(ctx).FirstOrCreate(&row).Error; err != nil {
		return pkg.Wrap(2010, "save run record failed", err)
	}
	return nil
}

// Finish 回填终态（状态 / 终因 / 用量 / 结束时间）。
func (r *RunRecordRepo) Finish(ctx context.Context, runID string, upd map[string]any) error {
	if runID == "" || len(upd) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&domain.RunRecordDO{}).
		Where("run_id = ?", runID).Updates(upd).Error; err != nil {
		return pkg.Wrap(2010, "update run record failed", err)
	}
	return nil
}

// List 运行历史倒序分页；sessionID 为空表示不过滤。
func (r *RunRecordRepo) List(ctx context.Context, sessionID string, limit, offset int) ([]domain.RunRecordDO, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := r.db.WithContext(ctx).Model(&domain.RunRecordDO{})
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, pkg.Wrap(2010, "count run records failed", err)
	}
	var rows []domain.RunRecordDO
	if err := q.Order("started_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, pkg.Wrap(2010, "list run records failed", err)
	}
	return rows, total, nil
}

// DeleteBySession 会话删除时级联清理运行历史。
func (r *RunRecordRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Delete(&domain.RunRecordDO{}).Error; err != nil {
		return pkg.Wrap(2010, "delete run records failed", err)
	}
	return nil
}
