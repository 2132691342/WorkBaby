package agent

import (
	"context"
	"strings"
	"sync"
)

// ToolPolicy 是工具的准入策略。
// 计划模式要拦的是「会改东西」的工具：读可以，写不行。
// 把这件事做成注册表上的一个开关，而不是在每个工具里各判一次——
// 各判一次的话，新增工具时忘了判，就等于没拦。
type ToolPolicy struct {
	// mu 保护只读态
	mu sync.RWMutex
	// readOnly 是否处于只读模式
	readOnly bool
	// sessionId 当前会话；只读态按会话记
	sessionId string
}

// NewToolPolicy 造一个工具策略。
func NewToolPolicy() *ToolPolicy { return &ToolPolicy{} }

// ReadOnly 是否处于只读模式。
func (p *ToolPolicy) ReadOnly() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.readOnly
}

// SetReadOnly 设置只读模式，并记住是哪个会话的状态。
func (p *ToolPolicy) SetReadOnly(sessionId string, enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readOnly = enabled
	p.sessionId = sessionId
}

// SessionId 当前策略绑定的会话。
func (p *ToolPolicy) SessionId() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionId
}

// readOnlyTools 是明确不会改变工作区或系统状态的工具。
// 只读模式下这些照常可用：计划模式的目的是「先调研再动手」，
// 把读工具也拦掉等于让模型什么都做不了。
var readOnlyTools = map[string]struct{}{
	"read":             {},
	"grep":             {},
	"find":             {},
	"ls":               {},
	"todo":             {},
	"ask":              {},
	"web_search":       {},
	"web_fetch":        {},
	"lens_diagnostics": {},
	"lens_symbols":     {},
}

// mutatingTools 是会改变工作区或系统状态的工具。
// 只读模式下这些一律被拦下——拦的是「执行」，不是「声明」：
// 模型仍然看得见这些工具，知道自己能做什么、不能做什么。
var mutatingTools = map[string]struct{}{
	"write":     {},
	"edit":      {},
	"bash":      {},
	"shell":     {},
	"apply":     {},
	"patch":     {},
	"delete":    {},
	"move":      {},
	"mkdir":     {},
	"git":       {},
	"web_apply": {},
}

// IsMutatingTool 判断一个工具名是否会改变状态。
// 判定顺序：先看只读白名单，再看写工具名单，都没登记就按「会改」处理。
// 默认拦是故意的：漏登记一个写工具，计划模式就名存实亡；
// 而漏登记一个读工具只会让模型多问一句——两者不对等。
func IsMutatingTool(name string) bool {
	trimmed := strings.TrimSpace(name)
	if _, ok := readOnlyTools[trimmed]; ok {
		return false
	}
	if _, ok := mutatingTools[trimmed]; ok {
		return true
	}
	return true
}

// BlockedToolNames 列出当前会被拦下的工具名；不在计划模式时给空。
// 它住在 agent 包里：工具集与准入策略都是引擎自己的事，
// 上层只该拿到一份名单，不该知道判定规则。
func BlockedToolNames(reg *Registry) []string {
	if reg == nil || reg.Policy == nil || !reg.Policy.ReadOnly() {
		return nil
	}
	out := make([]string, 0, 4)
	for _, spec := range reg.Specs() {
		if IsMutatingTool(spec.Name) {
			out = append(out, spec.Name)
		}
	}
	return out
}

// planDeniedResult 是只读模式下拦住写工具时给模型的结果。
// 措辞要说清「现在是计划模式」与「下一步做什么」：
// 模型拿到一句「禁止」，往往就自己放弃了，而不是换个思路。
func planDeniedResult(name string) Result {
	return Result{
		Text: "现在是计划模式：只能读，不能改。" +
			"你想做的改动我已记下，等退出计划模式（/plan off）后再执行。" +
			"如果需要先确认细节，可以先用 read / grep / find 看清楚现状。",
		IsError: true,
		Details: map[string]any{
			"blocked_by": "plan_mode",
			"tool_name":  name,
		},
	}
}

// GuardPolicy 按只读态拦下写工具。
// 返回 nil 表示放行；返回非 nil 时该结果直接作为工具输出。
func (r *Registry) GuardPolicy(ctx context.Context, name string) *Result {
	policy := r.Policy
	if policy == nil || !policy.ReadOnly() {
		return nil
	}
	if !IsMutatingTool(name) {
		return nil
	}
	result := planDeniedResult(name)
	return &result
}
