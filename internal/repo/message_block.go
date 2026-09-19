package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// MessageBlockRepo message_blocks 表 CRUD（消息块落库）。
type MessageBlockRepo struct{ db *gorm.DB }

func NewMessageBlockRepo(db *gorm.DB) *MessageBlockRepo { return &MessageBlockRepo{db: db} }

// CreateBatch 批量写入块（单事务，随工具事件落库）。
func (r *MessageBlockRepo) CreateBatch(ctx context.Context, rows []domain.MessageBlockDO) error {
	if len(rows) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return pkg.Wrap(2065, "insert message blocks failed", err)
	}
	return nil
}

// Create 单条写入。
func (r *MessageBlockRepo) Create(ctx context.Context, row *domain.MessageBlockDO) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2065, "insert message block failed", err)
	}
	return nil
}

// ListByMessage 按消息取块（seq ASC），供历史消息复现工具过程。
func (r *MessageBlockRepo) ListByMessage(ctx context.Context, messageID string) ([]domain.MessageBlockDO, error) {
	var rows []domain.MessageBlockDO
	if err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("seq ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2066, "list message blocks failed", err)
	}
	return rows, nil
}

// ListBySession 按会话取块（seq ASC），供 ListMessages 批量装配。
func (r *MessageBlockRepo) ListBySession(ctx context.Context, sessionID string) ([]domain.MessageBlockDO, error) {
	var rows []domain.MessageBlockDO
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("message_id ASC, seq ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2066, "list session blocks failed", err)
	}
	return rows, nil
}

// DeleteByMessage 删除某消息的全部块。
func (r *MessageBlockRepo) DeleteByMessage(ctx context.Context, messageID string) error {
	if err := r.db.WithContext(ctx).Where("message_id = ?", messageID).
		Delete(&domain.MessageBlockDO{}).Error; err != nil {
		return pkg.Wrap(2067, "delete message blocks failed", err)
	}
	return nil
}

// DeleteBySession 级联删除会话全部块（随会话清理）。
func (r *MessageBlockRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Delete(&domain.MessageBlockDO{}).Error; err != nil {
		return pkg.Wrap(2067, "delete session blocks failed", err)
	}
	return nil
}
