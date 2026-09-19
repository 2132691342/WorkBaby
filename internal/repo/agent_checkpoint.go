package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentCheckpointRepo Agent 检查点持久化（SQL，C2）。
type AgentCheckpointRepo struct{ db *gorm.DB }

// NewAgentCheckpointRepo 构造。
func NewAgentCheckpointRepo(db *gorm.DB) *AgentCheckpointRepo { return &AgentCheckpointRepo{db: db} }

// Save 保存/覆盖一轮检查点（(run_id, turn) 唯一）。
func (r *AgentCheckpointRepo) Save(ctx context.Context, row *domain.AgentCheckpointDO) error {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "turn"}},
		DoUpdates: clause.AssignmentColumns([]string{"messages_json", "state_json", "usage_json", "content", "thinking", "created_at"}),
	}).Create(row).Error; err != nil {
		return pkg.Wrap(2015, "save agent checkpoint failed", err)
	}
	return nil
}

// DeleteBeforeTurn 删除该 run 早于指定 turn 的检查点行（每 run 只留最新快照）。
func (r *AgentCheckpointRepo) DeleteBeforeTurn(ctx context.Context, runID string, turn int) error {
	if err := r.db.WithContext(ctx).
		Where("run_id = ? AND turn < ?", runID, turn).
		Delete(&domain.AgentCheckpointDO{}).Error; err != nil {
		return pkg.Wrap(2015, "prune agent checkpoints failed", err)
	}
	return nil
}

// LoadLast 返回指定 run 最新一轮检查点；无记录返回 5007。
func (r *AgentCheckpointRepo) LoadLast(ctx context.Context, runID string) (*domain.AgentCheckpointDO, error) {
	var row domain.AgentCheckpointDO
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("turn DESC").First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, pkg.New(5007, "检查点不存在", runID)
		}
		return nil, pkg.Wrap(5005, "load agent checkpoint failed", err)
	}
	return &row, nil
}

// CleanupRuns 保留该会话最近 keep 个有检查点的 run，删除其余 run 的全部检查点行。
func (r *AgentCheckpointRepo) CleanupRuns(ctx context.Context, sessionID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	sub := r.db.WithContext(ctx).Model(&domain.AgentCheckpointDO{}).
		Select("run_id").
		Where("session_id = ?", sessionID).
		Group("run_id").
		Order("MAX(created_at) DESC").
		Limit(keep)
	if err := r.db.WithContext(ctx).
		Where("session_id = ? AND run_id NOT IN (?)", sessionID, sub).
		Delete(&domain.AgentCheckpointDO{}).Error; err != nil {
		return pkg.Wrap(2015, "cleanup agent checkpoints failed", err)
	}
	return nil
}

// DeleteBySession 删除整个会话的检查点（会话删除级联清理）。
func (r *AgentCheckpointRepo) DeleteBySession(ctx context.Context, sessionID string) error {
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&domain.AgentCheckpointDO{}).Error; err != nil {
		return pkg.Wrap(2015, "delete agent checkpoints failed", err)
	}
	return nil
}

// CountBySession 会话检查点 run 数（调试/统计）。
func (r *AgentCheckpointRepo) CountBySession(ctx context.Context, sessionID string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&domain.AgentCheckpointDO{}).
		Where("session_id = ?", sessionID).
		Distinct("run_id").Count(&n).Error; err != nil {
		return 0, pkg.Wrap(2015, "count agent checkpoints failed", err)
	}
	return n, nil
}
