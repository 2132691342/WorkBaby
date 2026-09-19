package tool

import (
	"encoding/json"
	"sync"

	"WorkBaby/internal/pkg"
)

// Registry 工具注册中心：按名查找、列元信息。
// 启停状态不在此处，由 service 层结合 system_settings 决定是否暴露给 LLM。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 构造空注册中心。
func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

// Register 注册工具；重名返回 4005 错误；参数 Schema 编译失败返回 4004（注册即编译）。
func (r *Registry) Register(t Tool) error {
	if t == nil || t.Name() == "" {
		return pkg.New(4005, "tool must have a name", "")
	}
	if err := CompileSchema(t.Schema().Parameters); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tools[t.Name()]; dup {
		return pkg.New(4005, "tool already registered", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

// Unregister 注销工具；用于 MCP server 停用/重载时移除其已注册工具。
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get 按名取工具实例；未注册返回 ok=false。
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 返回全部已注册工具（无序）。
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// SelfCheckSchema 启动期自检：所有已注册工具 schema 必须是合法 JSON。
func (r *Registry) SelfCheckSchema() error {
	for _, t := range r.List() {
		params := t.Schema().Parameters
		if len(params) > 0 && !json.Valid(params) {
			return pkg.New(4004, "tool schema is not valid json", t.Name())
		}
	}
	return nil
}

// AllReadOnly 给定工具名列表是否全部存在且声明只读（ToolMeta 声明优先，RiskLevel 兜底）。
// 供 harness 只读并行执行裁决。
func (r *Registry) AllReadOnly(names []string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range names {
		t, ok := r.tools[name]
		if !ok || !MetaOf(t).ReadOnly {
			return false
		}
	}
	return true
}
