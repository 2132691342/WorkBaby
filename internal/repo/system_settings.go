// Package repo 是 GORM 持久化层；禁止引 service / api / 能力域（CLAUDE §2.2）。
package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// SystemSettingRepo system_settings 表 CRUD。
type SystemSettingRepo struct{ db *gorm.DB }

func NewSystemSettingRepo(db *gorm.DB) *SystemSettingRepo { return &SystemSettingRepo{db: db} }

func (r *SystemSettingRepo) Get(ctx context.Context, k string) (*domain.SystemSettingDO, error) {
	if k == "" {
		return nil, domain.ErrSettingKeyEmpty
	}
	var row domain.SystemSettingDO
	if err := r.db.WithContext(ctx).First(&row, "k = ?", k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSettingNotFound
		}
		return nil, pkg.Wrap(2050, "select setting failed", err)
	}
	return &row, nil
}

func (r *SystemSettingRepo) Set(ctx context.Context, k, v string) error {
	if k == "" {
		return domain.ErrSettingKeyEmpty
	}
	row := domain.SystemSettingDO{K: k, V: v}
	if err := r.db.WithContext(ctx).Save(&row).Error; err != nil {
		return pkg.Wrap(2051, "save setting failed", err)
	}
	return nil
}

// ListAll 用于设置面板展示（KV 完整快照）。
func (r *SystemSettingRepo) ListAll(ctx context.Context) ([]domain.SystemSettingDO, error) {
	var rows []domain.SystemSettingDO
	if err := r.db.WithContext(ctx).Order("k").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2052, "list settings failed", err)
	}
	return rows, nil
}
