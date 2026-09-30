package service

import (
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/tool"
)

// ToolService 把工具注册表投影成界面可读的清单，并承接工具启停。
type ToolService struct {
	env *Env
}

// NewToolService 构造工具服务。
func NewToolService(env *Env) *ToolService { return &ToolService{env: env} }

// Catalog 列出全部内置工具：名称、用途、风险、参数与当前启用状态。
// 清单由注册表实时投影，停用某个工具后这一项立刻变成未启用，
// 不会出现「文档说有、实际没有」的两份真相。
func (s *ToolService) Catalog() []domain.ToolVO {
	disabled := s.disabled()
	out := make([]domain.ToolVO, 0, 16)
	for _, t := range s.env.Registry.List() {
		out = append(out, domain.ToolVO{
			Name:        t.Name(),
			Label:       t.Label(),
			Category:    tool.CategoryOf(t),
			Description: t.Description(),
			Risk:        riskOfTool(t),
			Approval:    t.RequiresApproval(),
			Mode:        string(t.ExecutionMode()),
			Params:      tool.ParamNames(t.Parameters()),
			Enabled:     !disabled[t.Name()],
			Builtin:     true,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// SetEnabled 启停单个工具。停用只影响下一次 run 的工具集，
// 已经跑起来的回合不打断——半途中少一个工具比多一个更难排查。
func (s *ToolService) SetEnabled(name string, enabled bool) error {
	if _, ok := s.env.Registry.Get(name); !ok {
		return domain.ErrToolNotFound
	}
	cur := s.disabled()
	if enabled {
		delete(cur, name)
	} else {
		cur[name] = true
	}
	names := make([]string, 0, len(cur))
	for n := range cur {
		names = append(names, n)
	}
	sort.Strings(names)
	return s.env.Repo.SetSetting(domain.SettingDisabledTools, strings.Join(names, ","))
}

// disabled 读停用名单。
func (s *ToolService) disabled() map[string]bool {
	out := map[string]bool{}
	raw, _ := s.env.Repo.GetSetting(domain.SettingDisabledTools)
	for _, n := range strings.Split(raw, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out[n] = true
		}
	}
	return out
}

// ModelCapability 查模型能力画像；模型设置里的手填值优先于内置目录。
func (c *ChatService) ModelCapability(providerID, model string) domain.ModelCapability {
	return c.capabilityOf(providerID, model)
}

// riskOfTool 给工具一个静态风险档，供界面在停用前提醒用户。
func riskOfTool(t tool.Tool) string {
	switch t.Name() {
	case "powershell":
		return domain.RiskHigh
	case "write", "edit", "python":
		return domain.RiskMedium
	default:
		return domain.RiskLow
	}
}
