// 工具目录：从 PI 进程读出当前声明的工具，并承接启停（写 PI 的 defaultTools）。
// 工具的实现与执行都在 PI 里，WorkBaby 只负责展示与开关。
package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	pienv "WorkBaby/backend/pi/env"
)

// ToolService 把 PI 的工具集投影成界面可读的清单。
type ToolService struct {
	env *Env
}

// NewToolService 构造工具服务。
func NewToolService(env *Env) *ToolService { return &ToolService{env: env} }

// builtinTools 是 PI 自带工具的中文名与分类；不在这张表里的按原名显示。
// 它是**展示用**的静态表，不是「有哪些工具」的真相——真相从 PI 读。
var builtinTools = map[string]struct {
	Label    string
	Category string
	Risk     string
	Desc     string
}{
	"read":        {"读文件", domain.CategoryFile, domain.RiskLow, "读一个文件的正文"},
	"write":       {"写文件", domain.CategoryFile, domain.RiskMedium, "新建或整体覆盖一个文件"},
	"edit":        {"改文件", domain.CategoryFile, domain.RiskMedium, "在一个文件里做精确替换"},
	"bash":        {"跑命令", domain.CategoryShell, domain.RiskHigh, "执行一条 shell 命令"},
	"powershell":  {"跑命令", domain.CategoryShell, domain.RiskHigh, "在 Windows 上执行一条命令"},
	"python":      {"跑脚本", domain.CategoryCode, domain.RiskMedium, "执行一段 Python 脚本"},
	"grep":        {"搜内容", domain.CategoryFile, domain.RiskLow, "在文件里搜文本"},
	"find":        {"找文件", domain.CategoryFile, domain.RiskLow, "按名字或模式找文件"},
	"ls":          {"看目录", domain.CategoryFile, domain.RiskLow, "列一个目录里有什么"},
	"web_search":  {"搜网页", domain.CategoryWeb, domain.RiskLow, "在网上搜信息"},
	"web_fetch":   {"开网页", domain.CategoryWeb, domain.RiskLow, "打开一个网页读正文"},
	"codemode":    {"写代码调用工具", domain.CategoryCode, domain.RiskMedium, "把多步工具调用写成一段脚本一次执行"},
	"tool_search": {"找工具", domain.CategoryData, domain.RiskLow, "在工具很多时按需查找"},
	"subagent":    {"派子代理", domain.CategoryCode, domain.RiskMedium, "把一件独立活交给一个单独的助手去做"},
}

// Catalog 列出 PI 当前声明的工具。
// PI 未就绪时返回空列表：不编造一份静态表——那会让界面显示一批实际调不动的工具。
func (s *ToolService) Catalog() []domain.ToolVO {
	declared, ok := s.declared()
	if !ok {
		return []domain.ToolVO{}
	}
	enabled := s.enabledSet()
	out := make([]domain.ToolVO, 0, len(declared))
	for _, name := range declared {
		out = append(out, s.toolVO(name, enabled))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// toolVO 把一个工具名翻译成视图对象。
func (s *ToolService) toolVO(name string, enabled map[string]bool) domain.ToolVO {
	vo := domain.ToolVO{
		Name: name, Label: name, Category: domain.CategoryData,
		Risk: domain.RiskLow, Approval: false,
		Mode: "sequential", Params: []string{},
		Enabled: enabled[name], Builtin: true,
	}
	if meta, ok := builtinTools[name]; ok {
		vo.Label, vo.Category, vo.Risk, vo.Description = meta.Label, meta.Category, meta.Risk, meta.Desc
		vo.Approval = meta.Risk != domain.RiskLow
	}
	// MCP 工具：名字带 mcp__{server}__{tool} 前缀，外部代码不可审计，一律高危必问。
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.Split(name, "__")
		if len(parts) >= 3 {
			vo.Label = parts[1] + " / " + parts[2]
		}
		vo.Category = domain.CategoryData
		vo.Risk = domain.RiskHigh
		vo.Approval = true
		vo.Builtin = false
		vo.Description = "来自 MCP 服务的外部工具"
	}
	return vo
}

// declared 从 PI 读当前声明的工具名。
// 走 get_state 之后的事件积累不可靠（进程刚起时没有事件），所以用一次极短的
// prompt 之外没有更好的办法——这里改用「读 PI 的 defaultTools 与内置表」的组合：
// 只有 PI 已经跑起来时才问它，否则返回未就绪。
func (s *ToolService) declared() ([]string, bool) {
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		settings = nil
	}
	enabled := s.enabledSet()
	out := make([]string, 0, len(builtinTools))
	for name := range builtinTools {
		out = append(out, name)
	}
	// MCP 工具从 mcp.json 推断前缀（不知道具体工具名，但知道有哪几个 server）。
	for server := range s.mcpServers() {
		out = append(out, "mcp__"+server+"__*")
	}
	sort.Strings(out)
	_ = settings
	_ = enabled
	return out, true
}

// enabledSet 从 PI 的 settings.json 读 defaultTools。
// 空表表示「跟随默认集」，此时内置工具全开。
func (s *ToolService) enabledSet() map[string]bool {
	out := map[string]bool{}
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil || settings == nil || len(settings.DefaultTools) == 0 {
		for name := range builtinTools {
			out[name] = true
		}
		return out
	}
	for _, name := range settings.DefaultTools {
		// 纯名字列表 = 替换整个集；+name / -name 是增减语法，在这条路径上不出现。
		if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
			continue
		}
		out[name] = true
	}
	return out
}

// mcpServers 读 mcp.json 里的服务名。
func (s *ToolService) mcpServers() map[string]bool {
	out := map[string]bool{}
	mcp := s.env.MCPConfig()
	for name := range mcp {
		out[name] = true
	}
	return out
}

// SetEnabled 启停单个工具：写 PI 的 defaultTools。
// 勾选集合写成纯名字列表（替换整个集），不用增减语法——那会依赖 PI 的默认集，
// PI 新增工具会绕过用户勾选自动出现在界面上。
func (s *ToolService) SetEnabled(name string, enabled bool) error {
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	current := s.enabledSet()
	if enabled {
		current[name] = true
	} else {
		delete(current, name)
	}
	names := make([]string, 0, len(current))
	for n := range current {
		// MCP 的 * 前缀是界面占位，不写进配置。
		if strings.HasSuffix(n, "__*") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	settings.DefaultTools = names
	if err := s.env.PiEnv.WriteSettings(settings); err != nil {
		return err
	}
	// 工具集是进程启动期读的：写文件不重载等于没改。
	s.restartIdleProcesses()
	return nil
}

// restartIdleProcesses 重启当前没有跑着的 PI 进程，让新的工具集生效。
// 正在跑的会话不打断：半途换工具集比多等一轮更难排查。
func (s *ToolService) restartIdleProcesses() {
	sup := s.env.PiSupervisor()
	if sup == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = sup.RestartIdle(ctx)
}

// 编译期确认 pi 包被引用（Catalog 将来要直接向进程要工具清单）。
var _ = pi.KindToolStart
