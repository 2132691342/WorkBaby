package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	ErrDuplicateTool = pkg.New(4201, "工具重复注册", "")
	ErrNoToolName    = pkg.New(4203, "工具名不能为空", "")
)

// Registry 是工具注册中心：并发只读为主，写只在启动期发生。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

// New 构造空注册中心。
func New() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register 注册一个工具。空名、重名、Schema 不合法都在这里当场报错：
// 参数声明坏掉的工具留到运行期，只会变成一条看不懂的模型报错。
func (r *Registry) Register(t Tool) error {
	name := t.Name()
	if strings.TrimSpace(name) == "" {
		return ErrNoToolName
	}
	sch, err := compileSchema(name, t.Parameters())
	if err != nil {
		return pkg.Wrap(4202, fmt.Sprintf("工具 %s 的参数声明不合法", name), err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; ok {
		return pkg.Wrap(4201, "工具重复注册", ErrDuplicateTool)
	}
	r.tools[name] = t
	r.order = append(r.order, name)
	cacheSchema(name, sch)
	return nil
}

// Get 按名字取工具。
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All 按注册顺序列出全部工具。
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

// List 按工具名排序列出全部工具：声明顺序稳定才能命中上游的提示词缓存，
// 排查「模型怎么少了某个工具」时也更容易逐行比对。
func (r *Registry) List() []Tool {
	out := r.All()
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Enabled 按启用名单过滤；名单为空时全部启用。
func (r *Registry) Enabled(disabled []string) []Tool {
	blocked := map[string]bool{}
	for _, n := range disabled {
		blocked[n] = true
	}
	out := []Tool{}
	for _, t := range r.All() {
		if !blocked[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

// Defs 把工具转成上游需要的声明，顺序稳定以便提示词可复现。
func (r *Registry) Defs(tools []Tool) []llm.ToolDef {
	names := make([]string, 0, len(tools))
	byName := map[string]Tool{}
	for _, t := range tools {
		names = append(names, t.Name())
		byName[t.Name()] = t
	}
	sort.Strings(names)
	out := make([]llm.ToolDef, 0, len(names))
	for _, n := range names {
		t := byName[n]
		out = append(out, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return out
}

// ValidateSchemas 启动期全量自检一次参数声明，坏 schema 不许进运行期。
func (r *Registry) ValidateSchemas() error {
	for _, t := range r.List() {
		if _, err := compiledSchema(t); err != nil {
			return pkg.Wrap(4202, fmt.Sprintf("工具 %s 的参数声明不合法", t.Name()), err)
		}
	}
	return nil
}

// ValidateArgs 执行前校验参数，拦住模型给出的坏输入。
func ValidateArgs(t Tool, args map[string]any) error {
	sch, err := compiledSchema(t)
	if err != nil {
		return pkg.Wrap(4202, "编译参数校验器失败", err)
	}
	// 无参工具调用会带 nil，按空对象处理，否则会被 schema 判为不合法。
	if args == nil {
		args = map[string]any{}
	}
	inst, err := json.Marshal(args)
	if err != nil {
		return pkg.Wrap(4002, "工具参数无法序列化", err)
	}
	var value any
	if err := json.Unmarshal(inst, &value); err != nil {
		return pkg.Wrap(4002, "工具参数不是合法 JSON", err)
	}
	if err := sch.Validate(value); err != nil {
		return pkg.Wrap(4002, "工具参数不正确", err)
	}
	return nil
}

var (
	schemaMu sync.RWMutex
	schemas  = map[string]*jsonschema.Schema{}
)

// compiledSchema 复用注册期编译好的校验器：每次调用都新建编译器会把开销
// 摊到每一轮的每个工具上，而同一份 schema 编译结果是不可变的。
func compiledSchema(t Tool) (*jsonschema.Schema, error) {
	schemaMu.RLock()
	sch, ok := schemas[t.Name()]
	schemaMu.RUnlock()
	if ok {
		return sch, nil
	}
	sch, err := compileSchema(t.Name(), t.Parameters())
	if err != nil {
		return nil, err
	}
	cacheSchema(t.Name(), sch)
	return sch, nil
}

func cacheSchema(name string, sch *jsonschema.Schema) {
	schemaMu.Lock()
	schemas[name] = sch
	schemaMu.Unlock()
}

// compileSchema 把参数声明编译成校验器；编译器不跨工具共享。
func compileSchema(name string, params map[string]any) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(name+".json", doc); err != nil {
		return nil, err
	}
	return c.Compile(name + ".json")
}
