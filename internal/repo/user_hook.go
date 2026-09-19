// Package repo UserHookRepo user_hooks 表 CRUD。
package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// UserHookRepo user_hooks 表仓储。
type UserHookRepo struct{ db *gorm.DB }

// NewUserHookRepo 构造仓储。
func NewUserHookRepo(db *gorm.DB) *UserHookRepo { return &UserHookRepo{db: db} }

// List 全量按事件与排序号排序。
func (r *UserHookRepo) List(ctx context.Context) ([]domain.UserHookDO, error) {
	var rows []domain.UserHookDO
	if err := r.db.WithContext(ctx).Order("event, sort, created_at").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2050, "list user hooks failed", err)
	}
	return rows, nil
}

// GetByID 单条。
func (r *UserHookRepo) GetByID(ctx context.Context, id string) (*domain.UserHookDO, error) {
	var row domain.UserHookDO
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrHookNotFound
		}
		return nil, pkg.Wrap(2050, "get user hook failed", err)
	}
	return &row, nil
}

// Upsert 按 ID upsert；新产行硬删重建（防 GORM 空主键退化 INSERT 撞唯一索引）。
func (r *UserHookRepo) Upsert(ctx context.Context, row *domain.UserHookDO) error {
	var existing domain.UserHookDO
	err := r.db.WithContext(ctx).Unscoped().First(&existing, "id = ?", row.ID).Error
	if err == nil {
		if row.CreatedAt == 0 {
			row.CreatedAt = existing.CreatedAt
		}
		row.DeletedAt = gorm.DeletedAt{}
		if err := r.db.WithContext(ctx).Unscoped().Save(row).Error; err != nil {
			return pkg.Wrap(2051, "update user hook failed", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return pkg.Wrap(2051, "query user hook failed", err)
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2051, "create user hook failed", err)
	}
	return nil
}

// Delete 按 ID 软删。
func (r *UserHookRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&domain.UserHookDO{}, "id = ?", id)
	if res.Error != nil {
		return pkg.Wrap(2051, "delete user hook failed", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrHookNotFound
	}
	return nil
}
