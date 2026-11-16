package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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
	// mu 保护 tools / order。内置工具在装配时一次性装完，但 MCP 段
	// 会随配置增删改与开关切换在运行中整段重建——而轮次循环随时可能在读。
	mu sync.Mutex
	// mcpTools 当前挂着的 MCP 工具；重建时整段换掉
	mcpTools []Tool
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
	// BashConfirm 危险命令确认闸门是否开启；nil 视为开启（fail-closed）。
	// 由上层从设置台账注入：用户在设置页关掉后，命中危险特征也直接执行。
	BashConfirm func() bool
	// TidyPolicy 工具输出折叠策略
	TidyPolicy *tidyPolicy
	// SkillIndex 已启用技能索引；每轮开跑前由上层整段换上
	SkillIndex *SkillSet
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
		SkillIndex: NewSkillSet(),
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addLocked(tools)
}

// addLocked 实际登记；调用方必须已持锁。
func (r *Registry) addLocked(tools []Tool) {
	for _, tool := range tools {
		spec := tool.Spec()
		if _, seen := r.tools[spec.Name]; !seen {
			r.order = append(r.order, spec.Name)
		}
		r.tools[spec.Name] = tool
	}
}

// ReplaceMcpTools 整段换掉 MCP 工具：配置增删改、开关切换后由装配处调用。
// MCP 段是注册表里唯一会在运行中变动的部分，整段换才与「mcp.json 就是真相」一致。
// 与内置工具同名时 MCP 让位：内置行为在本进程内可修，MCP 实现在外面。
func (r *Registry) ReplaceMcpTools(tools []Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 先摘掉旧 MCP 段：tools 删名字，order 只留内置段，顺序不动。
	for _, tool := range r.mcpTools {
		delete(r.tools, tool.Spec().Name)
	}
	kept := r.order[:0]
	for _, name := range r.order {
		if _, stillThere := r.tools[name]; stillThere {
			kept = append(kept, name)
		}
	}
	r.order = kept
	// 再挂新段：撞内置名的丢弃，不进 order 也不进 tools。
	fresh := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		spec := tool.Spec()
		if _, clash := r.tools[spec.Name]; clash {
			continue
		}
		r.order = append(r.order, spec.Name)
		r.tools[spec.Name] = tool
		fresh = append(fresh, tool)
	}
	r.mcpTools = fresh
}

// Specs 全部工具声明，按注册顺序。
func (r *Registry) Specs() []ToolSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].Spec())
	}
	return out
}

// Run 按名字执行一次工具调用。
func (r *Registry) Run(ctx context.Context, name, rawArgs string) Result {
	r.mu.Lock()
	tool, ok := r.tools[name]
	r.mu.Unlock()
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
	// 技能正文只读：读写都走 resolve，而技能要能被 read 才用得上。
	// 改技能只有一个入口（技能页那套服务），工具改的是副本还是原件都说不清。
	// 这里按入参原文先拦一道；resolve 里对解析后的路径（含真实路径）再拦一道，
	// 相对路径与符号链接都绕不过去。
	if IsMutatingTool(name) && r.Skills().IsFile(stringArg(args, "path")) {
		return errorf("技能正文只读，请在技能页里修改：%s", stringArg(args, "path"))
	}
	return tool.Run(ctx, r, args)
}

// resolve 把工具入参里的路径解析成工作区内的绝对路径。
// 越界一律拒：模型给的路径来自对话内容，不能成为逃出工作区的通道。
// 真实路径也要校验：工作区里的符号链接 / junction 能把 ../ 守卫绕过去。
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
	if !pkg.Inside(workspace, clean) || !pkg.InsideResolved(workspace, clean) {
		// 技能正文在数据目录里，没有这条豁免模型就永远读不到它要遵循的说明。
		if !r.Skills().IsFile(clean) {
			return "", pkg.Err(pkg.CodePathOutside, "路径不在工作区内", clean)
		}
	}
	return clean, nil
}

// resolveWrite 解析写类工具的目标路径：技能文件一律拒。
// 判定放在解析之后：相对路径、大小写变体、工作区内指向技能的符号链接，
// 只有落到最终路径（含真实路径）上才判得准。
func (r *Registry) resolveWrite(raw string) (string, error) {
	clean, err := r.resolve(raw)
	if err != nil {
		return "", err
	}
	if r.Skills().IsFile(clean) || r.Skills().IsFile(pkg.RealPath(clean)) {
		return "", pkg.Err(pkg.CodePathOutside, "技能正文只读，请在技能页里修改", clean)
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

// intArg 取整数入参；缺失或认不出给 fallback。
// 三种形态都收：JSON 数字、数字字符串、布尔。
// 声明里参数一律是 string（`schemaOf`），照声明办事的模型会发 "20"——
// 只认 float64 的话它拿到的永远是缺省值，而且没有任何提示。
func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fallback
		}
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			return fallback
		}
		return n
	}
	return fallback
}

// boolArg 取布尔入参：同样收字符串形态（"true" / "1" / "yes" 为真）。
func boolArg(args map[string]any, key string) bool {
	switch value := args[key].(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes", "on":
			return true
		}
	}
	return false
}
