// MCP 编排：配置读写的是 PI 的 mcp.json，在线状态与工具清单来自 `pi mcp list --json`。
// WorkBaby 不再自己维护一份 MCP 清单——用户用 pi mcp add 加的，打开界面就该看到。
package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// MCPService 是 MCP 面板的数据出口。
type MCPService struct {
	env *Env
}

// NewMCPService 构造 MCP 服务。
func NewMCPService(env *Env) *MCPService { return &MCPService{env: env} }

// List 返回服务列表与两份配置原文。
func (s *MCPService) List(workspace string) (*domain.MCPListRESP, error) {
	if workspace == "" {
		workspace = s.env.Workspace()
	}
	globalPath := s.env.PiEnv.MCPPath()
	projectPath := s.env.PiEnv.ProjectMCPPath(workspace)

	globalRaw, err := s.env.PiEnv.MCPRaw(globalPath)
	if err != nil {
		return nil, err
	}
	projectRaw := []byte("{}\n")
	if projectPath != "" {
		if raw, err := s.env.PiEnv.MCPRaw(projectPath); err == nil {
			projectRaw = raw
		}
	}

	servers := []domain.MCPServerVO{}
	servers = append(servers, parseConfigured(globalRaw, "global", globalPath)...)
	if projectPath != "" {
		servers = append(servers, parseConfigured(projectRaw, "project", projectPath)...)
	}
	// 在线状态覆盖配置里的占位：连上了就把工具清单带回来。
	for _, live := range s.live() {
		replaced := false
		for i := range servers {
			if servers[i].Name == live.Name && samePath(servers[i].Source, live.Source) {
				live.Scope = servers[i].Scope
				servers[i] = live
				replaced = true
			}
		}
		if !replaced {
			servers = append(servers, live)
		}
	}

	return &domain.MCPListRESP{
		Servers:     servers,
		GlobalPath:  globalPath,
		ProjectPath: projectPath,
		GlobalRaw:   string(globalRaw),
		ProjectRaw:  string(projectRaw),
	}, nil
}

