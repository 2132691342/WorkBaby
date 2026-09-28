package service

import (
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
)

// SettingsService 负责键值设置与外观项。
type SettingsService struct {
	env   *Env
	paths runtime.Paths
}

// NewSettingsService 构造设置服务。
func NewSettingsService(env *Env, paths runtime.Paths) *SettingsService {
	return &SettingsService{env: env, paths: paths}
}

// All 取全部设置，缺失项用默认值补齐。
func (s *SettingsService) All() (map[string]string, error) {
	stored, err := s.env.Repo.AllSettings()
	if err != nil {
		return nil, err
	}
	for k, v := range domain.DefaultSettings() {
		if _, ok := stored[k]; !ok {
			stored[k] = v
		}
	}
	if stored[domain.SettingWorkspace] == "" && s.env.Cfg.Workspace != "" {
		stored[domain.SettingWorkspace] = s.env.Cfg.Workspace
	}
	return stored, nil
}

// Set 写入一个设置。
func (s *SettingsService) Set(key, value string) error {
	if key == "" {
		return pkg.New(2006, "设置项名不能为空", "")
	}
	switch key {
	case domain.SettingTheme:
		if value != "light" && value != "dark" {
			return pkg.New(2007, "主题只有晴空和紫夜两种", value)
		}
	case domain.SettingPermission:
		switch value {
		case domain.PermissionAsk, domain.PermissionAutoEdit, domain.PermissionYolo:
		default:
			return pkg.New(1106, "权限档位不正确", value)
		}
	case domain.SettingWorkspace:
		if value != "" && !pkg.DirExists(value) {
			return pkg.New(1003, "这个目录不存在", value)
		}
		if err := s.env.Cfg.SetWorkspace(value); err != nil {
			return err
		}
	}
	return s.env.Repo.SetSetting(key, value)
}

// SeedDefaults 首启写入默认值。
func (s *SettingsService) SeedDefaults() error {
	for k, v := range domain.DefaultSettings() {
		cur, err := s.env.Repo.GetSetting(k)
		if err != nil {
			return err
		}
		if cur == "" {
			if err := s.env.Repo.SetSetting(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
