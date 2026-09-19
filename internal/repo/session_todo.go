package repo

import (
	"context"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// SessionTodoRepo session_todos 表 CRUD：会话计划的持久化真相源。
type SessionTodoRepo struct{ db *gorm.DB }

func NewSessionTodoRepo(db *gorm.DB) *SessionTodoRepo { return &SessionTodoRepo{db: db} }

// List 按展示顺序返回会话计划项。
func (r *SessionTodoRepo) List(ctx context.Context, sessionID string) ([]domain.SessionTodoDO, error) {
	var rows []domain.SessionTodoDO
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Order("ordering ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2065, "list session todos failed", err)
	}
	return rows, nil
}

// ReplaceAll 用新计划整体替换（事务：删旧 + 批量插新）。
//
// 「覆盖」是计划语义：模型重列 plan 时不能留下上一版条目，否则新旧条目并存会让
// 勾选进度串味。
func (r *SessionTodoRepo) ReplaceAll(ctx context.Context, sessionID string, items []domain.TodoItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&domain.SessionTodoDO{}).Error; err != nil {
			return pkg.Wrap(2065, "clear session todos failed", err)
		}
		if len(items) == 0 {
			return nil
		}
		now := time.Now().UnixMilli()
		rows := make([]domain.SessionTodoDO, 0, len(items))
		for i, it := range items {
			rows = append(rows, domain.SessionTodoDO{
				ID:        pkg.NewID(domain.IDTodo),
				SessionID: sessionID,
				ItemID:    it.ID,
				Title:     it.Title,
				Done:      it.Done,
				Ordering:  i,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		if err := tx.Create(&rows).Error; err != nil {
			return pkg.Wrap(2065, "insert session todos failed", err)
		}
		return nil
	})
}

// UpdateDone 勾选/取消单条；返回是否命中（未命中由上层转 ErrTodoItemNotFound）。
func (r *SessionTodoRepo) UpdateDone(ctx context.Context, sessionID, itemID string, done bool) (bool, error) {
	res := r.db.WithContext(ctx).Model(&domain.SessionTodoDO{}).
		Where("session_id = ? AND item_id = ?", sessionID, itemID).
		Updates(map[string]any{"done": done, "updated_at": time.Now().UnixMilli()})
	if res.Error != nil {
		return false, pkg.Wrap(2065, "update session todo failed", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// DeleteBySession 清理会话计划（会话删除时调用）。
func (r *SessionTodoRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Delete(&domain.SessionTodoDO{}).Error; err != nil {
		return pkg.Wrap(2065, "delete session todos failed", err)
	}
	return nil
}
