package service

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Mcp 是 MCP 编排：用户级 mcp.json 与工作区 .pi\mcp.json 的增删改查与开关。
type Mcp struct {
	// Settings 设置（项目级 mcp.json 要跟着工作区走）
	Settings *Settings
	// Layout 沙箱布局
	Layout pkg.Layout
}

// NewMcp 造一个 MCP 门面。
func NewMcp(settings *Settings, layout pkg.Layout) *Mcp {
	return &Mcp{Settings: settings, Layout: layout}
}

// projectPath 项目级 mcp.json；未选工作区给空串。
func (m *Mcp) projectPath() string {
	if ws := m.Settings.Workspace(); ws != "" {
		return filepath.Join(ws, ".pi", "mcp.json")
	}
	return ""
}

// mcpFile 是 mcp.json 的形状。
type mcpFile struct {
	// McpServers 服务名 → 定义
	McpServers map[string]map[string]any `json:"mcpServers"`
}

// readMcpFile 读 mcp.json；不存在给空表。
func readMcpFile(path string) (mcpFile, error) {
	out := mcpFile{McpServers: map[string]map[string]any{}}
	if !pkg.Exists(path) {
		return out, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, pkg.WrapErr(pkg.CodeMcpRead, "mcp.json 读取失败", err)
	}
	var value mcpFile
	if err := pkg.UnmarshalJSON(raw, &value); err != nil {
		return out, pkg.WrapErr(pkg.CodeMcpRead, "mcp.json 解析失败", err)
	}
	if value.McpServers == nil {
		value.McpServers = map[string]map[string]any{}
	}
	return value, nil
}

// writeMcpFile 原子写 mcp.json。
func writeMcpFile(path string, value mcpFile) error {
	if err := pkg.WriteJSON(path, value); err != nil {
		return pkg.WrapErr(pkg.CodeMcpWrite, "mcp.json 写入失败", err)
	}
	return nil
}

// List 清单：用户级 + 项目级合并，项目级同名覆盖用户级。
func (m *Mcp) List() domain.McpListRESP {
	userPath := m.Layout.McpFile()
	projectPath := m.projectPath()
	out := domain.McpListRESP{UserPath: userPath, ProjectPath: projectPath}

	user, err := readMcpFile(userPath)
	if err != nil {
		// 读坏了不影响另一侧：界面要能看见一条服务并把它改掉。
		out.ReadErrors = append(out.ReadErrors, pkg.As(err).Message)
	}
	var project mcpFile
	if projectPath != "" && pkg.Exists(projectPath) {
		project, err = readMcpFile(projectPath)
		if err != nil {
			out.ReadErrors = append(out.ReadErrors, pkg.As(err).Message)
		}
	}

	for name, entry := range user.McpServers {
		if _, shadowed := project.McpServers[name]; shadowed {
			continue
		}
		out.Servers = append(out.Servers, toServerRow(name, entry, "user", userPath))
	}
	for name, entry := range project.McpServers {
		out.Servers = append(out.Servers, toServerRow(name, entry, "project", projectPath))
	}
	return out
}

// toServerRow 把一条定义转成界面行。
func toServerRow(name string, entry map[string]any, scope, configPath string) domain.McpServerVO {
	url, _ := entry["url"].(string)
	command, _ := entry["command"].(string)
	row := domain.McpServerVO{
		ServerName: name,
		ServerUrl:  url,
		ScopeKind:  scope,
		ConfigPath: configPath,
		McpEnabled: entry["enabled"] != false,
		McpDesc:    stringField(entry, "description"),
	}
	if url != "" {
		row.TransportKind = "http"
		return row
	}
	row.TransportKind = "stdio"
	args := asStringSlice(entry["args"])
	parts := append([]string{command}, args...)
	row.CommandLine = strings.TrimSpace(strings.Join(parts, " "))
	return row
}

// stringField 安全取字符串字段。
func stringField(entry map[string]any, key string) string {
	value, _ := entry[key].(string)
	return value
}

