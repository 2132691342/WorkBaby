package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	"gorm.io/gorm"
)

// ChatTaskRepo 后台任务持久化。
type ChatTaskRepo struct{ db *gorm.DB }

// NewChatTaskRepo 构造。
func NewChatTaskRepo(db *gorm.DB) *ChatTaskRepo { return &ChatTaskRepo{db: db} }

// Create 落一条 pending 任务。
func (r *ChatTaskRepo) Create(ctx context.Context, row *domain.ChatTaskDO) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2020, "create chat task failed", err)
	}
	return nil
}

// UpdateFields 按列更新（状态机迁移统一走这里，防整行覆盖竞态）。
func (r *ChatTaskRepo) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&domain.ChatTaskDO{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return pkg.Wrap(2020, "update chat task failed", err)
	}
	return nil
}

// GetByID 取单条。
func (r *ChatTaskRepo) GetByID(ctx context.Context, id string) (*domain.ChatTaskDO, error) {
	var row domain.ChatTaskDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, pkg.Wrap(2020, "get chat task failed", err)
	}
	return &row, nil
}

// ListRecent 最新任务在前（任务中心列表）。
func (r *ChatTaskRepo) ListRecent(ctx context.Context, limit int) ([]domain.ChatTaskDO, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []domain.ChatTaskDO
	var total int64
	if err := r.db.WithContext(ctx).Model(&domain.ChatTaskDO{}).Count(&total).Error; err != nil {
		return nil, 0, pkg.Wrap(2020, "count chat tasks failed", err)
	}
	if err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, pkg.Wrap(2020, "list chat tasks failed", err)
	}
	return rows, total, nil
}

// ListUnfinished 启动恢复用：取全部未终态任务。
func (r *ChatTaskRepo) ListUnfinished(ctx context.Context) ([]domain.ChatTaskDO, error) {
	var rows []domain.ChatTaskDO
	if err := r.db.WithContext(ctx).
		Where("state IN ?", []string{domain.TaskStatePending, domain.TaskStateRunning}).
		Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2020, "list unfinished chat tasks failed", err)
	}
	return rows, nil
}
