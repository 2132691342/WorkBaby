// Package service 是业务编排层；不引 wails / api（CLAUDE §2.2）。
package service

import (
	"context"

	"WorkBaby/internal/config"
	"WorkBaby/internal/domain"
)

// MetaService 健康检查 + 版本元信息。
type MetaService struct{ cfg *config.Config }

func NewMetaService(cfg *config.Config) *MetaService { return &MetaService{cfg: cfg} }

func (s *MetaService) GetVersion(_ context.Context) domain.VersionInfo {
	cfg := s.cfg
	return domain.VersionInfo{
		AppName:       cfg.App.Name,
		Version:       cfg.App.Version,
		Phase:         "phase-1-mvp",
		Env:           cfg.App.Env,
		DefaultTenant: "default",
		LocalUserID:   "local",
	}
}

func (s *MetaService) GetHealth(_ context.Context, providerCount int, dbOK bool) domain.HealthInfo {
	return domain.HealthInfo{
		Status:    "ok",
		Phase:     "phase-1-mvp",
		DBEnabled: dbOK,
		Providers: providerCount,
	}
}
