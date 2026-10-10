package repo

import (
	"WorkBaby/backend/domain"
)

// GetSetting 取一个设置值；不存在返回空串。
func (r *Repo) GetSetting(key string) (string, error) {
	var s domain.SettingDO
	if err := r.db.Where("`key` = ?", key).First(&s).Error; err != nil {
		if notFound(err) {
			return "", nil
		}
		return "", wrapDB("读取设置", err)
	}
	return s.Value, nil
}

// SetSetting 写入设置值。
func (r *Repo) SetSetting(key, value string) error {
	return wrapDB("写入设置", r.db.Save(&domain.SettingDO{Key: key, Value: value}).Error)
}

// SetSettings 批量写入设置值：成组的设置必须一次落库，
// 只写了一半会得到一个「两个设置互相矛盾」的界面状态。
func (r *Repo) SetSettings(kv map[string]string) error {
	return r.WithTx(func(tx *Repo) error {
		for k, v := range kv {
			if err := tx.SetSetting(k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

// AllSettings 取全部设置。
func (r *Repo) AllSettings() (map[string]string, error) {
	var list []domain.SettingDO
	if err := r.db.Find(&list).Error; err != nil {
		return nil, wrapDB("读取设置", err)
	}
	out := make(map[string]string, len(list))
	for _, s := range list {
		out[s.Key] = s.Value
	}
	return out, nil
}

// AddUsage 记一行上游调用用量。
func (r *Repo) AddUsage(u *domain.TokenUsageDO) error {
	return wrapDB("记录用量", r.db.Create(u).Error)
}
