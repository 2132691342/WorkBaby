package tool

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"WorkBaby/backend/pkg"
)

// 默认跳过的目录：只放「与内容无关且体积巨大」的确定性目标。
// 不跳 build / dist / vendor：它们在不少项目里就是源码目录，跳过等于永远搜不到。
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".svn": true, ".hg": true,
	"__pycache__": true, ".venv": true, ".workbaby": true,
}

const (
	maxWalk = 20000 // 遍历文件数上限，防止在巨型目录里跑不完
	maxHits = 200   // 命中数上限：同一个词反复命中对模型没有新信息
	// grepFileMax 单文件读取上限：源码文本几乎不会超过它。
	// 不加闸时搜一个几 GB 的日志/数据文件会把整文件读进内存，是「助手卡死」的典型来源。
	grepFileMax = 8 << 20
	hitLineMax  = 500 // 单行命中截断长度：压缩过的 JS 一行能有几十万字符
)

// lsTool 列目录。
type lsTool struct{}

func (lsTool) Name() string                 { return "ls" }
func (lsTool) Label() string                { return "看目录" }
func (lsTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (lsTool) RequiresApproval() bool       { return false }
func (lsTool) Description() string {
	return "列出目录下的文件与子目录，目录名带 / 后缀。"
}
func (lsTool) PromptSnippet() string { return "查看目录里有什么" }
func (lsTool) PromptGuidelines() []string {
	return []string{"不确定文件在哪时，先用 ls 看一眼，不要凭空猜路径。"}
}

func (lsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "目录路径，默认工作目录"},
		},
	}
}

func (t lsTool) Execute(ctx context.Context, in Input) (*Result, error) {
	target := Str(in.Args, "path")
	if target == "" {
		target = "."
	}
	full, err := pkg.SafeJoin(in.Workspace, target)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, pkg.Wrap(1011, "打开目录失败", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name + "\n")
	}
	out := b.String()
	if out == "" {
		out = "（空目录）"
	}
	return &Result{
		Content: Cut(out, in.Deps.TmpDir, "ls"),
		Title:   fmt.Sprintf("看了 %s（%d 项）", pkg.RelPath(in.Workspace, full), len(entries)),
		Detail:  out,
	}, nil
}

// findTool 按 glob 找文件。
type findTool struct{}

func (findTool) Name() string                 { return "find" }
func (findTool) Label() string                { return "找文件" }
func (findTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (findTool) RequiresApproval() bool       { return false }
func (findTool) Description() string {
	return "按文件名匹配查找文件，如 *.xlsx、**/*.xlsx 或 *报告*。自动跳过无关大目录。"
}
func (findTool) PromptSnippet() string { return "按名字找文件" }
func (findTool) PromptGuidelines() []string {
	return []string{"知道文件名但不知道位置时用 find；要搜内容用 grep。"}
}

func (findTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "文件名匹配式，如 *.md"},
			"path":    map[string]any{"type": "string", "description": "起始目录，默认工作目录"},
		},
		"required": []string{"pattern"},
	}
}

func (t findTool) Execute(ctx context.Context, in Input) (*Result, error) {
	pattern := Str(in.Args, "pattern")
	if pattern == "" {
		return nil, ErrMissingArg
	}
	target := Str(in.Args, "path")
	if target == "" {
		target = "."
	}
	root, err := pkg.SafeJoin(in.Workspace, target)
	if err != nil {
		return nil, err
	}
	hits := []string{}
	scanned, truncated := 0, false
	walk(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return true
		}
		if len(hits) >= maxHits {
			truncated = true
			return false
		}
		scanned++
		if matchName(pattern, d.Name(), pkg.RelPath(in.Workspace, path)) {
			hits = append(hits, pkg.RelPath(in.Workspace, path))
		}
		return scanned < maxWalk
	})
	truncated = truncated || scanned >= maxWalk
	return resultOfHits("找文件", hits, truncated, in, fmt.Sprintf("按 %s 找到 %d 个文件", pattern, len(hits)))
}

// grepTool 搜文件内容。
type grepTool struct{}

