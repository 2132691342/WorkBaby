package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// UserCommandRepo user_commands 表 CRUD。
type UserCommandRepo struct{ db *gorm.DB }

// NewUserCommandRepo 构造仓储。
func NewUserCommandRepo(db *gorm.DB) *UserCommandRepo { return &UserCommandRepo{db: db} }

// List 全量按名称排序。
func (r *UserCommandRepo) List(ctx context.Context) ([]domain.UserCommandDO, error) {
	var rows []domain.UserCommandDO
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2050, "list user commands failed", err)
	}
	return rows, nil
}

// Upsert 按 name upsert（Unscoped 让软删过的同名可复用，语义与 SkillRepo 一致）。
func (r *UserCommandRepo) Upsert(ctx context.Context, row *domain.UserCommandDO) error {
	var existing domain.UserCommandDO
	err := r.db.WithContext(ctx).Unscoped().First(&existing, "name = ?", row.Name).Error
	if err == nil {
		row.ID = existing.ID
		if row.CreatedAt == 0 {
			row.CreatedAt = existing.CreatedAt
		}
		row.DeletedAt = gorm.DeletedAt{}
		if err := r.db.WithContext(ctx).Unscoped().Save(row).Error; err != nil {
			return pkg.Wrap(2051, "update user command failed", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return pkg.Wrap(2051, "query user command failed", err)
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2051, "create user command failed", err)
	}
	return nil
}

// Delete 按 name 软删。
func (r *UserCommandRepo) Delete(ctx context.Context, name string) error {
	res := r.db.WithContext(ctx).Where("name = ?", name).Delete(&domain.UserCommandDO{})
	if res.Error != nil {
		return pkg.Wrap(2051, "delete user command failed", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrUserCommandNotFound
	}
	return nil
}