// Save 保存：写用户级文件。有 ServerUrl 即 HTTP，否则拆 CommandText。
func (m *Mcp) Save(req domain.McpSaveREQ) (domain.McpListRESP, error) {
	name := strings.TrimSpace(req.ServerName)
	if name == "" {
		return domain.McpListRESP{}, pkg.Err(pkg.CodeMcpWrite, "服务名不能为空")
	}
	userPath := m.Layout.McpFile()
	value, err := readMcpFile(userPath)
	if err != nil {
		return domain.McpListRESP{}, err
	}
	entry := map[string]any{}
	switch {
	case strings.TrimSpace(req.ServerUrl) != "":
		if req.TransportKind == "stdio" {
			return domain.McpListRESP{}, pkg.Err(pkg.CodeMcpWrite, "URL 服务不能配置命令行")
		}
		entry["url"] = strings.TrimSpace(req.ServerUrl)
	case strings.TrimSpace(req.CommandText) != "":
		parts := pkg.SplitCommandLine(strings.TrimSpace(req.CommandText))
		if len(parts) == 0 {
			return domain.McpListRESP{}, pkg.Err(pkg.CodeMcpWrite, "命令行不能为空")
		}
		entry["command"] = parts[0]
		if len(parts) > 1 {
			entry["args"] = toAnySlice(parts[1:])
		}
	default:
		return domain.McpListRESP{}, pkg.Err(pkg.CodeMcpWrite, "命令行与 URL 至少填一个")
	}
	if desc := strings.TrimSpace(req.McpDesc); desc != "" {
		entry["description"] = desc
	}
	if env := parseEnvList(req.EnvList); len(env) > 0 {
		entry["env"] = env
	}
	// 改定义不该顺手把停用的服务重新启用。
	if prev, ok := value.McpServers[name]; ok && prev["enabled"] == false {
		entry["enabled"] = false
	}
	value.McpServers[name] = entry
	if err := writeMcpFile(userPath, value); err != nil {
		return domain.McpListRESP{}, err
	}
	return m.List(), nil
}

// parseEnvList 把逐行 KEY=VALUE 解析成映射。
func parseEnvList(lines []string) map[string]any {
	env := map[string]any{}
	for _, line := range lines {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		env[strings.TrimSpace(key)] = value
	}
	return env
}

// Toggle 开关：找到条目改 enabled；项目级同名条目优先。
func (m *Mcp) Toggle(serverName string, enabled bool) (domain.McpListRESP, error) {
	for _, path := range []string{m.projectPath(), m.Layout.McpFile()} {
		if path == "" || !pkg.Exists(path) {
			continue
		}
		value, err := readMcpFile(path)
		if err != nil {
			return domain.McpListRESP{}, err
		}
		entry, ok := value.McpServers[serverName]
		if !ok {
			continue
		}
		entry["enabled"] = enabled
		value.McpServers[serverName] = entry
		if err := writeMcpFile(path, value); err != nil {
			return domain.McpListRESP{}, err
		}
		return m.List(), nil
	}
	return domain.McpListRESP{}, pkg.Err(pkg.CodeMcpWrite, "未找到 MCP 服务："+serverName)
}

// Remove 删除：用户级与项目级都尝试移除。
func (m *Mcp) Remove(serverName string) (domain.McpListRESP, error) {
	for _, path := range []string{m.projectPath(), m.Layout.McpFile()} {
		if path == "" || !pkg.Exists(path) {
			continue
		}
		value, err := readMcpFile(path)
		if err != nil {
			return domain.McpListRESP{}, err
		}
		if _, ok := value.McpServers[serverName]; !ok {
			continue
		}
		delete(value.McpServers, serverName)
		if err := writeMcpFile(path, value); err != nil {
			return domain.McpListRESP{}, err
		}
	}
	return m.List(), nil
}

// Count MCP 服务总数（健康面板用）。
func (m *Mcp) Count() int {
	return len(m.List().Servers)
}
