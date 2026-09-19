package api

import (
	"WorkBaby/internal/domain"
)

// TrustService 是 Handler 暴露给 router 与 Wails 绑定的信任服务访问器
// （避免 handler.go 字段被外部包直接引用）。

// ListTrust 全部信任登记（设置页数据源）。
func (h *Handler) ListTrust() ([]domain.WorkspaceTrustRESP, error) {
	return h.trustSvc.List(h.ctx)
}

// ResolveTrust 解析某目录的信任态（不落库；前端实时查询时使用）。
func (h *Handler) ResolveTrust(path string) (domain.TrustResolveRESP, error) {
	return h.trustSvc.Resolve(h.ctx, path)
}

// DecideTrust 写入信任决策（allow / ask / deny）。
func (h *Handler) DecideTrust(req domain.WorkspaceTrustREQ) (domain.WorkspaceTrustRESP, error) {
	return h.trustSvc.Decide(h.ctx, req)
}

// RevokeTrust 撤销某目录的信任登记（回到默认 ask）。
func (h *Handler) RevokeTrust(path string) error {
	return h.trustSvc.Revoke(h.ctx, path)
}

// TrustRoots 恒信任根（前端展示「这些目录默认信任」用）。
func (h *Handler) TrustRoots() []string {
	if h.trustSvc == nil {
		return nil
	}
	return h.trustSvc.Roots()
}
