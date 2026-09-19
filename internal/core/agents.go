package core

import (
	"sort"
	"sync"
)

// 内置 Agent 名：default 通用全能；explore 只读探索（白名单里刻意没有写工具）。
const (
	AgentDefault = "default"
	AgentExplore = "explore"
)

// builtinAgents 内置定义表。
var builtinAgents = map[string]Definition{
	AgentDefault: {
		Name:        AgentDefault,
		Description: "通用助手：本机读写文件、执行命令、联网检索，可沉淀记忆",
		Persona:     personaDefault,
		Memory:      MemoryPolicy{Enabled: true, RecallLimit: 6, Formation: true},
		Budget:      Budget{ContextTokens: 120_000},
	},
	AgentExplore: {
		Name:        AgentExplore,
		Description: "只读探索：在隔离上下文里查清事实并带回证据，不做任何改动",
		Persona:     personaExplore,
		Tools: ToolPolicy{Allow: []string{
			"file_read", "file_list", "file_grep",
			"websearch", "webfetch", "knowledge_search",
		}},
		// 探索是临时上下文：不召回也不沉淀，避免把中间结论写进长期记忆。
		Memory: MemoryPolicy{Enabled: false},
		Budget: Budget{MaxTurns: 24},
	},
}

// 自定义定义（设置页与定义文件物化）。整表替换，进程内只读。
var (
	customMu     sync.RWMutex
	customAgents = map[string]Definition{}
)

// SetCustomAgents 整表替换自定义定义。
// 内置名不可覆盖：default / explore 的行为是产品契约，被同名自定义顶掉会让
// 「界面上叫 explore，实际能写文件」这种问题无法解释。
func SetCustomAgents(defs []Definition) {
	m := make(map[string]Definition, len(defs))
	for _, d := range defs {
		if d.Name == "" {
			continue
		}
		if _, builtin := builtinAgents[d.Name]; builtin {
			continue
		}
		m[d.Name] = d
	}
	customMu.Lock()
	customAgents = m
	customMu.Unlock()
}

// Agent 按名取定义；未知名回退 default（委派与切换共用的唯一入口）。
func Agent(name string) Definition {
	customMu.RLock()
	d, ok := customAgents[name]
	customMu.RUnlock()
	if ok {
		return d
	}
	if d, ok := builtinAgents[name]; ok {
		return d
	}
	return builtinAgents[AgentDefault]
}

// BuiltinAgentNames 内置名清单（定义文件撞名检查用；自定义名不在此列）。
func BuiltinAgentNames() []string {
	out := make([]string, 0, len(builtinAgents))
	for n := range builtinAgents {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// AllAgents 全部可用定义（内置在前，自定义在后）。
func AllAgents() []Definition {
	customMu.RLock()
	out := make([]Definition, 0, len(builtinAgents)+len(customAgents))
	for _, n := range BuiltinAgentNames() {
		out = append(out, builtinAgents[n])
	}
	names := make([]string, 0, len(customAgents))
	for n := range customAgents {
		names = append(names, n)
	}
	customMu.RUnlock()
	sort.Strings(names)
	for _, n := range names {
		out = append(out, Agent(n))
	}
	return out
}

// AgentNames 全部可用名（内置 + 自定义，升序）。
func AgentNames() []string {
	customMu.RLock()
	out := make([]string, 0, len(builtinAgents)+len(customAgents))
	for n := range builtinAgents {
		out = append(out, n)
	}
	for n := range customAgents {
		out = append(out, n)
	}
	customMu.RUnlock()
	sort.Strings(out)
	return out
}
