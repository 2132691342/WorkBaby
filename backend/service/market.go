// 插件市场编排：按 PI 官方发现机制（npm 关键字 pi-package）列出可装包，并与本地已装对账。
// 安装 / 卸载直接复用插件服务，市场只是发现层，不引入第二套包规则。
package service

import (
	"WorkBaby/backend/domain"
	pienv "WorkBaby/backend/pi/env"
)

// MarketService 是插件市场的数据出口。
type MarketService struct {
	env *Env
}

// NewMarketService 构造市场服务。
func NewMarketService(env *Env) *MarketService { return &MarketService{env: env} }

// List 拉市场清单并对账已装状态。注册表连不上不算失败：清单给空、Error 带原因。
func (s *MarketService) List(refresh bool) *domain.MarketListRESP {
	resp := &domain.MarketListRESP{Packages: []domain.MarketPackageVO{}}
	entries, err := pienv.SearchMarket(refresh)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	installed := map[string]string{}
	for _, pkg := range resolvePackages(s.env) {
		if pkg.Kind == domain.PackageKindNpm && pkg.Name != "" {
			installed[pkg.Name] = pkg.Version
		}
	}
	resp.Packages = reconcileMarket(entries, installed)
	return resp
}

// reconcileMarket 把注册表条目与已装清单（npm 包名 → 本地版本）对账：
// 装了的没标出来，用户会在市场里重复安装。
func reconcileMarket(entries []pienv.MarketEntry, installed map[string]string) []domain.MarketPackageVO {
	out := make([]domain.MarketPackageVO, 0, len(entries))
	for _, entry := range entries {
		item := domain.MarketPackageVO{
			Name: entry.Name, Description: entry.Description, Version: entry.Version,
			Publisher: entry.Publisher, Repo: entry.Repo, PublishedAt: entry.PublishedAt,
		}
		if v, ok := installed[entry.Name]; ok {
			item.Installed, item.InstalledVersion = true, v
		}
		out = append(out, item)
	}
	return out
}
