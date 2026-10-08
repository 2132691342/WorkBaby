// Package tool 定义工具契约、注册表与 11 个内置工具。
package tool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
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

// Retain 决定超出阈值时保留哪一头：命令与脚本类输出的关键信息在结尾。
type Retain string

const (
	RetainHead Retain = "head"
	RetainTail Retain = "tail"
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
	PythonExe     string
	PowerShellExe string
	Knowledge     Searcher
	Reads         ReadTracker
	TmpDir        string
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

// Categorized 是实现了分类的可选接口：分类只影响菜单分组，不参与模型调用。
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
func Truncate(s string, retain Retain) string {
	lines := strings.Split(s, "\n")
	if len(lines) > MaxLines {
		if retain == RetainTail {
			lines = lines[len(lines)-MaxLines:]
		} else {
			lines = lines[:MaxLines]
		}
		s = strings.Join(lines, "\n")
	}
	if retain == RetainTail {
		return cutBytesTail(s, MaxBytes)
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

// cutBytesTail 是保留结尾版本的 CutBytes：往前推到字符边界而不是往后退。
func cutBytesTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := len(s) - max
	for cut < len(s) && !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

// Sanitize 剥掉会让 UI 与 JSON 序列化的控制字符，制表符与换行保留。
// 命令输出里混进一个转义序列就能把整条 SSE 帧打崩，所以在出口统一洗一遍。
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20 || r == 0x7f:
			return -1
		// U+FFF9..U+FFFB 是行间注记标记，不可见但会串进正文
		case r >= 0xfff9 && r <= 0xfffb:
			return -1
		}
		return r
	}, s)
}

// noteOf 描述这一刀砍掉了什么。只说「已截断」的话，模型不知道该整块重取
// 还是换个更窄的范围——丢 3 行和丢 3 万行的续读策略完全不同。
func noteOf(before, full string, path string) string {
	droppedLines := strings.Count(full, "\n") - strings.Count(before, "\n")
	droppedBytes := len(full) - len(before)
	tail := ""
	if path != "" {
		tail = "，完整内容见 " + path
	}
	if droppedLines > 0 {
		return fmt.Sprintf("……（输出过长已截断，还有 %d 行 / %d 字节没给你%s）", droppedLines, droppedBytes, tail)
	}
	return fmt.Sprintf("……（输出过长已截断，还有 %d 字节没给你%s）", droppedBytes, tail)
}

// saveOverflow 把完整输出落临时文件，返回可用路径；无法保存返回空串。
func saveOverflow(full, tmpDir, prefix string) string {
	if tmpDir == "" {
		return ""
	}
	path := tmpDir + string('/') + pkg.TempName(prefix, ".txt")
	if err := pkg.WriteText(path, full); err != nil {
		pkg.Warnf("tool: 写临时输出失败: %v", err)
		return ""
	}
	return path
}

// Cut 统一截断入口：未超阈值原样返回，超出则截断并说明去向。
func Cut(s, tmpDir, prefix string) string { return CutWith(s, tmpDir, prefix, RetainHead) }

// CutTail 同 Cut，但保留结尾：命令 / 脚本的关键信息在最后一行——
// 失败原因、堆栈尾巴、构建结论，只留开头等于永远看不到报错。
func CutTail(s, tmpDir, prefix string) string { return CutWith(s, tmpDir, prefix, RetainTail) }

// CutWith 是截断的唯一实现：衡量 -> 按保留策略裁剪 -> 落盘 -> 组装回执。
func CutWith(s, tmpDir, prefix string, retain Retain) string {
	if len(strings.Split(s, "\n")) <= MaxLines && len(s) <= MaxBytes {
		return s
	}
	kept := Truncate(s, retain)
	return kept + "\n" + noteOf(kept, s, saveOverflow(s, tmpDir, prefix))
}

// Since 取耗时毫秒，供工具结果回执使用。
func Since(start time.Time) int64 { return time.Since(start).Milliseconds() }
