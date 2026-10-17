// MCP 编排：编辑 PI 的 mcp.json，并从 PI 读回它报上来的工具。
// 客户端是 PI——它自己拉子进程、自己握手、自己把工具声明并进模型请求。
package service

import (
	"encoding/json"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// MCPService 编排 MCP 配置（文件即唯一真相源）。
type MCPService struct {
	env *Env
}

// NewMCPService 构造 MCP 服务。
func NewMCPService(env *Env) *MCPService { return &MCPService{env: env} }

// Config 返回配置面板要的全部东西：文件路径、原文、解析结果与解析错误。
func (s *MCPService) Config() (*domain.MCPConfigRESP, error) {
	raw, err := s.env.PiEnv.ReadMCPRaw()
	if err != nil {
		return nil, err
	}
	servers, parseErr := parseMCPServers(raw)
	return &domain.MCPConfigRESP{
		Path:       s.env.PiEnv.MCPPath(),
		Raw:        string(raw),
		Servers:    servers,
		ParseError: parseErr,
	}, nil
}

// SaveConfig 保存配置原文：先校验再落盘。
// 解析不过直接返回 9106，文件一个字符都不动——写坏一份配置等于把整个面板变成
// 「什么都连不上」，且用户看不出是自己改坏的。
func (s *MCPService) SaveConfig(req domain.SaveMCPConfigREQ) (*domain.MCPConfigRESP, error) {
	if _, parseErr := parseMCPServers([]byte(req.Raw)); parseErr != "" {
		return nil, pkg.New("9106", "配置有问题，没有保存", parseErr)
	}
	if err := s.env.PiEnv.WriteMCPRaw([]byte(req.Raw)); err != nil {
		return nil, err
	}
	// 知识库那条由程序维护，保存后补回：用户可能整段替换掉了它。
	if err := s.env.PiEnv.EnsureKnowledgeMCP(selfExe()); err != nil {
		pkgWarn("mcp: 补回知识库条目失败", err)
	}
	return s.Config()
}

// Templates 返回内置模板（一次给全：面板要列三个按钮）。
func (s *MCPService) Templates() []domain.MCPTemplateVO {
	return []domain.MCPTemplateVO{
		{
			Kind: "blank", Label: "空白", Hint: "从零写一份",
			Raw: "{\n  \"mcpServers\": {}\n}\n",
		},
		{
			Kind: "local", Label: "本地命令", Hint: "一个本地可执行程序",
			Raw: `{
  "mcpServers": {
    "my-tool": {
      "command": "python",
      "args": ["-m", "my_mcp_server"],
      "cwd": "D:\\tools\\my-mcp"
    }
  }
}
`,
		},
		{
			Kind: "example", Label: "常见示例", Hint: "文件系统服务",
			Raw: `{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "D:\\WorkBaby"]
    }
  }
}
`,
		},
	}
}

// List 列出配置里的服务。
func (s *MCPService) List() []domain.MCPServerVO {
	raw, err := s.env.PiEnv.ReadMCPRaw()
	if err != nil {
		return []domain.MCPServerVO{}
	}
	servers, _ := parseMCPServers(raw)
	return servers
}

// Tools 当前 PI 报上来的 MCP 工具（从工具目录里按前缀筛）。
func (s *MCPService) Tools() []domain.MCPToolVO {
	out := []domain.MCPToolVO{}
	for _, name := range s.mcpToolNames() {
		parts := strings.Split(name, "__")
		server := ""
		if len(parts) >= 3 {
			server = parts[1]
		}
		out = append(out, domain.MCPToolVO{Name: name, Label: name, Server: server})
	}
	return out
}

// mcpToolNames 从 mcp.json 推断各服务的前缀（具体工具名要 PI 跑起来才知道）。
func (s *MCPService) mcpToolNames() []string {
	raw, err := s.env.PiEnv.ReadMCPRaw()
	if err != nil {
		return nil
	}
	servers, _ := parseMCPServers(raw)
	out := make([]string, 0, len(servers))
	for _, sv := range servers {
		if sv.Disabled {
			continue
		}
		out = append(out, "mcp__"+sv.Name+"__*")
	}
	sort.Strings(out)
	return out
}

// parseMCPServers 解析 mcp.json；返回服务条目与解析错误（空串表示没问题）。
// 未知字段原样保留——从别的客户端拷来的配置带 timeout / headers 等，
// 按自己的理解裁剪会让它失效。
func parseMCPServers(raw []byte) ([]domain.MCPServerVO, string) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []domain.MCPServerVO{}, ""
	}
	var doc struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return []domain.MCPServerVO{}, "不是合法的 JSON：" + err.Error()
	}
	if doc.Servers == nil {
		// 缺失通常是把 mcpServers 写错了名字，静默当成空配置会让用户以为保存成功。
		return []domain.MCPServerVO{}, "缺少 mcpServers 字段"
	}
	out := make([]domain.MCPServerVO, 0, len(doc.Servers))
	for name, body := range doc.Servers {
		var entry struct {
			Command  string            `json:"command"`
			Args     []string          `json:"args"`
			Env      map[string]string `json:"env"`
			Cwd      string            `json:"cwd"`
			Disabled bool              `json:"disabled"`
		}
		_ = json.Unmarshal(body, &entry)
		if strings.TrimSpace(name) == "" {
			return []domain.MCPServerVO{}, "有服务的名字是空的"
		}
		if entry.Command == "" {
			return []domain.MCPServerVO{}, "服务「" + name + "」缺少 command"
		}
		keys := make([]string, 0, len(entry.Env))
		for k := range entry.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out = append(out, domain.MCPServerVO{
			Name:     name,
			Command:  entry.Command,
			Args:     entry.Args,
			EnvKeys:  keys,
			Cwd:      entry.Cwd,
			Disabled: entry.Disabled,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, ""
}

// Reload 触发 PI 重扫：mcp.json 是进程启动期读的，改文件不重启等于没改。
func (s *MCPService) Reload() error {
	sup := s.env.PiSupervisor()
	if sup == nil {
		return nil
	}
	return sup.RestartAll()
}
