package api

import (
	"path/filepath"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/service"
)

// ListProviders 列出全部 Provider（含 disabled）。
func (h *Handler) ListProviders() ([]domain.AiProviderRESP, error) {
	return h.provSvc.List(h.ctx)
}

// CreateProvider 新建一个 LLM Provider。
func (h *Handler) CreateProvider(req domain.AiProviderREQ) (domain.AiProviderRESP, error) {
	p, err := h.provSvc.Create(h.ctx, &req)
	if err != nil {
		return domain.AiProviderRESP{}, err
	}
	// 配置闭环：UI 新增同样落 model.json，并即时重建 Registry（否则重启被文件同步禁用/运行期不可用）
	h.syncProviders()
	return *p, nil
}

// UpdateProvider 更新 Provider；id 路径。
func (h *Handler) UpdateProvider(id string, req domain.AiProviderREQ) (domain.AiProviderRESP, error) {
	p, err := h.provSvc.Update(h.ctx, id, &req)
	if err != nil {
		return domain.AiProviderRESP{}, err
	}
	h.syncProviders()
	return *p, nil
}

// DeleteProvider 删除 Provider。
func (h *Handler) DeleteProvider(id string) error {
	if err := h.provSvc.Delete(h.ctx, id); err != nil {
		return err
	}
	h.syncProviders()
	return nil
}

// GetProvider 取单个 Provider。
func (h *Handler) GetProvider(id string) (domain.AiProviderRESP, error) {
	p, err := h.provSvc.Get(h.ctx, id)
	if err != nil {
		return domain.AiProviderRESP{}, err
	}
	return *p, nil
}

// ListAvailableModels 聊天模型选择器用，只列 enabled。
func (h *Handler) ListAvailableModels() ([]domain.AvailableModelRESP, error) {
	return h.provSvc.ListAvailable(h.ctx)
}

// ListProviderKinds 暴露给前端：所有已实现 ProviderKind + 展示元数据。
// 关键设计：前端不写死 kind 列表，从后端拉 ——
//
//	加新 ProviderKind 时：domain.AllProviderKindMetas 加一项 + internal/llm/<kind>/ 实现 Provider 接口
//	+ registry.go buildOne 加 case；前端 0 改动即可见
func (h *Handler) ListProviderKinds() (domain.ProviderKindsRESP, error) {
	return domain.AllProviderKindMetas, nil
}

// ListProviderTiers 暴露合法档位取值；前端下拉由此驱动，避免自造取值。
func (h *Handler) ListProviderTiers() ([]domain.ProviderTier, error) {
	return domain.AllProviderTiers, nil
}

// ListProviderPresets 内置常见模型服务预设：前端「新增模型」一键预填 kind/base_url/models，
// 新增一个兼容 provider 不需要写任何代码。
func (h *Handler) ListProviderPresets() ([]llm.ProviderPreset, error) {
	return llm.ProviderPresets(), nil
}

// TestProviderConnect 校验 Provider 字段完整性并测试连通。
func (h *Handler) TestProviderConnect(id string) error {
	return h.provSvc.TestConnect(h.ctx, id)
}

// GetCircuitStatus 返回全部 provider 的就绪状态（前端模型选择器徽标用）。
// 语义：CLOSED = registry 已构建可用；OPEN = 未构建/构建失败（reason 说明原因）；
// DISABLED = 用户在设置里关闭（不参与构建，也就不该展示成「熔断」）。
func (h *Handler) GetCircuitStatus() ([]domain.ProviderCircuitRESP, error) {
	ps, err := h.app.ProvRepo.List(h.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProviderCircuitRESP, 0, len(ps))
	for i := range ps {
		state := "CLOSED"
		reason := ""
		if !ps[i].Enabled {
			state = "DISABLED"
		} else if !h.reg.IsReady(ps[i].ID) {
			state = "OPEN"
			reason = h.reg.Reason(ps[i].ID)
		}
		out = append(out, domain.ProviderCircuitRESP{
			ID:      ps[i].ID,
			Name:    ps[i].Name,
			Enabled: ps[i].Enabled,
			State:   state,
			Reason:  reason,
		})
	}
	return out, nil
}

// syncProviders 写回 model.json 并重建 Registry（create/update/delete 后统一调用）。
// model.json 是配置真相源：仅写 DB 不改文件，重启时 SyncFromList 会把文件缺失的 provider 禁用。
func (h *Handler) syncProviders() {
	if h.paths == nil || h.paths.Home == "" {
		return
	}
	if err := h.provSvc.SaveModelFile(h.ctx, filepath.Join(h.paths.Home, "model.json")); err != nil {
		pkg.L.Warn("save model.json failed", "err", err)
	}
	if pwds, lerr := h.app.ProvRepo.List(h.ctx); lerr == nil {
		_ = h.reg.Build(pwds, func(encrypted string) (string, error) {
			return h.cipher.Decrypt(encrypted)
		})
	}
}

// ResetCircuit 重置指定 provider 的熔断（重新构建实例并标记就绪）。
func (h *Handler) ResetCircuit(id string) (map[string]any, error) {
	p, err := h.app.ProvRepo.GetByID(h.ctx, id)
	if err != nil {
		return nil, err
	}
	if err := h.reg.Reload(*p, func(enc string) (string, error) { return h.cipher.Decrypt(enc) }); err != nil {
		return map[string]any{"reset": false}, nil
	}
	return map[string]any{"reset": true}, nil
}

// ReloadProvidersFromFile 重新读取 model.json → 同步 ai_providers 表 → 重建 LLM Registry。
// 双主机配置热更新链路。
func (h *Handler) ReloadProvidersFromFile() (map[string]any, error) {
	if h.paths == nil || h.paths.Home == "" {
		return nil, pkg.New(2005, "home path not ready", "")
	}
	path := filepath.Join(h.paths.Home, "model.json")
	rows, err := service.ProvidersFromFile(path)
	if err != nil {
		return nil, pkg.Wrap(2004, "parse model.json failed", err)
	}
	if err := h.provSvc.SyncFromList(h.ctx, rows); err != nil {
		return nil, pkg.Wrap(2005, "sync model.json to db failed", err)
	}
	// 重建 Registry（含解密）
	if pwds, lerr := h.app.ProvRepo.List(h.ctx); lerr == nil {
		_ = h.reg.Build(pwds, func(encrypted string) (string, error) {
			return h.cipher.Decrypt(encrypted)
		})
	}
	return map[string]any{"synced": len(rows)}, nil
}
