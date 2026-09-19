package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// FileChangeRepo 文件变更持久化（file_changes 表）。
type FileChangeRepo struct{ db *gorm.DB }

// NewFileChangeRepo 构造。
func NewFileChangeRepo(db *gorm.DB) *FileChangeRepo { return &FileChangeRepo{db: db} }

// Create 写入一条变更记录。
func (r *FileChangeRepo) Create(ctx context.Context, row *domain.FileChangeDO) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2016, "create file change failed", err)
	}
	return nil
}

// ListBySession 会话变更列表（最新在前）。
func (r *FileChangeRepo) ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.FileChangeDO, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []domain.FileChangeDO
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2016, "list file changes failed", err)
	}
	return rows, nil
}

// ListByRun 单次 run 的变更（一轮任务的产物清单）。
func (r *FileChangeRepo) ListByRun(ctx context.Context, runID string) ([]domain.FileChangeDO, error) {
	var rows []domain.FileChangeDO
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2016, "list file changes by run failed", err)
	}
	return rows, nil
}

// GetByID 单条变更。
func (r *FileChangeRepo) GetByID(ctx context.Context, id string) (*domain.FileChangeDO, error) {
	var row domain.FileChangeDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrFileChangeNotFound
		}
		return nil, pkg.Wrap(2016, "get file change failed", err)
	}
	return &row, nil
}

// MarkRolledBack 标记已回滚（幂等覆盖）。
func (r *FileChangeRepo) MarkRolledBack(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&domain.FileChangeDO{}).
		Where("id = ?", id).Update("rolled_back", true).Error; err != nil {
		return pkg.Wrap(2016, "mark file change rolled back failed", err)
	}
	return nil
}

// DeleteBySession 会话删除级联清理。
func (r *FileChangeRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&domain.FileChangeDO{}).Error; err != nil {
		return pkg.Wrap(2016, "delete file changes failed", err)
	}
	return nil
}
