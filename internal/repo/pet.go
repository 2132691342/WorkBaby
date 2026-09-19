package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// PetConfigRepo pet_configs 表单行配置。
type PetConfigRepo struct{ db *gorm.DB }

// NewPetConfigRepo 构造仓储。
func NewPetConfigRepo(db *gorm.DB) *PetConfigRepo { return &PetConfigRepo{db: db} }

// Get 读取单行；不存在返回 ErrPetConfigNotFound。
func (r *PetConfigRepo) Get(ctx context.Context, userID string) (*domain.PetConfigDO, error) {
	var row domain.PetConfigDO
	if err := r.db.WithContext(ctx).First(&row, "user_id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPetConfigNotFound
		}
		return nil, pkg.Wrap(2060, "get pet config failed", err)
	}
	return &row, nil
}

// Upsert 保存（单行幂等）。
func (r *PetConfigRepo) Upsert(ctx context.Context, row *domain.PetConfigDO) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return pkg.Wrap(2061, "save pet config failed", err)
	}
	return nil
}

// PetSpriteRepo pet_sprites 表 CRUD（软删）。
type PetSpriteRepo struct{ db *gorm.DB }

// NewPetSpriteRepo 构造仓储。
func NewPetSpriteRepo(db *gorm.DB) *PetSpriteRepo { return &PetSpriteRepo{db: db} }

// List 全部（内置优先，按创建时间倒序）。
func (r *PetSpriteRepo) List(ctx context.Context) ([]domain.PetSpriteDO, error) {
	var rows []domain.PetSpriteDO
	if err := r.db.WithContext(ctx).Order("is_builtin DESC, created_at DESC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2062, "list pet sprites failed", err)
	}
	return rows, nil
}

// GetByID 按 ID 查找；找不到返回 ErrPetSpriteNotFound。
func (r *PetSpriteRepo) GetByID(ctx context.Context, id string) (*domain.PetSpriteDO, error) {
	var row domain.PetSpriteDO
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPetSpriteNotFound
		}
		return nil, pkg.Wrap(2062, "get pet sprite failed", err)
	}
	return &row, nil
}

// Create 新建。
func (r *PetSpriteRepo) Create(ctx context.Context, row *domain.PetSpriteDO) error {
	if row.ID == "" {
		row.ID = pkg.NewID(domain.IDSprite)
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2063, "create pet sprite failed", err)
	}
	return nil
}

// Delete 软删。
func (r *PetSpriteRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.PetSpriteDO{}, "id = ?", id).Error; err != nil {
		return pkg.Wrap(2064, "delete pet sprite failed", err)
	}
	return nil
}
