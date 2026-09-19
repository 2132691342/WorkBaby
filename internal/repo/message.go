package repo

import (
	"context"
	"errors"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// MessageRepo chat_messages 表 CRUD。
type MessageRepo struct{ db *gorm.DB }

func NewMessageRepo(db *gorm.DB) *MessageRepo { return &MessageRepo{db: db} }

// Insert 新增一条消息。
func (r *MessageRepo) Insert(ctx context.Context, m *domain.MessageDO) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return pkg.Wrap(2060, "insert message failed", err)
	}
	return nil
}

// UpdateStatus 更新单条状态/统计/终止原因。
func (r *MessageRepo) UpdateStatus(ctx context.Context, id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&domain.MessageDO{}).
		Where("id = ?", id).
		Updates(fields).Error; err != nil {
		return pkg.Wrap(2061, "update message failed", err)
	}
	return nil
}

// ReapStreaming 启动排空：进程崩溃时卡在 streaming 的 assistant 消息批量标
// failed + stop_reason=interrupted（前端据此给「继续」入口，可经 Resume 续跑）。
func (r *MessageRepo) ReapStreaming(ctx context.Context, status domain.MessageStatus, stopReason string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&domain.MessageDO{}).
		Where("status = ?", domain.MessageStatusStreaming).
		Updates(map[string]any{"status": string(status), "stop_reason": stopReason})
	if res.Error != nil {
		return 0, pkg.Wrap(2061, "reap streaming messages failed", res.Error)
	}
	return res.RowsAffected, nil
}

// DeleteAll 物理删除某会话的全部消息。
func (r *MessageRepo) DeleteAll(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Delete(&domain.MessageDO{}).Error; err != nil {
		return pkg.Wrap(2063, "delete messages failed", err)
	}
	return nil
}

// GetByID 按主键取单条消息（用于校验归属与取 seq）。
func (r *MessageRepo) GetByID(ctx context.Context, id string) (*domain.MessageDO, error) {
	var m domain.MessageDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMessageNotFound
		}
		return nil, pkg.Wrap(2062, "get message failed", err)
	}
	return &m, nil
}

// DeleteByID 物理删除单条消息。
func (r *MessageRepo) DeleteByID(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).
		Delete(&domain.MessageDO{}).Error; err != nil {
		return pkg.Wrap(2063, "delete message failed", err)
	}
	return nil
}

// DeleteFromSeq 从 seq 起（含）删除该会话后续消息（截断重跑）。
func (r *MessageRepo) DeleteFromSeq(ctx context.Context, sessionID string, seq int64) error {
	if err := r.db.WithContext(ctx).Where("session_id = ? AND seq >= ?", sessionID, seq).
		Delete(&domain.MessageDO{}).Error; err != nil {
		return pkg.Wrap(2063, "truncate messages failed", err)
	}
	return nil
}

// ListUpTo Seq 取 seq <= 指定值 的消息（分叉时复制到新会话）。
func (r *MessageRepo) ListUpTo(ctx context.Context, sessionID string, seq int64) ([]domain.MessageDO, error) {
	var ms []domain.MessageDO
	if err := r.db.WithContext(ctx).Where("session_id = ? AND seq <= ?", sessionID, seq).
		Order("seq ASC").Find(&ms).Error; err != nil {
		return nil, pkg.Wrap(2062, "list messages up to seq failed", err)
	}
	return ms, nil
}

// ListBySession 增量查询（afterSeq>0 时取 >= afterSeq，否则取全部；按 seq ASC）。
func (r *MessageRepo) ListBySession(ctx context.Context, sessionID string, afterSeq int64, limit int) ([]domain.MessageDO, error) {
	var ms []domain.MessageDO
	q := r.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if afterSeq > 0 {
		q = q.Where("seq > ?", afterSeq)
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if err := q.Order("seq ASC").Limit(limit).Find(&ms).Error; err != nil {
		return nil, pkg.Wrap(2062, "list messages failed", err)
	}
	return ms, nil
}

// MaxSeq 会话当前最大消息序号（0 = 无消息）。
//
// 序号分配器的首次水位必须由数据库权威给出：按行扫描受单页上限约束，长会话
// （消息数 > 单页上限）会漏掉更大的 seq，重启后从错误水位继续分配即与历史消息撞号，
// 增量分页（after_seq 游标）随之漏消息。
func (r *MessageRepo) MaxSeq(ctx context.Context, sessionID string) (int64, error) {
	var maxSeq int64
	if err := r.db.WithContext(ctx).Model(&domain.MessageDO{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxSeq).Error; err != nil {
		return 0, pkg.Wrap(2062, "query max message seq failed", err)
	}
	return maxSeq, nil
}

// SearchSessions 按正文关键词检索命中消息的会话（去重；按最大 seq 倒序）。
//
// 只扫 user/assistant 正文（tool 结果与 system 提示词噪声太大）；
// LIKE 走全表扫描，但会话消息量在桌面单机量级（万级），无需额外索引。
func (r *MessageRepo) SearchSessions(ctx context.Context, keyword string, limit int) ([]domain.SessionHitDTO, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	var out []domain.SessionHitDTO
	err := r.db.WithContext(ctx).
		Model(&domain.MessageDO{}).
		Select("session_id, COUNT(*) AS hit_count, MAX(seq) AS last_seq").
		Where("content LIKE ? AND role IN ?", "%"+keyword+"%",
			[]domain.MessageRole{domain.MessageRoleUser, domain.MessageRoleAssistant}).
		Group("session_id").
		Order("last_seq DESC").
		Limit(limit).
		Scan(&out).Error
	if err != nil {
		return nil, pkg.Wrap(2064, "search messages failed", err)
	}
	return out, nil
}

// ListRecentBySession 取最近 limit 条（seq DESC，供短期记忆窗口）。
func (r *MessageRepo) ListRecentBySession(ctx context.Context, sessionID string, limit int) ([]domain.MessageDO, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	var ms []domain.MessageDO
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq DESC").
		Limit(limit).
		Find(&ms).Error; err != nil {
		return nil, pkg.Wrap(2062, "list recent messages failed", err)
	}
	return ms, nil
}
