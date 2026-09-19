package repo

import (
	"context"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ApprovalGrantRepo 免审授权持久化（「本会话允许」升级为跨重启生效，可撤销、可回滚）。
type ApprovalGrantRepo struct{ db *gorm.DB }

// NewApprovalGrantRepo 构造。
func NewApprovalGrantRepo(db *gorm.DB) *ApprovalGrantRepo { return &ApprovalGrantRepo{db: db} }

// Create 落授权；同命令重复批准保持单条（command 唯一，冲突更新时间戳）。
func (r *ApprovalGrantRepo) Create(ctx context.Context, row *domain.ApprovalGrantDO) error {
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "command"}},
		DoUpdates: clause.AssignmentColumns([]string{"risk", "created_at"}),
	}).Create(row).Error; err != nil {
		return pkg.Wrap(2016, "create approval grant failed", err)
	}
	return nil
}

// List 全量授权（启动装载 + 设置页列表）。
func (r *ApprovalGrantRepo) List(ctx context.Context) ([]domain.ApprovalGrantDO, error) {
	var rows []domain.ApprovalGrantDO
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, pkg.Wrap(2016, "list approval grants failed", err)
	}
	return rows, nil
}

// Delete 撤销授权（设置页）。
func (r *ApprovalGrantRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.ApprovalGrantDO{}, "id = ?", id).Error; err != nil {
		return pkg.Wrap(2016, "delete approval grant failed", err)
	}
	return nil
}

// DeleteByCommand 回滚：按命令撤销（run 失败/取消时调用）。
func (r *ApprovalGrantRepo) DeleteByCommand(ctx context.Context, command string) error {
	if err := r.db.WithContext(ctx).Delete(&domain.ApprovalGrantDO{}, "command = ?", command).Error; err != nil {
		return pkg.Wrap(2016, "delete approval grant by command failed", err)
	}
	return nil
}
