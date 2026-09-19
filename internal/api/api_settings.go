package api

import (
	"context"

	"WorkBaby/internal/domain"
)

// GetSetting 读单个运行时配置项。
func (h *Handler) GetSetting(key string) (domain.SystemSettingRESP, error) {
	s, err := h.setSvc.Get(h.ctx, key)
	if err != nil {
		return domain.SystemSettingRESP{}, err
	}
	return *s, nil
}

// SetSetting 写入运行时配置项（覆盖式）。
func (h *Handler) SetSetting(key string, value string) (domain.SystemSettingRESP, error) {
	s, err := h.setSvc.Set(h.ctx, key, value)
	if err != nil {
		return domain.SystemSettingRESP{}, err
	}
	return *s, nil
}

// SettingValue 读设置原始字符串（内部使用：如托盘关闭行为；未配置返回空串不报错）。
func (h *Handler) SettingValue(ctx context.Context, key string) (string, error) {
	s, err := h.setSvc.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", nil
	}
	return s.V, nil
}

// ListSettings 全部运行时配置（设置面板"高级"页用）。
func (h *Handler) ListSettings() ([]domain.SystemSettingRESP, error) {
	return h.setSvc.ListAll(h.ctx)
}

// GetWebSearchConfig 读取联网搜索配置。
func (h *Handler) GetWebSearchConfig() (domain.WebSearchConfigRESP, error) {
	return h.setSvc.GetWebSearchConfig(h.ctx)
}

// SaveWebSearchConfig 保存联网搜索配置。
func (h *Handler) SaveWebSearchConfig(req domain.WebSearchConfigRESP) (map[string]any, error) {
	if err := h.setSvc.SaveWebSearchConfig(h.ctx, req); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true}, nil
}

// GetGeneralSettings 读取通用设置（主题 / 背景 / 字体等）。
func (h *Handler) GetGeneralSettings() (map[string]any, error) {
	return h.setSvc.GetGeneral(h.ctx)
}

// SaveGeneralSettings 保存通用设置（JSON 值）。
func (h *Handler) SaveGeneralSettings(req map[string]any) (map[string]any, error) {
	return h.setSvc.SaveGeneral(h.ctx, req)
}

// GetExecWhitelist 读取 exec 工具二进制白名单。
func (h *Handler) GetExecWhitelist() ([]string, error) {
	return h.setSvc.GetExecWhitelist(h.ctx)
}

// SaveExecWhitelist 保存 exec 工具二进制白名单。
func (h *Handler) SaveExecWhitelist(binaries []string) ([]string, error) {
	return h.setSvc.SaveExecWhitelist(h.ctx, binaries)
}
