package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// AgentProfileRepo agent_profiles 表 CRUD。
type AgentProfileRepo struct{ db *gorm.DB }

// NewAgentProfileRepo 构造仓储。
func NewAgentProfileRepo(db *gorm.DB) *AgentProfileRepo { return &AgentProfileRepo{db: db} }

// List 全量（含 disabled，按名称排序）。
func (r *AgentProfileRepo) List(ctx context.Context) ([]domain.AgentProfileDO, error) {
	var rows []domain.AgentProfileDO
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2050, "list agent profiles failed", err)
	}
	return rows, nil
}

// ListEnabled 启用中的自定义 Agent（启动同步进 harness 注册表用）。
func (r *AgentProfileRepo) ListEnabled(ctx context.Context) ([]domain.AgentProfileDO, error) {
	var rows []domain.AgentProfileDO
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2050, "list enabled agent profiles failed", err)
	}
	return rows, nil
}

// GetByName 按唯一名查找；不存在返回 ErrAgentProfileNotFound。
func (r *AgentProfileRepo) GetByName(ctx context.Context, name string) (*domain.AgentProfileDO, error) {
	var row domain.AgentProfileDO
	if err := r.db.WithContext(ctx).First(&row, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAgentProfileNotFound
		}
		return nil, pkg.Wrap(2050, "get agent profile failed", err)
	}
	return &row, nil
}

// Upsert 按 name upsert（Unscoped 让软删过的同名可复用，语义与 SkillRepo 一致）。
// ID 为空的行（主键缺失）不可 Save——按主键更新退化为 INSERT 必撞 name 唯一索引，
// 遇到则整行硬删后走新建。
func (r *AgentProfileRepo) Upsert(ctx context.Context, row *domain.AgentProfileDO) error {
	var existing domain.AgentProfileDO
	err := r.db.WithContext(ctx).Unscoped().First(&existing, "name = ?", row.Name).Error
	if err == nil && existing.ID == "" {
		if err := r.db.WithContext(ctx).Unscoped().Where("name = ? AND id = ''", row.Name).
			Delete(&domain.AgentProfileDO{}).Error; err != nil {
			return pkg.Wrap(2051, "purge broken agent profile failed", err)
		}
		err = gorm.ErrRecordNotFound
	}
	if err == nil {
		row.ID = existing.ID
		if row.CreatedAt == 0 {
			row.CreatedAt = existing.CreatedAt
		}
		row.DeletedAt = gorm.DeletedAt{}
		if err := r.db.WithContext(ctx).Unscoped().Save(row).Error; err != nil {
			return pkg.Wrap(2051, "update agent profile failed", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return pkg.Wrap(2051, "query agent profile failed", err)
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2051, "create agent profile failed", err)
	}
	return nil
}

// SetEnabled 启停（按 name）。
func (r *AgentProfileRepo) SetEnabled(ctx context.Context, name string, enabled bool) error {
	res := r.db.WithContext(ctx).Model(&domain.AgentProfileDO{}).Where("name = ?", name).
		Update("enabled", enabled)
	if res.Error != nil {
		return pkg.Wrap(2051, "set agent profile enabled failed", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrAgentProfileNotFound
	}
	return nil
}

// Delete 按 name 软删；不存在返回 ErrAgentProfileNotFound。
func (r *AgentProfileRepo) Delete(ctx context.Context, name string) error {
	res := r.db.WithContext(ctx).Where("name = ?", name).Delete(&domain.AgentProfileDO{})
	if res.Error != nil {
		return pkg.Wrap(2051, "delete agent profile failed", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrAgentProfileNotFound
	}
	return nil
}
