package repo

import (
	"WorkBaby/internal/domain"
)

// GetSetting 取一个设置值；不存在返回空串。
func (r *Repo) GetSetting(key string) (string, error) {
	var s domain.SettingDO
	if err := r.db.Where("`key` = ?", key).First(&s).Error; err != nil {
		if notFound(err) {
			return "", nil
		}
		return "", err
	}
	return s.Value, nil
}

// SetSetting 写入设置值。
func (r *Repo) SetSetting(key, value string) error {
	return r.db.Save(&domain.SettingDO{Key: key, Value: value}).Error
}

// AllSettings 取全部设置。
func (r *Repo) AllSettings() (map[string]string, error) {
	var list []domain.SettingDO
	if err := r.db.Find(&list).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, s := range list {
		out[s.Key] = s.Value
	}
	return out, nil
}

// AddUsage 记一行上游调用用量。
func (r *Repo) AddUsage(u *domain.TokenUsageDO) error {
	return r.db.Create(u).Error
}

// SumSessionTokens 汇总会话累计 token，用于列表页展示。
func (r *Repo) SumSessionTokens(sessionID string) (int, error) {
	var total int
	err := r.db.Model(&domain.TokenUsageDO{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(SUM(total), 0)").Scan(&total).Error
	return total, err
}
