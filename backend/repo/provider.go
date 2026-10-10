package repo

import (
	"WorkBaby/backend/domain"
)

// UpsertProvider 新增或更新模型服务。
func (r *Repo) UpsertProvider(p *domain.ProviderDO) error {
	return wrapDB("保存模型服务", r.db.Save(p).Error)
}

// GetProvider 按 id 取模型服务。
func (r *Repo) GetProvider(id string) (*domain.ProviderDO, error) {
	var p domain.ProviderDO
	if err := r.db.Where("id = ?", id).First(&p).Error; err != nil {
		if notFound(err) {
			return nil, domain.ErrProviderNotFound
		}
		return nil, wrapDB("读取模型服务", err)
	}
	return &p, nil
}

// ListProviders 列出全部模型服务。
func (r *Repo) ListProviders() ([]domain.ProviderDO, error) {
	var list []domain.ProviderDO
	if err := r.db.Order("created_at ASC").Find(&list).Error; err != nil {
		return nil, wrapDB("列出模型服务", err)
	}
	return list, nil
}

// DeleteProvider 删除模型服务。
func (r *Repo) DeleteProvider(id string) error {
	return wrapDB("删除模型服务", r.db.Where("id = ?", id).Delete(&domain.ProviderDO{}).Error)
}

// ClearDefault 清掉所有默认标记，保证默认服务唯一。
func (r *Repo) ClearDefault() error {
	return wrapDB("清除默认标记", r.db.Model(&domain.ProviderDO{}).Where("is_default = ?", true).
		Update("is_default", false).Error)
}
