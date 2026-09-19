package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// FileRepo files 表 CRUD（内容在磁盘 {home}/files/{id}，DB 只存元数据）。
type FileRepo struct{ db *gorm.DB }

func NewFileRepo(db *gorm.DB) *FileRepo { return &FileRepo{db: db} }

// Create 新建。
func (r *FileRepo) Create(ctx context.Context, f *domain.FileDO) error {
	if f.ID == "" {
		f.ID = pkg.NewID(domain.IDFile)
	}
	if err := r.db.WithContext(ctx).Create(f).Error; err != nil {
		return pkg.Wrap(2036, "create file failed", err)
	}
	return nil
}

// GetByID 按 ID 查询；找不到返回 ErrFileNotFound。
func (r *FileRepo) GetByID(ctx context.Context, id string) (*domain.FileDO, error) {
	var f domain.FileDO
	if err := r.db.WithContext(ctx).First(&f, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrFileNotFound
		}
		return nil, pkg.Wrap(2037, "get file failed", err)
	}
	return &f, nil
}

// SearchByName 按文件名模糊搜索（归属用户），创建时间倒序。
func (r *FileRepo) SearchByName(ctx context.Context, userID, q string) ([]domain.FileDO, error) {
	like := "%" + q + "%"
	var rows []domain.FileDO
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND (name LIKE ? OR original_name LIKE ?)", userID, like, like).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2038, "search files failed", err)
	}
	return rows, nil
}

// ListByUser 某用户全部文件（创建时间倒序）。
func (r *FileRepo) ListByUser(ctx context.Context, userID string) ([]domain.FileDO, error) {
	var rows []domain.FileDO
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2038, "list files failed", err)
	}
	return rows, nil
}

// Delete 硬删除（磁盘文件由 service 级联清理）。
func (r *FileRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.FileDO{}, "id = ?", id).Error; err != nil {
		return pkg.Wrap(2039, "delete file failed", err)
	}
	return nil
}
