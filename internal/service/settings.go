package service

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// SettingsService KV 运行时配置（system_settings 表）+ SMTP/websearch 组装。
type SettingsService struct {
	r      *repo.SystemSettingRepo
	cipher *pkg.Cipher
}

// NewSettingsService 构造；cipher 用于 SMTP 密码加解密。
func NewSettingsService(r *repo.SystemSettingRepo, cipher *pkg.Cipher) *SettingsService {
	return &SettingsService{r: r, cipher: cipher}
}

func (s *SettingsService) Get(ctx context.Context, k string) (*domain.SystemSettingRESP, error) {
	row, err := s.r.Get(ctx, k)
	if err != nil {
		return nil, err
	}
	return &domain.SystemSettingRESP{K: row.K, V: row.V, UpdatedAt: row.UpdatedAt}, nil
}

func (s *SettingsService) Set(ctx context.Context, k, v string) (*domain.SystemSettingRESP, error) {
	if err := s.r.Set(ctx, k, v); err != nil {
		return nil, err
	}
	return s.Get(ctx, k)
}

func (s *SettingsService) ListAll(ctx context.Context) ([]domain.SystemSettingRESP, error) {
	rows, err := s.r.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SystemSettingRESP, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.SystemSettingRESP{K: r.K, V: r.V, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

// GetWebSearchConfig 组装 websearch 配置。
func (s *SettingsService) GetWebSearchConfig(ctx context.Context) (domain.WebSearchConfigRESP, error) {
	get := func(k, def string) string {
		row, err := s.r.Get(ctx, k)
		if err != nil {
			return def
		}
		return row.V
	}
	return domain.WebSearchConfigRESP{
		Enabled: get(domain.SettingKeyWebSearchEnabled, "true"),
		Engine:  get(domain.SettingKeyWebSearchEngine, "duckduckgo"),
		APIKey:  "",
	}, nil
}

// SaveWebSearchConfig 保存 websearch 配置。
func (s *SettingsService) SaveWebSearchConfig(ctx context.Context, req domain.WebSearchConfigRESP) error {
	vals := map[string]string{
		domain.SettingKeyWebSearchEnabled: req.Enabled,
		domain.SettingKeyWebSearchEngine:  req.Engine,
	}
	for k, v := range vals {
		if err := s.r.Set(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetGeneral 读取通用设置（JSON 值）；缺失返回空 map。
func (s *SettingsService) GetGeneral(ctx context.Context) (map[string]any, error) {
	row, err := s.r.Get(ctx, domain.SettingKeyGeneral)
	if err != nil {
		if err == domain.ErrSettingNotFound {
			return map[string]any{}, nil
		}
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(row.V), &out); err != nil {
		return map[string]any{}, nil
	}
	return out, nil
}

// SaveGeneral 保存通用设置（JSON 值），返回规范化后的 map。
func (s *SettingsService) SaveGeneral(ctx context.Context, v map[string]any) (map[string]any, error) {
	bs, err := json.Marshal(v)
	if err != nil {
		return nil, pkg.Wrap(2020, "marshal general settings failed", err)
	}
	if err := s.r.Set(ctx, domain.SettingKeyGeneral, string(bs)); err != nil {
		return nil, err
	}
	return v, nil
}

// GetExecWhitelist 读取 exec 工具二进制白名单（JSON 数组；未配置返回空列表）。
func (s *SettingsService) GetExecWhitelist(ctx context.Context) ([]string, error) {
	row, err := s.r.Get(ctx, domain.SettingKeyExecWhitelist)
	if err != nil {
		if err == domain.ErrSettingNotFound {
			return []string{}, nil
		}
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(row.V), &out); err != nil {
		return []string{}, nil
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// SaveExecWhitelist 保存 exec 工具二进制白名单并返回规范化结果。
func (s *SettingsService) SaveExecWhitelist(ctx context.Context, bins []string) ([]string, error) {
	if bins == nil {
		bins = []string{}
	}
	bs, err := json.Marshal(bins)
	if err != nil {
		return nil, pkg.Wrap(2020, "marshal exec whitelist failed", err)
	}
	if err := s.r.Set(ctx, domain.SettingKeyExecWhitelist, string(bs)); err != nil {
		return nil, err
	}
	return bins, nil
}
