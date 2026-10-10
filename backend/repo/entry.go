package repo

import (
	"WorkBaby/backend/domain"

	"gorm.io/gorm"
)

// AppendEntry 追加一条条目并把会话的 leaf 指针与计数一起推进（同一事务，保证链不裂）。
// seq 在事务内分配：连接固定为一条，事务把「读最大 seq」与「插入」绑成原子步，
// 并发追加（run 收尾与入队落库交错）不会撞号、不会分叉。
func (r *Repo) AppendEntry(e *domain.EntryDO) error {
	return wrapDB("落库条目", r.db.Transaction(func(tx *gorm.DB) error {
		var seq int
		if err := tx.Model(&domain.EntryDO{}).
			Where("session_id = ?", e.SessionID).
			Select("COALESCE(MAX(seq), 0)").Scan(&seq).Error; err != nil {
			return err
		}
		e.Seq = seq + 1
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

// EntryExists 判断条目是否属于该会话；只查主键，不拉整棵条目树。
func (r *Repo) EntryExists(sessionID, entryID string) (bool, error) {
	var n int64
	err := r.db.Model(&domain.EntryDO{}).
		Where("id = ? AND session_id = ?", entryID, sessionID).
		Limit(1).Count(&n).Error
	if err != nil {
		return false, wrapDB("查询条目", err)
	}
	return n > 0, nil
}

// ListEntries 按 seq 顺序列出会话全部条目；树还原在调用方做。
func (r *Repo) ListEntries(sessionID string) ([]domain.EntryDO, error) {
	var list []domain.EntryDO
	if err := r.db.Where("session_id = ?", sessionID).Order("seq ASC").Find(&list).Error; err != nil {
		return nil, wrapDB("读取会话条目", err)
	}
	return list, nil
}
