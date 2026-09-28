package repo

import (
	"WorkBaby/internal/domain"

	"gorm.io/gorm"
)

// CreateSession 创建会话。
func (r *Repo) CreateSession(s *domain.SessionDO) error {
	return r.db.Create(s).Error
}

// GetSession 按 id 取会话；未命中返回 domain.ErrSessionNotFound。
func (r *Repo) GetSession(id string) (*domain.SessionDO, error) {
	var s domain.SessionDO
	if err := r.db.Where("id = ?", id).First(&s).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return &s, nil
}

// ListSessions 按更新时间倒序列出会话。
func (r *Repo) ListSessions(limit int) ([]domain.SessionDO, error) {
	var list []domain.SessionDO
	q := r.db.Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateSession 全量更新会话。
func (r *Repo) UpdateSession(s *domain.SessionDO) error {
	return r.db.Save(s).Error
}

// UpdateSessionColumns 只更新指定列，避免并发写覆盖掉别的字段。
func (r *Repo) UpdateSessionColumns(id string, cols map[string]any) error {
	if len(cols) == 0 {
		return nil
	}
	return r.db.Model(&domain.SessionDO{}).Where("id = ?", id).Updates(cols).Error
}

// DeleteSession 删除会话及其全部条目与用量记录。
func (r *Repo) DeleteSession(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&domain.EntryDO{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.TokenUsageDO{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.ApprovalDO{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.SessionDO{}).Error
	})
}
