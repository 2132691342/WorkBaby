package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 跳过的目录：版本库 / 过程数据 / 依赖树，检索与遍历都无意义且量大。
var skipDirs = map[string]bool{
	".git": true, ".workbaby": true, "node_modules": true, ".idea": true, ".vscode": true,
}

// maxSearchBytes 单文件读取上限：超过按前缀匹配（大 dump 文件不进检索）。
const maxSearchBytes = 4 << 20 // 4MB

// isBinary 前 512 字节含 NUL 即视为二进制（检索按文本语义，二进制命中无意义）。
func isBinary(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(bufio.NewReader(f), buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

// ===== file_grep =====

// GrepTool 工作区内容检索（file_grep）：正则逐行匹配，返回 file:line: text。
type GrepTool struct{ resolve func(context.Context) string }

// NewGrep 构造 file_grep。
func NewGrep(resolver tool.RootResolver, defRoot string) *GrepTool {
	return &GrepTool{resolve: tool.ResolveRoot(resolver, defRoot)}
}

func (t *GrepTool) Name() string              { return "file_grep" }
func (t *GrepTool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }
func (t *GrepTool) Description() string {
	return "在工作区内按正则检索文件内容，返回 `路径:行号: 行文本`。定位代码/配置用法时优先于 file_read。"
}

func (t *GrepTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["pattern"],
			"properties": {
				"pattern": {"type": "string", "description": "Go 正则表达式（RE2 语法）"},
				"path": {"type": "string", "description": "检索起始目录，缺省工作区根"},
				"include": {"type": "string", "description": "文件名通配过滤，如 *.go"},
				"max_results": {"type": "integer", "description": "最多返回行数，缺省 100 上限 500"}
			}
		}`),
	}
}

// Meta 声明：纯读、文件组、信息类动作。
func (t *GrepTool) Meta() tool.ToolMeta {
	return tool.ToolMeta{Group: tool.GroupFile, ReadOnly: true, Category: tool.CategoryInfo, ActivityDesc: "检索内容", MaxResultChars: 20_000}
}

// ActivityDescription 时间线文案。
func (t *GrepTool) ActivityDescription(args json.RawMessage) string {
	var req grepReq
	if json.Unmarshal(args, &req) != nil || req.Pattern == "" {
		return ""
	}
	p := req.Pattern
	if r := []rune(p); len(r) > 40 {
		p = string(r[:40]) + "…"
	}
	return "正在检索「" + p + "」"
}

type grepReq struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Include    string `json:"include"`
	MaxResults int    `json:"max_results"`
}

func (t *GrepTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req grepReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "file_grep args parse failed", err)}
	}
	if len(req.Pattern) > 512 {
		return tool.ToolResult{Err: pkg.New(4001, "pattern too long", "")}
	}
	re, err := regexp.Compile(req.Pattern)
	if err != nil {
		return tool.ToolResult{Err: pkg.New(4001, "invalid regex: "+err.Error(), req.Pattern)}
	}
	limit := req.MaxResults
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	root := t.resolve(ctx)
	base := root
	if req.Path != "" {
		if base, err = safePath(root, req.Path); err != nil {
			return tool.ToolResult{Err: err}
		}
	}

	var sb strings.Builder
	matchedFiles := 0
	hits := 0
	fileErrs := 0
	ig := NewIgnoreChecker(root, base)
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil // 单个不可访问路径不中断检索
		}
		if rel, rerr := filepath.Rel(root, p); rerr == nil && ig.Ignored(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		name := d.Name()
		if req.Include != "" {
			if ok, _ := filepath.Match(req.Include, name); !ok {
				return nil
			}
		}
		if isBinary(p) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			fileErrs++
			return nil
		}
		defer func() { _ = f.Close() }() //nolint:revive
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchBytes {
			return nil
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNo := 0
		fileHit := false
		for sc.Scan() {
			lineNo++
			if re.Match(sc.Bytes()) {
				text := sc.Text()
				if r := []rune(text); len(r) > 200 {
					text = string(r[:200]) + "…"
				}
				rel, _ := filepath.Rel(root, p)
				fmt.Fprintf(&sb, "%s:%d: %s\n", filepath.ToSlash(rel), lineNo, text)
				hits++
				fileHit = true
				if hits >= limit {
					sb.WriteString("... (more matches omitted)\n")
					return fs.SkipAll
				}
			}
		}
		if fileHit {
			matchedFiles++
		}
		return nil
	})
	if walkErr != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "grep walk failed", walkErr)}
	}
	out := strings.TrimRight(sb.String(), "\n")
	if out == "" {
		out = "no matches"
	}
	return tool.ToolResult{Content: out, Meta: map[string]string{
		"matched_files": fmt.Sprintf("%d", matchedFiles),
		"hits":          fmt.Sprintf("%d", hits),
	}}
}

// ===== file_glob =====

// GlobTool 文件名模式匹配（file_glob）：支持 `**` 跨层递归，返回相对路径。
type GlobTool struct{ resolve func(context.Context) string }

// NewGlob 构造 file_glob。
func NewGlob(resolver tool.RootResolver, defRoot string) *GlobTool {
	return &GlobTool{resolve: tool.ResolveRoot(resolver, defRoot)}
}

func (t *GlobTool) Name() string              { return "file_glob" }
func (t *GlobTool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }
func (t *GlobTool) Description() string {
	return "按通配模式列出工作区文件（相对路径），支持 ** 跨层匹配，如 src/**/*.go。"
}

func (t *GlobTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["pattern"],
			"properties": {
				"pattern": {"type": "string", "description": "相对工作区根的通配模式；** 匹配任意层目录，如 **/*.test.go"}
			}
		}`),
	}
}

// Meta 声明：纯读、文件组、信息类动作。
func (t *GlobTool) Meta() tool.ToolMeta {
	return tool.ToolMeta{Group: tool.GroupFile, ReadOnly: true, Category: tool.CategoryInfo, ActivityDesc: "查找文件", MaxResultChars: 20_000}
}

// ActivityDescription 时间线文案。
func (t *GlobTool) ActivityDescription(args json.RawMessage) string {
	var req struct {
		Pattern string `json:"pattern"`
	}
	if json.Unmarshal(args, &req) != nil || req.Pattern == "" {
		return ""
	}
	return "正在查找 " + req.Pattern
}

// globRe 把通配模式（支持 **）转成正则：** → 任意路径段（含空）、* → 单段内任意、? → 单字符。
func globRe(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > 256 {
		return nil, pkg.New(4001, "pattern too long", "")
	}
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					sb.WriteString("(?:.*/)?")
					i += 2
				} else {
					sb.WriteString(".*")
					i++
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

func (t *GlobTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "file_glob args parse failed", err)}
	}
	re, err := globRe(strings.TrimLeft(filepath.ToSlash(req.Pattern), "./"))
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	root := t.resolve(ctx)
	var out []string
	ig := NewIgnoreChecker(root, "")
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr == nil && ig.Ignored(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if rerr != nil {
			return nil
		}
		if re.MatchString(filepath.ToSlash(rel)) {
			out = append(out, filepath.ToSlash(rel))
			if len(out) >= 500 {
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "glob walk failed", err)}
	}
	if len(out) == 0 {
		return tool.ToolResult{Content: "no matches", Meta: map[string]string{"pattern": req.Pattern}}
	}
	return tool.ToolResult{
		Content: strings.Join(out, "\n"),
		Meta:    map[string]string{"count": fmt.Sprintf("%d", len(out))},
	}
}
