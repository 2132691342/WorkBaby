// Package service 是业务编排层：model.json / mcp.json 文件为源的配置同步。
package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// ProvidersFromFile 解析 model.json 为 AiProviderDO 列表（APIKey 保持明文，落库前加密）。
func ProvidersFromFile(path string) ([]domain.AiProviderDO, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f domain.ModelConfigFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, pkg.Wrap(2004, "parse model.json failed", err)
	}
	out := make([]domain.AiProviderDO, 0, len(f.Providers))
	for _, p := range f.Providers {
		id := p.ID
		if id == "" {
			id = pkg.NewID(domain.IDProvider)
		}
		enabled := true
		if p.Enabled != nil {
			enabled = *p.Enabled
		}
		ratio := p.CompressRatio
		if ratio <= 0 {
			ratio = defaultCompressionRatio
		}
		out = append(out, domain.AiProviderDO{
			ID:                id,
			Name:              p.Name,
			Kind:              p.Kind,
			APIKey:            p.APIKey,
			BaseURL:           p.BaseURL,
			Model:             p.Model,
			Alias:             p.Alias,
			Tier:              p.Tier,
			Enabled:           enabled,
			ContextWindow:     p.ContextWindow,
			MaxOutputTokens:   p.MaxOutputTokens,
			CompressRatio:     ratio,
			Temperature:       p.Temperature,
			TopP:              p.TopP,
			ThinkingEffort:    p.ThinkingEffort,
			ThinkingStyle:     p.ThinkingStyle,
			SupportsToolCall:  p.SupportsToolCall,
			SupportsVision:    p.SupportsVision,
			SupportsReasoning: p.SupportsReasoning,
		})
	}
	return out, nil
}

// SyncFromList 把文件解析的 Provider 列表同步进 DB：
//   - 已存在（按 id）→ upsert 更新（APIKey 非空才覆盖）
//   - 新增 → Create
//   - DB 存在但文件缺失 → 标记 disabled（保留历史，不删除）
func (s *ProviderService) SyncFromList(ctx context.Context, rows []domain.AiProviderDO) error {
	existing, err := s.r.List(ctx)
	if err != nil {
		return err
	}
	existingByID := make(map[string]*domain.AiProviderDO, len(existing))
	for i := range existing {
		existingByID[existing[i].ID] = &existing[i]
	}
	seen := make(map[string]bool, len(rows))
	for i := range rows {
		p := rows[i]
		seen[p.ID] = true
		if old, ok := existingByID[p.ID]; ok {
			// 更新已存在行
			old.Name = p.Name
			old.Kind = p.Kind
			old.BaseURL = p.BaseURL
			old.Model = p.Model
			old.Alias = p.Alias
			old.Tier = p.Tier
			old.Enabled = p.Enabled
			old.ContextWindow = p.ContextWindow
			old.MaxOutputTokens = p.MaxOutputTokens
			old.CompressRatio = p.CompressRatio
			old.Temperature = p.Temperature
			old.TopP = p.TopP
			old.ThinkingEffort = p.ThinkingEffort
			old.ThinkingStyle = p.ThinkingStyle
			old.SupportsToolCall = p.SupportsToolCall
			old.SupportsVision = p.SupportsVision
			old.SupportsReasoning = p.SupportsReasoning
			if p.APIKey != "" {
				ct, cerr := s.encryptAPIKey(p.APIKey)
				if cerr != nil {
					return pkg.Wrap(2028, "encrypt api key failed", cerr)
				}
				old.APIKey = ct
			}
			if uerr := s.r.Update(ctx, old); uerr != nil {
				return uerr
			}
			continue
		}
		// 新增
		ct, cerr := s.encryptAPIKey(p.APIKey)
		if cerr != nil {
			return pkg.Wrap(2028, "encrypt api key failed", cerr)
		}
		p.APIKey = ct
		if cerr := s.r.Create(ctx, &p); cerr != nil {
			return cerr
		}
	}
	// 文件缺失 → disabled（保留历史）
	for id, old := range existingByID {
		if !seen[id] && old.Enabled {
			old.Enabled = false
			if uerr := s.r.Update(ctx, old); uerr != nil {
				return uerr
			}
		}
	}
	return nil
}

