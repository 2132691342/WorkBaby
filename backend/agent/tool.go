package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// Result 是一次工具执行的输出。
type Result struct {
	// Text 给模型看的正文
	Text string
	// IsError 是否失败（失败也要有正文，模型靠它自救）
	IsError bool
	// Details 给界面看的附加信息（差异、行数、耗时无关的那些）
	Details map[string]any
}

// errorf 造一个失败结果。
func errorf(format string, args ...any) Result {
	return Result{Text: fmt.Sprintf(format, args...), IsError: true}
}

// Tool 是一个可被模型调用的工具。
// Registery 显式传入而不是注入：工具需要工作区上下文，隐式字段会让
// 「哪个工具拿到了哪个工作区」变成一件靠运气的事。
type Tool interface {
	// Spec 工具声明
	Spec() ToolSpec
	// Run 执行一次；args 是已解析的入参
	Run(ctx context.Context, reg *Registry, args map[string]any) Result
}

// Registry 是工具注册表：按工作区解析路径，按名字分发调用。
type Registry struct {
	// Workspace 当前工作区根目录
	Workspace func() string
	// tools 名字 → 工具
	tools map[string]Tool
	// order 声明顺序：写进系统提示时保持稳定，缓存才不会每次都失效
	order []string
	// Todos 待办清单；清单本体挂在注册表上，全局一份
	Todos *TodoStore
	// Asker 提问出口；装上提问工具的前提
	Asker Asker
	// Policy 工具准入策略；计划模式的只读闸门挂在这里
	Policy *ToolPolicy
	// Web 联网出口；装上联网工具的前提
	Web WebKit
	// Index 检索出口；装上检索工具的前提
	Index Indexer
	// TidyPolicy 工具输出折叠策略
	TidyPolicy *tidyPolicy
}

// Indexer 是工作区预索引的出口。
// 引擎只认这个接口，不认具体的索引实现：换检索引擎不该改引擎代码。
type Indexer interface {
	// Search 检索工作区，返回若干条命中
	Search(ctx context.Context, query string, limit int) ([]IndexHit, error)
}

// IndexerFunc 是 Indexer 的函数形态。
type IndexerFunc func(ctx context.Context, query string, limit int) ([]IndexHit, error)

// Search 执行检索。
func (f IndexerFunc) Search(ctx context.Context, query string, limit int) ([]IndexHit, error) {
	return f(ctx, query, limit)
}

// IndexHit 是一条检索命中。
type IndexHit struct {
	// HitPath 相对工作区的路径
	HitPath string
	// HitKind 种类：file / symbol
	HitKind string
	// HitSymbol 符号名
	HitSymbol string
	// HitLine 符号所在行号
	HitLine int
	// HitReason 为什么命中
	HitReason string
}

// NewRegistry 造一个注册表，装上全部内置工具。
// 工具集是能力的唯一边界：模型能做什么，取决于这里注册了什么。
func NewRegistry(workspace func() string) *Registry {
	r := &Registry{
		Workspace:  workspace,
		tools:      map[string]Tool{},
		Todos:      NewTodoStore(),
		Policy:     NewToolPolicy(),
		TidyPolicy: newTidyPolicy(),
	}
	r.Add(&readTool{}, &writeTool{}, &editTool{}, &lsTool{},
		&grepTool{}, &findTool{}, &bashTool{}, &todoTool{store: r.Todos},
		&askTool{reg: r})
	return r
}

// AddIndexTool 装上预索引检索工具。
// 单独装：没有索引时它只会报错，不如一开始就不给。
func (r *Registry) AddIndexTool(indexer Indexer) {
	if indexer == nil {
		return
	}
	r.Index = indexer
	r.Add(&indexSearchTool{indexer: indexer})
}

// AddWebTools 装上联网工具。
// 联网工具单独装：没配搜索服务时装上它们，模型一调用就报错，
// 不如一开始就不给。
func (r *Registry) AddWebTools(kit WebKit) {
	if kit == nil {
		return
	}
	r.Web = kit
	r.Add(&webSearchTool{kit: kit}, &webFetchTool{kit: kit})
}

// Add 注册工具。
func (r *Registry) Add(tools ...Tool) {
	for _, tool := range tools {
		spec := tool.Spec()
		if _, seen := r.tools[spec.Name]; !seen {
			r.order = append(r.order, spec.Name)
		}
		r.tools[spec.Name] = tool
	}
}

// Specs 全部工具声明，按注册顺序。
func (r *Registry) Specs() []ToolSpec {
	out := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].Spec())
	}
	return out
}

// Run 按名字执行一次工具调用。
func (r *Registry) Run(ctx context.Context, name, rawArgs string) Result {
	tool, ok := r.tools[name]
	if !ok {
		return errorf("没有这个工具：%s", name)
	}
	// 只读闸门在解析入参之前：计划模式下的写工具连参数都不该被看一眼。
	if denied := r.GuardPolicy(ctx, name); denied != nil {
		return *denied
	}
	args := map[string]any{}
	if trimmed := strings.TrimSpace(rawArgs); trimmed != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return errorf("入参不是合法 JSON：%v（收到的是 %s）", err, pkg.Truncate(trimmed, 200))
		}
		args = parsed
	}
	return tool.Run(ctx, r, args)
}

// resolve 把工具入参里的路径解析成工作区内的绝对路径。
// 越界一律拒：模型给的路径来自对话内容，不能成为逃出工作区的通道。
func (r *Registry) resolve(raw string) (string, error) {
	workspace := strings.TrimSpace(r.Workspace())
	if workspace == "" {
		return "", pkg.Err(pkg.CodeSessionBind, "还没有选择工作区")
	}
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", pkg.Err(pkg.CodeBadRequest, "路径不能为空")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(workspace, target)
	}
	clean := filepath.Clean(target)
	if !pkg.Inside(workspace, clean) {
		return "", pkg.Err(pkg.CodePathOutside, "路径不在工作区内", clean)
	}
	return clean, nil
}

// relative 把绝对路径折成相对工作区的正斜杠形式：工具输出里的路径要能直接再喂回来。
func (r *Registry) relative(path string) string {
	workspace := strings.TrimSpace(r.Workspace())
	if workspace == "" {
		return path
	}
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// stringArg 取字符串入参。
func stringArg(args map[string]any, key string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

// intArg 取整数入参；缺失给 fallback。
func intArg(args map[string]any, key string, fallback int) int {
	if value, ok := args[key].(float64); ok {
		return int(value)
	}
	return fallback
}

// boolArg 取布尔入参。
func boolArg(args map[string]any, key string) bool {
	value, _ := args[key].(bool)
	return value
}
