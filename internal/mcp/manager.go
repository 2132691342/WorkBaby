package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/tool"
)

// ServerStatus MCP server 运行时状态（落库配置之外的运行态，供设置页展示）。
type ServerStatus struct {
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	ToolCount int    `json:"toolCount"`
	Error     string `json:"error,omitempty"`
}

// managedServer 已接管的 server：指纹用于增量重载时判断配置是否变化。
type managedServer struct {
	fingerprint string
	client      *StdioClient
	toolNames   []string
	status      ServerStatus
}

// Manager 管理 MCP server 生命周期：启动 → 握手 → 拉工具 → 注册进 tool.Registry。
// 单个 server 失败只标记 unready，不阻断整体启动。
type Manager struct {
	toolReg  *tool.Registry
	pathDirs func() []string // 可选：注入内置运行时 bin 目录（如 node/python 解压根），启动时前置到子进程 PATH
	opMu     sync.Mutex      // 串行化 Reload / Close
	mu       sync.Mutex      // 保护 servers
	servers  map[string]*managedServer
}

// NewManager 注入全局工具注册中心（MCP 工具注册/注销的目标）。
func NewManager(toolReg *tool.Registry) *Manager {
	return &Manager{toolReg: toolReg, servers: make(map[string]*managedServer)}
}

// WithPathDirs 设置内置运行时 bin 目录来源；启动时会把这些目录前置到子进程 PATH。
// 不调则走系统 PATH——npx / uvx 等依赖 node/python 内置运行时的 server 将用系统安装版本。
func (m *Manager) WithPathDirs(f func() []string) *Manager {
	m.pathDirs = f
	return m
}

// Reload 按期望配置增量对齐：停掉被删除或改了配置的 server，启动缺失的。
// 单 server 失败降级处理，错误落在 Status 里而非中断整体。
func (m *Manager) Reload(ctx context.Context, servers []domain.McpServerDO) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	desired := make(map[string]domain.McpServerDO, len(servers))
	for i := range servers {
		srv := servers[i]
		if !srv.Enabled || srv.Transport != domain.McpTransportStdio {
			continue
		}
		desired[srv.Name] = srv
	}

	stale := make([]string, 0, len(m.servers))
	m.mu.Lock()
	for name, ms := range m.servers {
		if srv, ok := desired[name]; ok && fingerprint(srv) == ms.fingerprint {
			continue
		}
		stale = append(stale, name)
	}
	m.mu.Unlock()

	for _, name := range stale {
		m.stop(name)
	}
	for _, srv := range desired {
		m.start(ctx, srv)
	}
}

// Status 查询单个 server 运行时状态。
func (m *Manager) Status(name string) ServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ms, ok := m.servers[name]; ok {
		return ms.status
	}
	return ServerStatus{Name: name}
}

// List 返回全部受管 server 状态（按名称排序）。
func (m *Manager) List() []ServerStatus {
	m.mu.Lock()
	out := make([]ServerStatus, 0, len(m.servers))
	for _, ms := range m.servers {
		out = append(out, ms.status)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Close 停掉全部 server 并注销其工具（应用退出时调用）。
func (m *Manager) Close() {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.Lock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	m.mu.Unlock()

	for _, name := range names {
		m.stop(name)
	}
}

// start 启动单个 server 并完成握手 + 工具注册；失败原因写进 status。
func (m *Manager) start(ctx context.Context, srv domain.McpServerDO) {
	name := srv.Name
	m.mu.Lock()
	if _, running := m.servers[name]; running {
		m.mu.Unlock()
		return
	}
	m.servers[name] = &managedServer{fingerprint: fingerprint(srv), status: ServerStatus{Name: name}}
	m.mu.Unlock()

	st := ServerStatus{Name: name}
	var client *StdioClient
	var registered []string
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		ms, ok := m.servers[name]
		if !ok { // 启动期间被 stop（并发重载），丢弃本次成果
			if client != nil {
				_ = client.Close()
			}
			return
		}
		ms.status = st
		ms.client = client
		ms.toolNames = registered
	}()

	client, err := DialStdio(name, srv.Command, parseStringList(srv.Args), parseEnvMap(srv.Env), m.pathDirs)
	if err != nil {
		st.Error = err.Error()
		return
	}
	if _, err := client.Initialize(ctx); err != nil {
		st.Error = err.Error()
		_ = client.Close()
		client = nil
		return
	}
	// 死亡自清理：子进程一旦崩溃，注销其已注册工具——避免模型反复调用空指针。
	go func() {
		<-client.Done()
		m.stop(name)
	}()
	tools, err := client.ListTools(ctx)
	if err != nil {
		st.Error = err.Error()
		_ = client.Close()
		client = nil
		return
	}

	conflicts := 0
	for i := range tools {
		adapter := &MCPAdapter{ServerName: name, Client: client, Tool: tools[i]}
		if err := m.toolReg.Register(adapter); err != nil { // 8007 冲突：跳过不覆盖
			conflicts++
			continue
		}
		registered = append(registered, adapter.Name())
	}
	st.Ready = true
	st.ToolCount = len(registered)
	if conflicts > 0 {
		st.Error = fmt.Sprintf("%d 个工具因命名冲突被跳过", conflicts)
	}
}

// stop 停掉 server、注销其已注册工具。
func (m *Manager) stop(name string) {
	m.mu.Lock()
	ms, ok := m.servers[name]
	if ok {
		delete(m.servers, name)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	if ms.client != nil {
		_ = ms.client.Close()
	}
	for _, toolName := range ms.toolNames {
		m.toolReg.Unregister(toolName)
	}
}

// fingerprint 配置指纹：命令 / args / env 任一变化都需要重启子进程。
// Args 与 Env 落库时即为 json.Marshal 结果（map key 有序），可直接参与比较。
func fingerprint(srv domain.McpServerDO) string {
	return srv.Command + "\x00" + srv.Args + "\x00" + srv.Env
}

func parseStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// parseEnvMap 环境变量 map → ["K=V", ...]（按 key 排序，保证子进程环境稳定）。
func parseEnvMap(raw string) []string {
	if raw == "" {
		return nil
	}
	var kv map[string]string
	if err := json.Unmarshal([]byte(raw), &kv); err != nil {
		return nil
	}
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(kv))
	for _, k := range keys {
		out = append(out, k+"="+kv[k])
	}
	return out
}
