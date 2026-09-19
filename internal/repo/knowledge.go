package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// KnowledgeDocRepo knowledge_docs 表 CRUD。
type KnowledgeDocRepo struct{ db *gorm.DB }

// NewKnowledgeDocRepo 构造仓储。
func NewKnowledgeDocRepo(db *gorm.DB) *KnowledgeDocRepo { return &KnowledgeDocRepo{db: db} }

// List 全量（按创建时间倒序）。
func (r *KnowledgeDocRepo) List(ctx context.Context) ([]domain.KnowledgeDocDO, error) {
	var rows []domain.KnowledgeDocDO
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2080, "list knowledge docs failed", err)
	}
	return rows, nil
}

// ListBySourceType 按来源类型过滤（source_type 为空的按 text 兜底）。
func (r *KnowledgeDocRepo) ListBySourceType(ctx context.Context, st domain.KnowledgeSourceType) ([]domain.KnowledgeDocDO, error) {
	if st == "" {
		st = domain.KnowledgeSourceText
	}
	var rows []domain.KnowledgeDocDO
	if err := r.db.WithContext(ctx).
		Where("source_type = ?", st).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2080, "list knowledge docs by type failed", err)
	}
	return rows, nil
}

// GetByID 按 ID 查找。
func (r *KnowledgeDocRepo) GetByID(ctx context.Context, id string) (*domain.KnowledgeDocDO, error) {
	var row domain.KnowledgeDocDO
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrKnowledgeDocNotFound
		}
		return nil, pkg.Wrap(2080, "get knowledge doc failed", err)
	}
	return &row, nil
}

// Create 新建文档。
func (r *KnowledgeDocRepo) Create(ctx context.Context, row *domain.KnowledgeDocDO) error {
	if row.ID == "" {
		row.ID = pkg.NewID(domain.IDKnowledgeDoc)
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2081, "create knowledge doc failed", err)
	}
	return nil
}

// Update 更新（状态机回写）。
func (r *KnowledgeDocRepo) Update(ctx context.Context, row *domain.KnowledgeDocDO) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return pkg.Wrap(2081, "update knowledge doc failed", err)
	}
	return nil
}

// Delete 软删并物理清掉其分块。
func (r *KnowledgeDocRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.GetByID(ctx, id); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM knowledge_chunks_fts WHERE doc_id = ?`, id).Error; err != nil {
			return pkg.Wrap(2082, "delete fts rows failed", err)
		}
		if err := tx.Where("doc_id = ?", id).Delete(&domain.KnowledgeChunkDO{}).Error; err != nil {
			return pkg.Wrap(2082, "delete chunks failed", err)
		}
		if err := tx.Delete(&domain.KnowledgeDocDO{}, "id = ?", id).Error; err != nil {
			return pkg.Wrap(2082, "delete doc failed", err)
		}
		return nil
	})
}