func (grepTool) Name() string                 { return "grep" }
func (grepTool) Label() string                { return "搜内容" }
func (grepTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (grepTool) RequiresApproval() bool       { return false }
func (grepTool) Description() string {
	return "按正则表达式搜索文件内容，返回 路径:行号:内容。长行截断到 500 字符。"
}
func (grepTool) PromptSnippet() string { return "在文件里搜索内容" }
func (grepTool) PromptGuidelines() []string {
	return []string{"搜内容用 grep，不要用命令行工具，输出更干净。"}
}

func (grepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "正则表达式"},
			"path":    map[string]any{"type": "string", "description": "起始目录，默认工作目录"},
			"glob":    map[string]any{"type": "string", "description": "只搜文件名匹配该式的文件，如 *.go"},
		},
		"required": []string{"pattern"},
	}
}

func (t grepTool) Execute(ctx context.Context, in Input) (*Result, error) {
	pattern := Str(in.Args, "pattern")
	if pattern == "" {
		return nil, ErrMissingArg
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, pkg.Wrap(4002, "搜索表达式不正确", err)
	}
	target := Str(in.Args, "path")
	if target == "" {
		target = "."
	}
	root, err := pkg.SafeJoin(in.Workspace, target)
	if err != nil {
		return nil, err
	}
	glob := Str(in.Args, "glob")

	hits := []string{}
	scanned, truncated := 0, false
	walk(root, func(path string, d fs.DirEntry) bool {
		if d.IsDir() {
			return true
		}
		scanned++
		if glob != "" && !matchName(glob, d.Name(), pkg.RelPath(in.Workspace, path)) {
			return scanned < maxWalk
		}
		if info, err := d.Info(); err == nil && info.Size() > grepFileMax {
			truncated = true
			return scanned < maxWalk
		}
		raw, err := os.ReadFile(path)
		if err != nil || pkg.LooksBinary(raw) {
			return scanned < maxWalk
		}
		rel := pkg.RelPath(in.Workspace, path)
		for i, line := range strings.Split(pkg.DecodeText(raw), "\n") {
			if len(hits) >= maxHits {
				truncated = true
				return false
			}
			if !re.MatchString(line) {
				continue
			}
			if len(line) > hitLineMax {
				line = CutBytes(line, hitLineMax) + "…"
			}
			hits = append(hits, fmt.Sprintf("%s:%d:%s", rel, i+1, line))
		}
		return scanned < maxWalk
	})
	truncated = truncated || scanned >= maxWalk
	return resultOfHits("搜内容", hits, truncated, in, fmt.Sprintf("搜到 %d 处匹配", len(hits)))
}

// matchName 判断文件名是否命中模型写的 glob。
// filepath.Match 不支持 ** 且 * 不跨分隔符，而模型习惯写 **/*.xlsx：
// 去掉 **/ 前缀后按基名匹配；显式带目录的写法按相对路径逐段匹配。
func matchName(pattern, name, rel string) bool {
	p := strings.TrimPrefix(pattern, "./")
	for strings.HasPrefix(p, "**/") {
		p = strings.TrimPrefix(p, "**/")
	}
	if p == "" {
		return true
	}
	if !strings.ContainsAny(p, `/\`) {
		ok, err := filepath.Match(p, name)
		return err == nil && ok
	}
	ok, err := path.Match(p, filepath.ToSlash(rel))
	return err == nil && ok
}

// walk 遍历目录：跳过无关大目录，遵守 .gitignore 的目录与文件名规则。
// 剪枝在进入目录时一次完成，不逐个文件进去再丢。
func walk(root string, fn func(path string, d fs.DirEntry) bool) {
	patterns := loadIgnore(root)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || matchIgnore(patterns, d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchIgnore(patterns, d.Name()) {
			return nil
		}
		if !fn(path, d) {
			return filepath.SkipAll
		}
		return nil
	})
}

// loadIgnore 读 .gitignore：只取目录名与含通配符的行，够用且不会误伤。
func loadIgnore(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		out = append(out, line)
	}
	return out
}

func matchIgnore(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

// resultOfHits 统一命中结果的截断与回执；超限时明确告诉模型「还有更多」。
func resultOfHits(kind string, hits []string, truncated bool, in Input, title string) (*Result, error) {
	if len(hits) == 0 {
		return &Result{Content: "（没有匹配结果）", Title: title, Detail: ""}, nil
	}
	sort.Strings(hits)
	out := strings.Join(hits, "\n") + "\n"
	content := Cut(out, in.Deps.TmpDir, kind)
	if truncated {
		content += "……（结果已截断，还有更多：把范围收窄或换更精确的条件再试）\n"
		title += "（已截断）"
	}
	return &Result{
		Content: content,
		Title:   title,
		Detail:  out,
	}, nil
}
