package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// McpServerRepo mcp_servers 表 CRUD。
type McpServerRepo struct{ db *gorm.DB }

// NewMcpServerRepo 构造仓储。
func NewMcpServerRepo(db *gorm.DB) *McpServerRepo { return &McpServerRepo{db: db} }

// List 全量（含 disabled，按名称排序）。
func (r *McpServerRepo) List(ctx context.Context) ([]domain.McpServerDO, error) {
	var rows []domain.McpServerDO
	if err := r.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2070, "list mcp servers failed", err)
	}
	return rows, nil
}

// GetByName 按唯一名查找。
func (r *McpServerRepo) GetByName(ctx context.Context, name string) (*domain.McpServerDO, error) {
	var row domain.McpServerDO
	if err := r.db.WithContext(ctx).First(&row, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMcpServerNotFound
		}
		return nil, pkg.Wrap(2070, "get mcp server failed", err)
	}
	return &row, nil
}

// Upsert 按 name upsert（新增 / 改配置）。
// 用「先查后写」而非 FirstOrCreate：dest 自带新主键时后者会把主键带进查询条件，
// 导致查不到 → 走 INSERT → 撞 name 唯一索引。Unscoped 让软删过的同名配置可复用（等于取消软删）。
func (r *McpServerRepo) Upsert(ctx context.Context, row *domain.McpServerDO) error {
	// 每步都开新 session：GORM 的 session 会携带上一步的 ErrRecordNotFound
	var existing domain.McpServerDO
	err := r.db.WithContext(ctx).Unscoped().First(&existing, "name = ?", row.Name).Error
	if err == nil {
		row.ID = existing.ID
		if row.CreatedAt == 0 {
			row.CreatedAt = existing.CreatedAt
		}
		row.DeletedAt = gorm.DeletedAt{}
		if err := r.db.WithContext(ctx).Unscoped().Save(row).Error; err != nil {
			return pkg.Wrap(2071, "update mcp server failed", err)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return pkg.Wrap(2071, "query mcp server failed", err)
	}
	if row.ID == "" {
		row.ID = pkg.NewID(domain.IDMcpServer)
	}
	if err := r.db.WithContext(ctx).Unscoped().Create(row).Error; err != nil {
		return pkg.Wrap(2071, "insert mcp server failed", err)
	}
	return nil
}

// Update 更新行（启停 / 运行时回写 ToolCount）。
func (r *McpServerRepo) Update(ctx context.Context, row *domain.McpServerDO) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return pkg.Wrap(2071, "update mcp server failed", err)
	}
	return nil
}

// Delete 软删。
func (r *McpServerRepo) Delete(ctx context.Context, name string) error {
	if err := r.db.WithContext(ctx).
		Where("name = ?", name).
		Delete(&domain.McpServerDO{}).Error; err != nil {
		return pkg.Wrap(2073, "delete mcp server failed", err)
	}
	return nil
}
