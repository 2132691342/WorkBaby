package repo

import (
	"WorkBaby/backend/domain"

	"gorm.io/gorm"
)

// AppendEntry 追加一条条目并把会话的 leaf 指针与计数一起推进（同一事务，保证链不裂）。
func (r *Repo) AppendEntry(e *domain.EntryDO) error {
	return wrapDB("落库条目", r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(e).Error; err != nil {
			return err
		}
		return tx.Model(&domain.SessionDO{}).
			Where("id = ?", e.SessionID).
			Updates(map[string]any{
				"leaf_entry_id": e.ID,
				"message_count": gorm.Expr("message_count + 1"),
				"updated_at":    e.CreatedAt,
			}).Error
	}))
}

// ListEntries 按 seq 顺序列出会话全部条目；树还原在调用方做。
func (r *Repo) ListEntries(sessionID string) ([]domain.EntryDO, error) {
	var list []domain.EntryDO
	if err := r.db.Where("session_id = ?", sessionID).Order("seq ASC").Find(&list).Error; err != nil {
		return nil, wrapDB("读取会话条目", err)
	}
	return list, nil
}

// MaxSeq 取会话当前最大 seq，用于分配下一条。
func (r *Repo) MaxSeq(sessionID string) (int, error) {
	var seq int
	err := r.db.Model(&domain.EntryDO{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").Scan(&seq).Error
	return seq, wrapDB("读取会话序号", err)
}
