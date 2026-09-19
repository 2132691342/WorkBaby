package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// ChatSessionRepo chat_sessions 表 CRUD。
type ChatSessionRepo struct{ db *gorm.DB }

func NewChatSessionRepo(db *gorm.DB) *ChatSessionRepo { return &ChatSessionRepo{db: db} }

func (r *ChatSessionRepo) Create(ctx context.Context, s *domain.ChatSessionDO) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return pkg.Wrap(2040, "create session failed", err)
	}
	return nil
}

func (r *ChatSessionRepo) Update(ctx context.Context, s *domain.ChatSessionDO) error {
	if err := r.db.WithContext(ctx).Save(s).Error; err != nil {
		return pkg.Wrap(2041, "update session failed", err)
	}
	return nil
}

// SoftDelete 软删（deleted_at=now）；不返回 error（业务层不区分）。
func (r *ChatSessionRepo) SoftDelete(ctx context.Context, id string, atMs int64) error {
	if err := r.db.WithContext(ctx).Model(&domain.ChatSessionDO{}).
		Where("id = ?", id).
		Update("deleted_at", atMs).Error; err != nil {
		return pkg.Wrap(2042, "soft delete session failed", err)
	}
	return nil
}

func (r *ChatSessionRepo) GetByID(ctx context.Context, id string) (*domain.ChatSessionDO, error) {
	var s domain.ChatSessionDO
	if err := r.db.WithContext(ctx).Where("deleted_at = ?", 0).First(&s, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, pkg.Wrap(2043, "get session failed", err)
	}
	return &s, nil
}

// listVisible 会话列表的可见性条件：未删除的 normal 会话（side 辅助会话只在辅助面板出现）。
// kind 兼容三态：老数据列为空 / NULL / 'normal' 都算主会话。
const listVisibleConds = "user_id = ? AND deleted_at = ? AND (kind = 'normal' OR kind = '' OR kind IS NULL)"

// ListByUser 列出某用户未删除的主会话（side 辅助会话除外），最近活跃优先。
func (r *ChatSessionRepo) ListByUser(ctx context.Context, userID string) ([]domain.ChatSessionDO, error) {
	var ss []domain.ChatSessionDO
	if err := r.db.WithContext(ctx).
		Where(listVisibleConds, userID, 0).
		Order("pinned DESC, last_message_at DESC, created_at DESC").
		Find(&ss).Error; err != nil {
		return nil, pkg.Wrap(2044, "list sessions failed", err)
	}
	return ss, nil
}

// CountByUser 某用户未删除主会话总数（分页 total）。
func (r *ChatSessionRepo) CountByUser(ctx context.Context, userID string) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&domain.ChatSessionDO{}).
		Where(listVisibleConds, userID, 0).
		Count(&n).Error; err != nil {
		return 0, pkg.Wrap(2044, "count sessions failed", err)
	}
	return int(n), nil
}

// ListByUserPage 分页列出某用户未删除的主会话（page 从 1 起；side 辅助会话除外）。
func (r *ChatSessionRepo) ListByUserPage(ctx context.Context, userID string, page, pageSize int) ([]domain.ChatSessionDO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var ss []domain.ChatSessionDO
	if err := r.db.WithContext(ctx).
		Where(listVisibleConds, userID, 0).
		Order("pinned DESC, last_message_at DESC, created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&ss).Error; err != nil {
		return nil, pkg.Wrap(2044, "list sessions failed", err)
	}
	return ss, nil
}

// FindSideByParent 取某主会话的辅助会话（一个主会话至多一个；多个时取最新）。
// 不存在返回 (nil, nil)——辅助面板据此决定「新建还是复用」。
func (r *ChatSessionRepo) FindSideByParent(ctx context.Context, parentID string) (*domain.ChatSessionDO, error) {
	var s domain.ChatSessionDO
	err := r.db.WithContext(ctx).
		Where("parent_id = ? AND kind = 'side' AND deleted_at = ?", parentID, 0).
		Order("created_at DESC").
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, pkg.Wrap(2043, "find side session failed", err)
	}
	return &s, nil
}
