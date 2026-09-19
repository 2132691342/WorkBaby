package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// SkillRepo skills 表 CRUD。
type SkillRepo struct{ db *gorm.DB }

// NewSkillRepo 构造仓储。
func NewSkillRepo(db *gorm.DB) *SkillRepo { return &SkillRepo{db: db} }

// List 全量（含 disabled，按名称排序）。
func (r *SkillRepo) List(ctx context.Context) ([]domain.SkillDO, error) {
	var rows []domain.SkillDO
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2050, "list skills failed", err)
	}
	return rows, nil
}

// GetByName 按唯一名查找。
func (r *SkillRepo) GetByName(ctx context.Context, name string) (*domain.SkillDO, error) {
	var row domain.SkillDO
	if err := r.db.WithContext(ctx).First(&row, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSkillNotFound
		}
		return nil, pkg.Wrap(2050, "get skill failed", err)
	}
	return &row, nil
}

// Upsert 按 name upsert（builtin 重装 / custom 更新）。
// 用「先查后写」而非 FirstOrCreate：内置 Skill 每次启动都带新 ULID，
// dest 自带主键会被 FirstOrCreate 带进查询条件 → 查不到 → 走 INSERT → 撞 name 唯一索引（二次启动必崩）。
// Unscoped 让软删过的同名 Skill 可复用（等于取消软删）。
func (r *SkillRepo) Upsert(ctx context.Context, row *domain.SkillDO) error {
	// 每步都开新 session：GORM 的 session 会携带上一步的 ErrRecordNotFound
	var existing domain.SkillDO
	err := r.db.WithContext(ctx).Unscoped().First(&existing, "name = ?", row.Name).Error
	if err == nil {
		row.ID = existing.ID
		if row.CreatedAt == 0 {
			row.CreatedAt = existing.CreatedAt
		}
		row.DeletedAt = gorm.DeletedAt{}
		if err := r.db.WithContext(ctx).Unscoped().Save(row).Error; err != nil {
			return pkg.Wrap(2051, "update skill failed", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return pkg.Wrap(2051, "query skill failed", err)
	}
	if row.ID == "" {
		row.ID = pkg.NewID(domain.IDSkill)
	}
	if err := r.db.WithContext(ctx).Unscoped().Create(row).Error; err != nil {
		return pkg.Wrap(2051, "insert skill failed", err)
	}
	return nil
}

// Update 更新自定义 Skill（custom 来源）。
func (r *SkillRepo) Update(ctx context.Context, row *domain.SkillDO) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return pkg.Wrap(2051, "update skill failed", err)
	}
	return nil
}

// Delete 软删。
func (r *SkillRepo) Delete(ctx context.Context, name string) error {
	if err := r.db.WithContext(ctx).
		Where("name = ?", name).
		Delete(&domain.SkillDO{}).Error; err != nil {
		return pkg.Wrap(2053, "delete skill failed", err)
	}
	return nil
}
