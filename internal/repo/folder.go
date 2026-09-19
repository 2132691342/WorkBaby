package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// FolderRepo folders 表 CRUD（硬删除；删除前由 service 校验子目录为空）。
type FolderRepo struct{ db *gorm.DB }

func NewFolderRepo(db *gorm.DB) *FolderRepo { return &FolderRepo{db: db} }

// Create 新建。
func (r *FolderRepo) Create(ctx context.Context, d *domain.FolderDO) error {
	if d.ID == "" {
		d.ID = pkg.NewID(domain.IDFolder)
	}
	if err := r.db.WithContext(ctx).Create(d).Error; err != nil {
		return pkg.Wrap(2030, "create folder failed", err)
	}
	return nil
}

// GetByID 按 ID 查询；找不到返回 ErrFolderNotFound。
func (r *FolderRepo) GetByID(ctx context.Context, id string) (*domain.FolderDO, error) {
	var d domain.FolderDO
	if err := r.db.WithContext(ctx).First(&d, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrFolderNotFound
		}
		return nil, pkg.Wrap(2031, "get folder failed", err)
	}
	return &d, nil
}

// ListByParent 列出某父目录下（parentId 为空 = 根目录）的子文件夹。
func (r *FolderRepo) ListByParent(ctx context.Context, parentID *string) ([]domain.FolderDO, error) {
	q := r.db.WithContext(ctx)
	if parentID == nil || *parentID == "" {
		q = q.Where("parent_id IS NULL OR parent_id = ''")
	} else {
		q = q.Where("parent_id = ?", *parentID)
	}
	var rows []domain.FolderDO
	if err := q.Order("name").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2032, "list folders by parent failed", err)
	}
	return rows, nil
}

// ListAll 全部文件夹（树构建用）。
func (r *FolderRepo) ListAll(ctx context.Context) ([]domain.FolderDO, error) {
	var rows []domain.FolderDO
	if err := r.db.WithContext(ctx).Order("path").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2032, "list all folders failed", err)
	}
	return rows, nil
}

// ListByWorkspace 列出绑定到指定 workspace 的文件夹（workspaceId 空 = 全局）。
func (r *FolderRepo) ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.FolderDO, error) {
	q := r.db.WithContext(ctx)
	if workspaceID == "" {
		q = q.Where("workspace_id IS NULL OR workspace_id = ''")
	} else {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	var rows []domain.FolderDO
	if err := q.Order("path").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2032, "list folders by workspace failed", err)
	}
	return rows, nil
}

// CountChildren 统计直接子目录数。
func (r *FolderRepo) CountChildren(ctx context.Context, parentID string) (int, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&domain.FolderDO{})
	if parentID == "" {
		q = q.Where("parent_id IS NULL OR parent_id = ''")
	} else {
		q = q.Where("parent_id = ?", parentID)
	}
	if err := q.Count(&n).Error; err != nil {
		return 0, pkg.Wrap(2033, "count folder children failed", err)
	}
	return int(n), nil
}

// Update 保存整行。
func (r *FolderRepo) Update(ctx context.Context, d *domain.FolderDO) error {
	if err := r.db.WithContext(ctx).Save(d).Error; err != nil {
		return pkg.Wrap(2034, "update folder failed", err)
	}
	return nil
}

// Delete 硬删除。
func (r *FolderRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.FolderDO{}, "id = ?", id).Error; err != nil {
		return pkg.Wrap(2035, "delete folder failed", err)
	}
	return nil
}