// McpServersFromFile 解析 mcp.json 为 McpServerDO 列表。
func McpServersFromFile(path string) ([]domain.McpServerDO, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f domain.McpConfigFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, pkg.Wrap(2014, "parse mcp.json failed", err)
	}
	servers := f.Servers
	if len(f.McpServers) > 0 {
		servers = make([]domain.McpConfigEntry, 0, len(f.McpServers))
		for name, entry := range f.McpServers {
			entry.Name = name
			servers = append(servers, entry)
		}
	}
	out := make([]domain.McpServerDO, 0, len(servers))
	for i := range servers {
		s := &servers[i]
		if s.Transport == "" {
			s.Transport = domain.McpTransportStdio
		}
		if s.Enabled == nil {
			enabled := true
			s.Enabled = &enabled
		}
		id := s.ID
		if id == "" {
			id = pkg.NewID(domain.IDMcpServer)
		}
		args, _ := json.Marshal(s.Args)
		env, _ := json.Marshal(s.Env)
		out = append(out, domain.McpServerDO{
			ID:        id,
			Name:      s.Name,
			Transport: s.Transport,
			Command:   s.Command,
			Args:      string(args),
			Env:       string(env),
			BaseURL:   s.BaseURL,
			Enabled:   *s.Enabled,
		})
	}
	return out, nil
}

// SyncMcpFromList 把 mcp.json 的 server 列表同步进 DB。
// 按 name upsert（复用 repo.Upsert 语义：新增/更新/取消软删）。
// DB 中存在但文件缺失 → enabled=false（保留历史，不删除）。
func (s *McpService) SyncMcpFromList(ctx context.Context, rows []domain.McpServerDO) error {
	existing, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	fileNames := make(map[string]bool, len(rows))
	for _, r := range rows {
		fileNames[r.Name] = true
		if err := s.repo.Upsert(ctx, &r); err != nil {
			return err
		}
	}
	for _, e := range existing {
		if !fileNames[e.Name] && e.Enabled {
			e.Enabled = false
			if err := s.repo.Update(ctx, &e); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveModelFile 把 DB 全部 Provider 写回 model.json（含解密后的明文 APIKey）。
// 这是「文件为源 + DB 同步」闭环中 DB → 文件的另一半：
// UI 里增删改 Provider 后必须落文件，否则下次启动 SyncFromList 会把文件缺失的 provider 禁用。
func (s *ProviderService) SaveModelFile(ctx context.Context, path string) error {
	ps, err := s.r.List(ctx)
	if err != nil {
		return err
	}
	file := domain.ModelConfigFile{Version: 1, Providers: make([]domain.ModelProvider, 0, len(ps))}
	for i := range ps {
		p := ps[i]
		enabled := p.Enabled
		mp := domain.ModelProvider{
			ID:                p.ID,
			Name:              p.Name,
			Kind:              p.Kind,
			BaseURL:           p.BaseURL,
			Model:             p.Model,
			Alias:             p.Alias,
			Tier:              p.Tier,
			Enabled:           &enabled,
			ContextWindow:     p.ContextWindow,
			MaxOutputTokens:   p.MaxOutputTokens,
			CompressRatio:     p.CompressRatio,
			Temperature:       p.Temperature,
			TopP:              p.TopP,
			ThinkingEffort:    p.ThinkingEffort,
			ThinkingStyle:     p.ThinkingStyle,
			SupportsToolCall:  p.SupportsToolCall,
			SupportsVision:    p.SupportsVision,
			SupportsReasoning: p.SupportsReasoning,
		}
		if p.APIKey != "" {
			if pt, derr := s.decryptAPIKey(p.APIKey); derr == nil {
				mp.APIKey = pt
			}
		}
		file.Providers = append(file.Providers, mp)
	}
	bs, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return pkg.Wrap(2004, "marshal model.json failed", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bs, 0o600); err != nil {
		return pkg.Wrap(2004, "write model.json failed", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return pkg.Wrap(2004, "rename model.json failed", err)
	}
	return nil
}

// McpRawPath 计算 mcp.json 数据文件路径；空路径表示尚未初始化。
func McpRawPath(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "mcp.json")
}

// ModelRawPath 计算 model.json 数据文件路径。
func ModelRawPath(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "model.json")
}
