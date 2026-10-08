package repo

import (
	"WorkBaby/backend/domain"
)

// CreateApproval 落一条待决审批。
func (r *Repo) CreateApproval(a *domain.ApprovalDO) error {
	return r.db.Create(a).Error
}

// GetApproval 按 id 取审批。
func (r *Repo) GetApproval(id string) (*domain.ApprovalDO, error) {
	var a domain.ApprovalDO
	if err := r.db.Where("id = ?", id).First(&a).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrApprovalNotFound
		}
		return nil, err
	}
	return &a, nil
}

// ListPendingApprovals 列出会话内待决审批（sessionID 为空表示全部会话）。
func (r *Repo) ListPendingApprovals(sessionID string) ([]domain.ApprovalDO, error) {
	var list []domain.ApprovalDO
	q := r.db.Where("status = ?", domain.ApprovalPending).Order("created_at ASC")
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// SettleApproval 落决策结果；只从 pending 迁移，避免重复决策。
func (r *Repo) SettleApproval(id, status string, decidedAt int64) error {
	res := r.db.Model(&domain.ApprovalDO{}).
		Where("id = ? AND status = ?", id, domain.ApprovalPending).
		Updates(map[string]any{"status": status, "decided_at": decidedAt})
	if err := res.Error; err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domain.ErrApprovalSettled
	}
	return nil
}

// SettleAllPending 把全部待决审批一次性收口，供启动时清理上次运行的残留。
func (r *Repo) SettleAllPending(status string, at int64) error {
	return r.db.Model(&domain.ApprovalDO{}).
		Where("status = ?", domain.ApprovalPending).
		Updates(map[string]any{"status": status, "decided_at": at}).Error
}

// CancelSessionApprovals 把会话内剩余待决审批置为取消（run 被停止时调用）。
func (r *Repo) CancelSessionApprovals(sessionID string, at int64) error {
	return r.db.Model(&domain.ApprovalDO{}).
		Where("session_id = ? AND status = ?", sessionID, domain.ApprovalPending).
		Updates(map[string]any{"status": domain.ApprovalDenied, "decided_at": at}).Error
}
