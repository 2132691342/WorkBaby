package repo

import (
	"context"
	"errors"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
)

// ApprovalRecordRepo 审批/补充输入记录持久化。
type ApprovalRecordRepo struct{ db *gorm.DB }

// NewApprovalRecordRepo 构造。
func NewApprovalRecordRepo(db *gorm.DB) *ApprovalRecordRepo { return &ApprovalRecordRepo{db: db} }

// Create 落未决记录。
func (r *ApprovalRecordRepo) Create(ctx context.Context, row *domain.ApprovalRecordDO) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return pkg.Wrap(2016, "create approval record failed", err)
	}
	return nil
}

// SetStatus 更新终态（带用户回复，input 类）。
func (r *ApprovalRecordRepo) SetStatus(ctx context.Context, id, status, answer string) error {
	updates := map[string]any{"status": status, "decided_at": time.Now().UnixMilli()}
	if answer != "" {
		updates["answer"] = answer
	}
	if err := r.db.WithContext(ctx).Model(&domain.ApprovalRecordDO{}).
		Where("id = ?", id).Updates(updates).Error; err != nil {
		return pkg.Wrap(2016, "update approval record failed", err)
	}
	return nil
}

// ListPending 未决记录（重启恢复视图 / 决策窗口重武装）。
func (r *ApprovalRecordRepo) ListPending(ctx context.Context) ([]domain.ApprovalRecordDO, error) {
	var rows []domain.ApprovalRecordDO
	if err := r.db.WithContext(ctx).
		Where("status = ?", domain.ApprovalStatusPending).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2016, "list pending approvals failed", err)
	}
	return rows, nil
}

// Get 按 id 取记录（重启后决策恢复路径：记录在库而内存无等待者）。
func (r *ApprovalRecordRepo) Get(ctx context.Context, id string) (*domain.ApprovalRecordDO, error) {
	var row domain.ApprovalRecordDO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, pkg.Wrap(2016, "get approval record failed", err)
	}
	return &row, nil
}

// FindDecided 按 (runID, kind, command) 取匹配的已决记录（护栏链快速放行/拒绝用）：
// 续跑重放的同一调用不再二次询问，消费后置 consumed 防重复免审。取最新一条。
func (r *ApprovalRecordRepo) FindDecided(ctx context.Context, runID, kind, command string, statuses []string) (*domain.ApprovalRecordDO, error) {
	var row domain.ApprovalRecordDO
	q := r.db.WithContext(ctx).
		Where("run_id = ? AND kind = ? AND command = ? AND status IN ?", runID, kind, command, statuses).
		Order("created_at DESC")
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, pkg.Wrap(2016, "find approval record failed", err)
	}
	return &row, nil
}

// ListPendingBySession 会话内未决记录（新 run 启动时清理同会话旧挂起）。
func (r *ApprovalRecordRepo) ListPendingBySession(ctx context.Context, sessionID string) ([]domain.ApprovalRecordDO, error) {
	var rows []domain.ApprovalRecordDO
	if err := r.db.WithContext(ctx).
		Where("session_id = ? AND status = ?", sessionID, domain.ApprovalStatusPending).
		Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2016, "list session pending approvals failed", err)
	}
	return rows, nil
}

// UpdateExpiresAt 重武装决策窗口：重启后 pending 记录延长有效期，用户仍可决策。
func (r *ApprovalRecordRepo) UpdateExpiresAt(ctx context.Context, id string, expiresAt int64) error {
	if err := r.db.WithContext(ctx).Model(&domain.ApprovalRecordDO{}).
		Where("id = ?", id).Update("expires_at", expiresAt).Error; err != nil {
		return pkg.Wrap(2016, "rearm approval record failed", err)
	}
	return nil
}

// DeleteBySession 会话删除级联清理。
func (r *ApprovalRecordRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&domain.ApprovalRecordDO{}).Error; err != nil {
		return pkg.Wrap(2016, "delete approval records failed", err)
	}
	return nil
}
