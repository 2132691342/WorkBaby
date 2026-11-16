package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/pkg"
)

// mcpEnvBase 是 MCP 子进程环境的必须项：与工具子进程同一套纪律。
func mcpEnvBase(layout pkg.Layout) []string { return layout.ChildEnv(nil) }

// mcpRuntime 持有活着的服务会话，负责装配与重建注册表的 MCP 工具段。
// 真相始终是 mcp.json：这里只是把清单里 enabled 的服务变成真能力，
// 重建时以清单为准整段换掉——清单里没有的服务不留残余工具。
type mcpRuntime struct {
	// settings 设置（项目级 mcp.json 要跟着工作区走）
	settings *Settings
	// layout 目录布局
	layout pkg.Layout
	// registry 引擎注册表
	registry *agent.Registry
	// clients 活着的服务会话，按服务名
	clients map[string]*agent.McpClient
	// rebuildMu 串行化重建：并发触发时后到的等先到的完成，避免半新半旧
	rebuildMu chan struct{}
}

// newMcpRuntime 造一个运行时。
func newMcpRuntime(settings *Settings, layout pkg.Layout, registry *agent.Registry) *mcpRuntime {
	return &mcpRuntime{
		settings:  settings,
		layout:    layout,
		registry:  registry,
		clients:   map[string]*agent.McpClient{},
		rebuildMu: make(chan struct{}, 1),
	}
}

// Rebuild 按 mcp.json 现状重建注册表的 MCP 工具段。
// 单个服务失败只记 Warn：探测按钮就在配置页，失败原因界面看得见，
// 为一个坏服务把其他服务拖下水不划算。同名条目项目级优先，与 List 的合并语义一致。
func (r *mcpRuntime) Rebuild() {
	r.rebuildMu <- struct{}{}
	defer func() { <-r.rebuildMu }()

	entries := r.enabledEntries()
	envBase := mcpEnvBase(r.layout)
	fresh := map[string]*agent.McpClient{}
	var tools []agent.Tool
	for _, def := range entries {
		loaded, client, err := agent.LoadMcpTools(def, envBase)
		if err != nil {
			pkg.Warn("MCP 服务 %s 装载失败：%s（详情：%s）",
				def.ServerName, pkg.As(err).Message, pkg.As(err).Detail)
			continue
		}
		if len(loaded) == 0 {
			client.Close()
			continue
		}
		fresh[def.ServerName] = client
		tools = append(tools, loaded...)
		pkg.Info("MCP 服务 %s 已装载 %d 个工具", def.ServerName, len(loaded))
	}
	// 先换段再收旧进程：先清后装会让全部 MCP 工具在装载窗口里凭空消失。
	r.registry.ReplaceMcpTools(tools)
	r.reset()
	r.clients = fresh
}

// enabledEntries 收集所有 enabled 的 stdio 服务定义。
// 项目级与用户级同名时只留项目级；条目顺序按服务名排，清单形状稳定。
func (r *mcpRuntime) enabledEntries() []agent.McpServerDef {
	merged := map[string]agent.McpServerDef{}
	addFrom := func(path string) {
		value, err := readMcpFile(path)
		if err != nil {
			return
		}
		for name, entry := range value.McpServers {
			if strings.TrimSpace(name) == "" || entry["enabled"] == false {
				continue
			}
			if def, ok := mcpServerDefOf(name, entry); ok {
				merged[name] = def
			}
		}
	}
	addFrom(r.layout.McpFile())
	// 项目级后读、直接覆盖同名：UserPath 在前 ProjectPath 在后，与 List 一致。
	project := r.projectPath()
	if project != "" {
		addFrom(project)
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]agent.McpServerDef, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out
}

// projectPath 项目级 mcp.json；未选工作区给空串。与 Mcp.projectPath 同一语义。
func (r *mcpRuntime) projectPath() string {
	if r.settings != nil {
		if ws := r.settings.Workspace(); ws != "" {
			return filepath.Join(workspaceConfigDir(ws), "mcp.json")
		}
	}
	return ""
}

// mcpServerDefOf 把一个 mcp.json 条目解成引擎要的定义；只认 stdio。
func mcpServerDefOf(name string, entry map[string]any) (agent.McpServerDef, bool) {
	command := stringField(entry, "command")
	if strings.TrimSpace(command) == "" {
		return agent.McpServerDef{}, false
	}
	def := agent.McpServerDef{
		ServerName: name,
		Command:    command,
		Args:       asStringSlice(entry["args"]),
		Cwd:        stringField(entry, "cwd"),
		Env:        map[string]string{},
	}
	if userEnv, ok := entry["env"].(map[string]any); ok {
		for key, value := range userEnv {
			def.Env[key] = fmt.Sprint(value)
		}
	}
	if n, ok := entry["timeout"].(float64); ok && n > 0 {
		def.TimeoutMs = int(n * 1000)
	}
	return def, true
}

// reset 收掉所有服务进程；重建与退出时都要走。
func (r *mcpRuntime) reset() {
	for name, client := range r.clients {
		client.Close()
		delete(r.clients, name)
	}
}
