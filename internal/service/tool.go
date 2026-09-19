package service

import (
	"context"
	"encoding/json"
	"sort"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
)

// ToolService 工具编排：启停状态持久化在 system_settings（tool.enabled.{name}），
// 并负责把启用工具装配为 LLM 可见的 ToolDefinition。
type ToolService struct {
	reg      *tool.Registry
	settings *repo.SystemSettingRepo
}

// NewToolService 注入注册中心与设置仓储。
func NewToolService(reg *tool.Registry, settings *repo.SystemSettingRepo) *ToolService {
	return &ToolService{reg: reg, settings: settings}
}

// ListTools 返回全部已注册工具（含启停状态）。
func (s *ToolService) ListTools(ctx context.Context) ([]domain.ToolMeta, error) {
	ts := s.reg.List()
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name() < ts[j].Name() })
	out := make([]domain.ToolMeta, 0, len(ts))
	for _, t := range ts {
		enabled, err := s.enabled(ctx, t.Name())
		if err != nil {
			enabled = true // 未配置默认启用
		}
		meta := tool.MetaOf(t)
		out = append(out, domain.ToolMeta{
			Name:         t.Name(),
			Description:  t.Description(),
			RiskLevel:    string(t.RiskLevel()),
			Group:        meta.Group,
			Category:     meta.Category,
			ActivityDesc: meta.ActivityDesc,
			ReadOnly:     meta.ReadOnly,
			Destructive:  meta.Destructive,
			Enabled:      enabled,
			Params:       parseParams(t.Schema().Parameters),
			SchemaJSON:   string(t.Schema().Parameters),
		})
	}
	return out, nil
}

// parseParams 从 JSON Schema 提取参数清单；解析失败返回空切片。
func parseParams(raw json.RawMessage) []domain.ToolParamVO {
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &schema) != nil || len(schema.Properties) == 0 {
		return []domain.ToolParamVO{}
	}
	req := make(map[string]bool, len(schema.Required))
	for _, r := range schema.Required {
		req[r] = true
	}
	names := make([]string, 0, len(schema.Properties))
	for n := range schema.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]domain.ToolParamVO, 0, len(names))
	for _, n := range names {
		p := schema.Properties[n]
		out = append(out, domain.ToolParamVO{
			Name:        n,
			Type:        p.Type,
			Required:    req[n],
			Description: p.Description,
		})
	}
	return out
}

// SetToolEnabled 持久化启停。
func (s *ToolService) SetToolEnabled(ctx context.Context, name string, enabled bool) error {
	if _, ok := s.reg.Get(name); !ok {
		return tool.ErrToolNotFound
	}
	v := "false"
	if enabled {
		v = "true"
	}
	return s.settings.Set(ctx, domain.SettingKeyToolEnabledPrefix+name, v)
}

// EnabledTools 返回启用工具列表（供 harness 注入 ToolDefinition）。
func (s *ToolService) EnabledTools(ctx context.Context) []tool.Tool {
	ts := s.reg.List()
	out := make([]tool.Tool, 0, len(ts))
	for _, t := range ts {
		if enabled, err := s.enabled(ctx, t.Name()); err == nil && enabled {
			out = append(out, t)
		}
	}
	return out
}

// LLMDefinitions 启用工具 → LLM ToolDefinition（Parameters map[string]any）。
func (s *ToolService) LLMDefinitions(ctx context.Context) []llm.ToolDefinition {
	return s.LLMDefinitionsFiltered(ctx, nil)
}

// LLMDefinitionsFiltered 按白名单过滤；allowed 为空 = 不过滤（全量启用工具）。
func (s *ToolService) LLMDefinitionsFiltered(ctx context.Context, allowed []string) []llm.ToolDefinition {
	ts := s.EnabledTools(ctx)
	allowSet := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allowSet[n] = true
	}
	defs := make([]llm.ToolDefinition, 0, len(ts))
	for _, t := range ts {
		if len(allowSet) > 0 && !allowSet[t.Name()] {
			continue
		}
		params := map[string]any{}
		raw := t.Schema().Parameters
		if len(raw) > 0 && json.Unmarshal(raw, &params) == nil {
			// params 已是完整 schema
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  params,
		})
	}
	return defs
}

// enabled 读取工具启停状态；未配置时按分组给默认值：functools 默认禁用（数量多、易干扰选择），
// 其余默认启用。已显式配置过的一律以配置为准。
func (s *ToolService) enabled(ctx context.Context, name string) (bool, error) {
	row, err := s.settings.Get(ctx, domain.SettingKeyToolEnabledPrefix+name)
	if err != nil {
		if err == domain.ErrSettingNotFound {
			if t, ok := s.reg.Get(name); ok && tool.MetaOf(t).Group == tool.GroupFunctools {
				return false, nil
			}
			return true, nil
		}
		return false, err
	}
	return row.V == "true", nil
}

// Registry 导出注册中心给 handler/chat 装配。
func (s *ToolService) Registry() *tool.Registry { return s.reg }
