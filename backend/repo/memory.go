// 记忆的持久层：读写与去重查找。
package repo

import (
	"WorkBaby/backend/domain"
)

// ListMemories 列出全部记忆，最近更新的在前——提示注入按这个顺序取最新的那些。
func (r *Repo) ListMemories() ([]domain.MemoryDO, error) {
	var list []domain.MemoryDO
	if err := r.db.Order("updated_at DESC").Find(&list).Error; err != nil {
		return nil, wrapDB("列出记忆", err)
	}
	return list, nil
}

// FindMemoryByContent 按内容精确查找：写入前先查一次，避免同一件事记十遍。
func (r *Repo) FindMemoryByContent(content string) (*domain.MemoryDO, error) {
	var m domain.MemoryDO
	err := r.db.Where("content = ?", content).First(&m).Error
	if err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, wrapDB("查找记忆", err)
	}
	return &m, nil
}

// GetMemory 取一条记忆。
func (r *Repo) GetMemory(id string) (*domain.MemoryDO, error) {
	var m domain.MemoryDO
	if err := r.db.Where("id = ?", id).First(&m).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrMemoryNotFound
		}
		return nil, wrapDB("读取记忆", err)
	}
	return &m, nil
}

// AddMemory 写入一条记忆。
func (r *Repo) AddMemory(m *domain.MemoryDO) error {
	return wrapDB("保存记忆", r.db.Create(m).Error)
}

// UpdateMemory 更新内容与开关；只更新传入的字段。
func (r *Repo) UpdateMemory(id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := r.db.Model(&domain.MemoryDO{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return wrapDB("更新记忆", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrMemoryNotFound
	}
	return nil
}

// DeleteMemory 删除一条记忆；本来就不在也算成功——界面上的删除应当是幂等的。
func (r *Repo) DeleteMemory(id string) error {
	return wrapDB("删除记忆", r.db.Where("id = ?", id).Delete(&domain.MemoryDO{}).Error)
}

// CountMemories 当前记忆条数，用于总量上限。
func (r *Repo) CountMemories() (int64, error) {
	var n int64
	if err := r.db.Model(&domain.MemoryDO{}).Count(&n).Error; err != nil {
		return 0, wrapDB("统计记忆", err)
	}
	return n, nil
}
