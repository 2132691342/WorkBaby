// Package tool 定义工具契约、注册表与 11 个内置工具。
package tool

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// ExecutionMode 决定一批工具调用能否并发执行。
type ExecutionMode string

// 执行模式：sequential 会让整批退化为串行。
const (
	ExecutionSequential ExecutionMode = "sequential"
	ExecutionParallel   ExecutionMode = "parallel"
)

// 输出统一阈值，超出即落临时文件，避免撑爆上下文。
const (
	MaxLines = 2000
	MaxBytes = 50 << 10
)

// Result 是工具产出：Content 回填给模型，Title 给 UI 一行摘要，Detail 是展开后的完整输出。
type Result struct {
	Content string
	Title   string
	Detail  string
	IsError bool
}

// ReadTracker 记录本次会话已读过的文件路径，用于「写前必读」。
type ReadTracker interface {
	HasRead(path string) bool
	MarkRead(path string)
}

// Deps 是工具需要的外界依赖，构造期注入，执行期只读。
type Deps struct {
	PythonExe string
	Knowledge Searcher
	Reads     ReadTracker
	TmpDir    string
}

// Searcher 是知识库检索能力，由 knowledge 包实现。
type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]domain.SearchHitVO, error)
}

// Input 是工具执行的入参。
type Input struct {
	Args      map[string]any
	Workspace string
	Deps      Deps
}

// Tool 是工具契约。标签与准则都写在工具上，系统提示由工具集驱动重建，不需要两处维护。
type Tool interface {
	Name() string
	Label() string
	Description() string
	PromptSnippet() string
	PromptGuidelines() []string
	Parameters() map[string]any
	ExecutionMode() ExecutionMode
	RequiresApproval() bool
	Execute(ctx context.Context, in Input) (*Result, error)
}

// Categorized 是实现了分类的可选接口。
// 分类只影响「工具」菜单里的分组展示，不参与模型调用，因此不强加在 Tool 上：
// 测试用的替身工具不必为了编译通过去实现它。
type Categorized interface {
	Category() string
}

// CategoryOf 取工具分类，未实现时归到「文件」这一兜底类。
func CategoryOf(t Tool) string {
	if c, ok := t.(Categorized); ok {
		if v := c.Category(); v != "" {
			return v
		}
	}
	return domain.CategoryFile
}

// ParamNames 从参数声明里取出参数名，按声明顺序返回，供界面渲染用法说明。
func ParamNames(params map[string]any) []string {
	props, ok := params["properties"].(map[string]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(props))
	for name := range props {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

var (
	ErrMissingArg = pkg.New(4001, "工具参数不完整", "")
	ErrBadArg     = pkg.New(4002, "工具参数不正确", "")
	ErrTimedOut   = pkg.New(4003, "执行超时", "")
)

// Str 取字符串参数。
func Str(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// Int 取整数参数，缺失或类型不符返回默认值。
func Int(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return def
	}
}

// Slice 取数组参数。
func Slice(args map[string]any, key string) []any {
	if v, ok := args[key].([]any); ok {
		return v
	}
	return nil
}

// Truncate 按行数与字节数截断；字节上限回退到字符边界，避免切出半个汉字。
func Truncate(s string, tmpDir string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > MaxLines {
		s = strings.Join(lines[:MaxLines], "\n")
	}
	return CutBytes(s, MaxBytes)
}

// CutBytes 按字节数截断并回退到 UTF-8 字符边界。半个字符进了上下文，
// 后续每一次编码处理都会带着它出错。
func CutBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// Overflow 把完整输出落临时文件，返回给模型看的一句话。
func Overflow(full, tmpDir, prefix string) string {
	if tmpDir == "" {
		return "（输出已截断，且未配置临时目录，完整内容无法保存）"
	}
	path := tmpDir + string('/') + pkg.TempName(prefix, ".txt")
	if err := pkg.WriteText(path, full); err != nil {
		return "（输出已截断）"
	}
	return "……（输出过长已截断，完整内容见 " + path + "）"
}

// Cut 统一截断入口：未超阈值原样返回，超出则截断并附完整路径。
func Cut(s, tmpDir, prefix string) string {
	if len(strings.Split(s, "\n")) <= MaxLines && len(s) <= MaxBytes {
		return s
	}
	return Truncate(s, tmpDir) + "\n" + Overflow(s, tmpDir, prefix)
}

// Since 取耗时毫秒，供工具结果回执使用。
func Since(start time.Time) int64 { return time.Since(start).Milliseconds() }
