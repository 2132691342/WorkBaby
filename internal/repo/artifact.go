package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ArtifactRepo 工件持久化（artifacts 表）。
type ArtifactRepo struct{ db *gorm.DB }

// NewArtifactRepo 构造。
func NewArtifactRepo(db *gorm.DB) *ArtifactRepo { return &ArtifactRepo{db: db} }

// Upsert 按 (session_id, rel_path) 幂等登记；已存在则更新体积/时间/来源 run。
func (r *ArtifactRepo) Upsert(ctx context.Context, row *domain.ArtifactDO) error {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "session_id"}, {Name: "rel_path"}},
		DoUpdates: clause.AssignmentColumns([]string{"run_id", "kind", "name", "mime_type", "size", "updated_at"}),
	}).Create(row).Error; err != nil {
		return pkg.Wrap(2017, "upsert artifact failed", err)
	}
	return nil
}

// ListBySession 会话工件（最近更新在前）。
func (r *ArtifactRepo) ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.ArtifactDO, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []domain.ArtifactDO
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2017, "list artifacts failed", err)
	}
	return rows, nil
}

// GetByID 单条工件。
func (r *ArtifactRepo) GetByID(ctx context.Context, id string) (*domain.ArtifactDO, error) {
	var row domain.ArtifactDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrArtifactNotFound
		}
		return nil, pkg.Wrap(2017, "get artifact failed", err)
	}
	return &row, nil
}

// Delete 删除登记（不删磁盘文件：工件是引用，删引用不该销毁用户数据）。
func (r *ArtifactRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).Delete(&domain.ArtifactDO{}).Error; err != nil {
		return pkg.Wrap(2017, "delete artifact failed", err)
	}
	return nil
}

// DeleteBySession 会话删除级联清理。
func (r *ArtifactRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&domain.ArtifactDO{}).Error; err != nil {
		return pkg.Wrap(2017, "delete artifacts failed", err)
	}
	return nil
}
