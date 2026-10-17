// 会话的持久层：增删查改与 token 累计。
package repo

import (
	"WorkBaby/backend/domain"

	"gorm.io/gorm"
)

// CreateSession 创建会话。
func (r *Repo) CreateSession(s *domain.SessionDO) error {
	return wrapDB("创建会话", r.db.Create(s).Error)
}

// GetSession 按 id 取会话；未命中返回 domain.ErrSessionNotFound。
func (r *Repo) GetSession(id string) (*domain.SessionDO, error) {
	var s domain.SessionDO
	if err := r.db.Where("id = ?", id).First(&s).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, wrapDB("读取会话", err)
	}
	return &s, nil
}

// ListSessions 按更新时间倒序分页列出会话；offset 为 0 即第一页。
func (r *Repo) ListSessions(limit, offset int) ([]domain.SessionDO, error) {
	var list []domain.SessionDO
	// 隐藏会话（编排域子运行 / 任务执行的载体）不进用户列表：
	// 列表里出现一堆来路不明的会话就是乱账。
	q := r.db.Where("hidden = ?", false).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, wrapDB("列出会话", err)
	}
	return list, nil
}

// UpdateSessionColumns 只更新指定列，避免并发写覆盖掉别的字段。
func (r *Repo) UpdateSessionColumns(id string, cols map[string]any) error {
	if len(cols) == 0 {
		return nil
	}
	return wrapDB("更新会话", r.db.Model(&domain.SessionDO{}).Where("id = ?", id).Updates(cols).Error)
}

// AddSessionTokens 原子累加会话累计 token：读-改-写在两条路径并发收尾时会丢账。
func (r *Repo) AddSessionTokens(id string, delta int) error {
	return wrapDB("累计会话 token", r.db.Model(&domain.SessionDO{}).
		Where("id = ?", id).
		Update("total_tokens", gorm.Expr("total_tokens + ?", delta)).Error)
}

// DeleteSession 删除会话及其用量记录。
// PI 的 JSONL 文件由 service 层负责删：repo 不感知文件系统。
func (r *Repo) DeleteSession(id string) error {
	return wrapDB("删除会话", r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&domain.TokenUsageDO{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.ApprovalDO{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.SessionDO{}).Error
	}))
}
