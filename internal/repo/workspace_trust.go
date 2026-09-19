package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkspaceTrustRepo workspace_trust 表 CRUD。
type WorkspaceTrustRepo struct{ db *gorm.DB }

func NewWorkspaceTrustRepo(db *gorm.DB) *WorkspaceTrustRepo { return &WorkspaceTrustRepo{db: db} }

// Get 按规范化路径取单条登记。
func (r *WorkspaceTrustRepo) Get(ctx context.Context, path string) (*domain.WorkspaceTrustDO, error) {
	var row domain.WorkspaceTrustDO
	if err := r.db.WithContext(ctx).First(&row, "path = ?", path).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未登记不是错误：默认 ask
		}
		return nil, pkg.Wrap(2070, "get workspace trust failed", err)
	}
	return &row, nil
}

// List 全部登记（按更新时间倒序）。
func (r *WorkspaceTrustRepo) List(ctx context.Context) ([]domain.WorkspaceTrustDO, error) {
	var rows []domain.WorkspaceTrustDO
	if err := r.db.WithContext(ctx).Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2071, "list workspace trust failed", err)
	}
	return rows, nil
}

// Upsert 原子写入信任决策（主键冲突即更新 state/updated_at）。
//
// 信任登记是安全决策，必须原子落盘：先写临时值再改会让并发工具调用读到中间态。
func (r *WorkspaceTrustRepo) Upsert(ctx context.Context, row *domain.WorkspaceTrustDO) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return pkg.Wrap(2072, "save workspace trust failed", err)
	}
	return nil
}

// Delete 撤销某目录的信任登记（回到默认 ask）。
func (r *WorkspaceTrustRepo) Delete(ctx context.Context, path string) error {
	if err := r.db.WithContext(ctx).Where("path = ?", path).
		Delete(&domain.WorkspaceTrustDO{}).Error; err != nil {
		return pkg.Wrap(2073, "delete workspace trust failed", err)
	}
	return nil
}
