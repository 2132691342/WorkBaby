// 工具面板：清单来自 PI（最近一次会话写给模型的工具声明 + MCP 在线工具），
// 启停写的是 PI settings.json 的 defaultTools。WorkBaby 不自己实现任何工具。
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

// builtinTools 是 PI 自带工具的中文名与说明；它只是**展示**用的静态表，
// 「当前有哪些工具」的真相来自会话里的工具声明。
var builtinTools = map[string]struct{ Label, Desc string }{
	"read":        {"读文件", "读一个文件的正文"},
	"write":       {"写文件", "新建或整体覆盖一个文件"},
	"edit":        {"改文件", "在一个文件里做精确替换"},
	"bash":        {"跑命令", "执行一条 shell 命令"},
	"powershell":  {"跑命令", "在 Windows 上执行一条命令"},
	"grep":        {"搜内容", "在文件里搜文本"},
	"find":        {"找文件", "按名字或模式找文件"},
	"ls":          {"看目录", "列一个目录里有什么"},
	"codemode":    {"写代码调用工具", "把多步工具调用写成一段脚本一次执行"},
	"tool_search": {"找工具", "在工具很多时按需查找"},
}

// builtinOrder 是没有会话快照时的兜底顺序。
var builtinOrder = []string{"read", "write", "edit", "bash", "powershell", "grep", "find", "ls", "codemode", "tool_search"}

// defaultTools 是 PI 在没有 defaultTools 配置时的默认集。
var defaultTools = []string{"read", "bash", "edit", "write"}

// Catalog 列出 PI 当前声明的工具。
func (s *ToolService) Catalog() *domain.ToolListRESP {
	enabled := s.enabledSet()
	resp := &domain.ToolListRESP{Tools: []domain.ToolVO{}, Snapshot: "static"}

	// 找最近一条带工具声明的会话：那就是模型此刻真能调到的工具。
	metas, _ := pi.ListSessions(s.env.PiEnv.SessionsDir())
	var declared []pi.SessionTool
	for _, m := range metas {
		if len(m.Tools) > 0 {
			declared, resp.SnapshotAt = m.Tools, m.UpdatedAt
			resp.Snapshot = "session"
			break
		}
	}

	seen := map[string]bool{}
	for _, t := range declared {
		if strings.HasPrefix(t.Name, "mcp__") {
			continue // MCP 工具从在线服务取，清单更完整
		}
		seen[t.Name] = true
		resp.Tools = append(resp.Tools, s.toolVO(t.Name, t.Description, enabled))
	}
	if len(declared) == 0 {
		for _, name := range builtinOrder {
			seen[name] = true
			resp.Tools = append(resp.Tools, s.toolVO(name, "", enabled))
		}
	}
	for _, t := range NewMCPService(s.env).MCPTools() {
		if seen[t.Name] {
			continue
		}
		resp.Tools = append(resp.Tools, t)
	}
	sort.SliceStable(resp.Tools, func(i, j int) bool {
		if resp.Tools[i].Source != resp.Tools[j].Source {
			return resp.Tools[i].Source < resp.Tools[j].Source
		}
		return resp.Tools[i].Name < resp.Tools[j].Name
	})
	return resp
}

// toolVO 把一个工具名翻译成视图对象。
func (s *ToolService) toolVO(name, desc string, enabled map[string]bool) domain.ToolVO {
	vo := domain.ToolVO{Name: name, Description: desc, Source: domain.ToolSourceExtension}
	if meta, ok := builtinTools[name]; ok {
		vo.Source = domain.ToolSourceBuiltin
		label := meta.Label
		vo.Description = "「" + label + "」 " + meta.Desc
	}
	if vo.Description == "" {
		vo.Description = "由插件注册的工具"
	}
	vo.Enabled = enabled[name]
	return vo
}

// enabledSet 按 PI 的语义算出启用的工具集。
// defaultTools 为空 = 默认集；纯名字列表 = 替换；只有 +name / -name = 在默认集上增减。
func (s *ToolService) enabledSet() map[string]bool {
	out := map[string]bool{}
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil || settings == nil || len(settings.DefaultTools) == 0 {
		for _, name := range defaultTools {
			out[name] = true
		}
		// 没有显式配置时，声明过的扩展工具默认是启用的（注册即激活）。
		return out
	}
	var plain, add, remove []string
	for _, raw := range settings.DefaultTools {
		name := strings.TrimSpace(raw)
		switch {
		case name == "":
		case strings.HasPrefix(name, "+"):
			add = append(add, strings.TrimPrefix(name, "+"))
		case strings.HasPrefix(name, "-"):
			remove = append(remove, strings.TrimPrefix(name, "-"))
		default:
			plain = append(plain, name)
		}
	}
	if len(plain) > 0 {
		for _, name := range plain {
			out[name] = true
		}
		for _, name := range add {
			out[name] = true
		}
		for _, name := range remove {
			delete(out, name)
		}
		return out
	}
	for _, name := range defaultTools {
		out[name] = true
	}
	for _, name := range add {
		out[name] = true
	}
	for _, name := range remove {
		delete(out, name)
	}
	return out
}

// SetEnabled 启停单个工具：写 PI 的 defaultTools，用纯名字列表替换整个集。
// 增减语法会依赖 PI 的默认集，让新增工具绕过用户勾选自动出现在界面上。
func (s *ToolService) SetEnabled(req domain.ToggleToolREQ) error {
	if strings.HasPrefix(req.Name, "mcp__") {
		// MCP 工具的开关在 PI 里属于暴露档（mcp.json 的 exposure），不归 defaultTools。
		return nil
	}
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	current := s.enabledSet()
	// 声明过但没被算进来的工具（例如扩展工具）也要进列表，否则一保存就丢了。
	declared, _ := pi.ListSessions(s.env.PiEnv.SessionsDir())
	for _, m := range declared {
		for _, t := range m.Tools {
			if strings.HasPrefix(t.Name, "mcp__") {
				continue
			}
			if _, ok := current[t.Name]; !ok {
				current[t.Name] = false
			}
		}
		if len(m.Tools) > 0 {
			break
		}
	}
	if req.Enabled {
		current[req.Name] = true
	} else {
		delete(current, req.Name)
	}
	names := make([]string, 0, len(current))
	for name, on := range current {
		if on {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	settings.DefaultTools = names
	if err := s.env.PiEnv.WriteSettings(settings); err != nil {
		return err
	}
	// 工具集是进程启动期读的：写文件不重载等于没改。
	sup := s.env.PiSupervisor()
	if sup == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return sup.RestartIdle(ctx)
}
