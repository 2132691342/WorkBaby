package service

import "WorkBaby/backend/domain"

// ConfigFiles 可查看的配置文件清单（白名单）。
func (a *App) ConfigFiles() []domain.ConfigFileVO { return configFiles(a.Layout) }

// ConfigText 读配置文件原文。
func (a *App) ConfigText(name string) (domain.ConfigTextRESP, error) {
	return ConfigText(a.Layout, name)
}