// parseConfigured 从一份 mcp.json 里抽出服务条目（不带在线状态）。
func parseConfigured(raw []byte, scope, source string) []domain.MCPServerVO {
	out := []domain.MCPServerVO{}
	var doc struct {
		Servers map[string]struct {
			Type     string   `json:"type"`
			URL      string   `json:"url"`
			Command  string   `json:"command"`
			Args     []string `json:"args"`
			Disabled bool     `json:"disabled"`
			Exposure string   `json:"exposure"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return out
	}
	for name, cfg := range doc.Servers {
		transport := cfg.URL
		if transport == "" {
			transport = strings.TrimSpace(cfg.Command + " " + strings.Join(cfg.Args, " "))
		}
		exposure := cfg.Exposure
		if exposure == "" {
			exposure = "codemode"
		}
		out = append(out, domain.MCPServerVO{
			Name: name, Scope: scope, Source: source,
			Enabled: !cfg.Disabled, Exposure: exposure, Transport: transport,
			State: "unknown", Tools: []string{},
		})
	}
	return out
}

// live 读 PI 报的在线状态；PI 未就绪或命令失败时返回空表（界面显示「未连接」）。
func (s *MCPService) live() []domain.MCPServerVO {
	out := []domain.MCPServerVO{}
	if exe, _ := s.env.PiEnv.PiExe(); exe == "" {
		return out
	}
	raw, err := s.env.PiEnv.RunCLIOutput("mcp", "list", "--json")
	if err != nil {
		return out
	}
	body := raw
	if idx := strings.Index(body, "{"); idx > 0 {
		body = body[idx:]
	}
	var doc struct {
		Servers []struct {
			Name      string   `json:"name"`
			Scope     string   `json:"scope"`
			Source    string   `json:"source"`
			Enabled   bool     `json:"enabled"`
			Exposure  string   `json:"exposure"`
			Transport string   `json:"transport"`
			State     string   `json:"state"`
			Error     string   `json:"error"`
			Tools     []string `json:"tools"`
		} `json:"servers"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return out
	}
	for _, srv := range doc.Servers {
		tools := srv.Tools
		if tools == nil {
			tools = []string{}
		}
		out = append(out, domain.MCPServerVO{
			Name: srv.Name, Scope: srv.Scope, Source: srv.Source, Enabled: srv.Enabled,
			Exposure: srv.Exposure, Transport: srv.Transport, State: srv.State,
			Error: srv.Error, Tools: tools,
		})
	}
	return out
}

// Save 校验并写入一份 mcp.json；scope 决定写哪一份。
// 只做结构校验，不判断某个服务连不连得上——那是 `pi mcp list` 的事。
func (s *MCPService) Save(req domain.SaveMCPREQ) error {
	path, err := s.pathOf(req.Scope)
	if err != nil {
		return err
	}
	raw := strings.TrimSpace(req.Raw)
	if raw == "" {
		raw = "{}"
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return pkg.Wrap("9101", "这段 JSON 解析不了", err)
	}
	if servers, ok := doc["mcpServers"]; ok {
		var table map[string]json.RawMessage
		if err := json.Unmarshal(servers, &table); err != nil {
			return pkg.New("9101", "mcpServers 必须是一个对象（服务名 → 配置）", "")
		}
	} else {
		doc["mcpServers"] = json.RawMessage("{}")
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return pkg.Wrap("9102", "配置无法序列化", err)
	}
	if err := s.env.PiEnv.WriteMCPRaw(path, append(out, '\n')); err != nil {
		return err
	}
	return s.Reload()
}

// Remove 删掉一个服务（走 PI 的命令，它知道全局与项目两种落点）。
func (s *MCPService) Remove(req domain.RemoveMCPServerREQ) error {
	if strings.TrimSpace(req.Name) == "" {
		return domain.ErrMCPServer
	}
	args := []string{"mcp", "remove", req.Name}
	if req.Scope == "project" {
		args = append(args, "--local")
	}
	if err := s.env.PiEnv.RunCLI(args, nil); err != nil {
		return err
	}
	return s.Reload()
}

// Login 给 OAuth 服务发起登录。
func (s *MCPService) Login(name string, onLine func(string)) error {
	if strings.TrimSpace(name) == "" {
		return domain.ErrMCPServer
	}
	return s.env.PiEnv.RunCLI([]string{"mcp", "login", name}, onLine)
}

// Logout 清掉某个服务的 OAuth 凭据。
func (s *MCPService) Logout(name string) error {
	if strings.TrimSpace(name) == "" {
		return domain.ErrMCPServer
	}
	return s.env.PiEnv.RunCLI([]string{"mcp", "logout", name}, nil)
}

// Reload 重启 PI 进程让新的 MCP 配置生效。
// 有对话在跑时拒绝：重启会把跑着的那一轮掐断，而配置已经落盘，不会丢。
func (s *MCPService) Reload() error {
	if s.env.Chat != nil && s.env.Chat.AnyRunning() {
		return pkg.New("9105", "配置已保存；有对话在跑，先停下再重连 MCP", "")
	}
	sup := s.env.PiSupervisor()
	if sup == nil {
		return nil
	}
	return sup.RestartAll()
}

// pathOf 把 scope 映射到配置文件路径。
func (s *MCPService) pathOf(scope string) (string, error) {
	switch scope {
	case "", "global":
		return s.env.PiEnv.MCPPath(), nil
	case "project":
		workspace := s.env.Workspace()
		if workspace == "" {
			return "", pkg.New("9104", "还没有选工作目录，无法保存项目级 MCP 配置", "")
		}
		return filepath.Join(workspace, ".pi", "mcp.json"), nil
	default:
		return "", pkg.New("9104", "不支持的配置范围", scope)
	}
}

// MCPTools 把在线服务的工具展开成工具清单（工具页与清单合并用）。
func (s *MCPService) MCPTools() []domain.ToolVO {
	out := []domain.ToolVO{}
	for _, srv := range s.live() {
		for _, tool := range srv.Tools {
			out = append(out, domain.ToolVO{
				Name:        "mcp__" + srv.Name + "__" + tool,
				Description: srv.Name + " 提供的工具",
				Source:      domain.ToolSourceMCP,
				Server:      srv.Name,
				Enabled:     srv.Enabled,
			})
		}
	}
	return out
}

// 编译期确认 context 被引用（未来给 mcp list 加超时用）。
var _ = context.Background
