package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// AiProviderRepo ai_providers 表 CRUD。
type AiProviderRepo struct{ db *gorm.DB }

func NewAiProviderRepo(db *gorm.DB) *AiProviderRepo { return &AiProviderRepo{db: db} }

func (r *AiProviderRepo) Create(ctx context.Context, p *domain.AiProviderDO) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return pkg.Wrap(2030, "create provider failed", err)
	}
	return nil
}

func (r *AiProviderRepo) Update(ctx context.Context, p *domain.AiProviderDO) error {
	if err := r.db.WithContext(ctx).Save(p).Error; err != nil {
		return pkg.Wrap(2031, "update provider failed", err)
	}
	return nil
}

func (r *AiProviderRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.AiProviderDO{}, "id = ?", id).Error; err != nil {
		return pkg.Wrap(2032, "delete provider failed", err)
	}
	return nil
}

func (r *AiProviderRepo) GetByID(ctx context.Context, id string) (*domain.AiProviderDO, error) {
	var p domain.AiProviderDO
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProviderNotFound
		}
		return nil, pkg.Wrap(2033, "get provider failed", err)
	}
	return &p, nil
}

func (r *AiProviderRepo) List(ctx context.Context) ([]domain.AiProviderDO, error) {
	var ps []domain.AiProviderDO
	if err := r.db.WithContext(ctx).Order("created_at").Find(&ps).Error; err != nil {
		return nil, pkg.Wrap(2034, "list providers failed", err)
	}
	return ps, nil
}

func (r *AiProviderRepo) ListEnabled(ctx context.Context) ([]domain.AiProviderDO, error) {
	var ps []domain.AiProviderDO
	// CASE 而非裸 tier：字符串序会让 backup 排在 primary 之前。
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).
		Order("CASE WHEN tier = 'primary' THEN 0 ELSE 1 END, created_at").Find(&ps).Error; err != nil {
		return nil, pkg.Wrap(2034, "list enabled providers failed", err)
	}
	return ps, nil
}
