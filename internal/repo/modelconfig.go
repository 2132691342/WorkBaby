package repo

import "WorkBaby/internal/domain"

// UpsertModelConfig 保存模型级配置；复合主键不存在则插入。
func (r *Repo) UpsertModelConfig(d *domain.ModelConfigDO) error {
	return r.db.Save(d).Error
}

// GetModelConfig 取一条模型配置；没有配置返回 nil 而不是错误，调用方按缺省处理。
func (r *Repo) GetModelConfig(providerID, model string) (*domain.ModelConfigDO, error) {
	var d domain.ModelConfigDO
	err := r.db.Where("provider_id = ? AND model = ?", providerID, model).First(&d).Error
	if err != nil {
		if notFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// ListModelConfigs 列出一个服务下的全部模型配置。
func (r *Repo) ListModelConfigs(providerID string) ([]domain.ModelConfigDO, error) {
	list := []domain.ModelConfigDO{}
	q := r.db.Model(&domain.ModelConfigDO{})
	if providerID != "" {
		q = q.Where("provider_id = ?", providerID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteModelConfigsByProvider 删除服务时级联清理它的模型配置。
func (r *Repo) DeleteModelConfigsByProvider(providerID string) error {
	return r.db.Where("provider_id = ?", providerID).Delete(&domain.ModelConfigDO{}).Error
}
