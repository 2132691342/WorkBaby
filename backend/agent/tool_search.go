package agent

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"WorkBaby/backend/pkg"
)

// skipDirs 是递归搜索时一律跳过的目录。
// node_modules 与 .git 能把一次搜索放大成几十万文件，几秒变几分钟，
// 而答案几乎从来不在里面。
var skipDirs = map[string]struct{}{
	"node_modules": {}, ".git": {}, "dist": {}, "build": {},
	".next": {}, "target": {}, "vendor": {}, "__pycache__": {}, ".venv": {},
}

// grepTool 在文件内容里正则搜索。
type grepTool struct{}

// Spec 工具声明。
func (t *grepTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "grep",
		Description: "在工作区内的文件内容里按正则表达式搜索，返回命中的文件、行号与整行内容。",
		Schema: schemaOf(map[string]any{
			"pattern":     "正则表达式",
			"path":        "可选，搜索范围目录或文件；默认工作区根",
			"ignore_case": "可选，true 表示大小写不敏感",
		}, "pattern"),
	}
}

// Run 执行搜索。
func (t *grepTool) Run(ctx context.Context, reg *Registry, args map[string]any) Result {
	pattern := stringArg(args, "pattern")
	if pattern == "" {
		return errorf("pattern 不能为空")
	}
	expression := pattern
	if boolArg(args, "ignore_case") {
		expression = "(?i)" + pattern
	}
	re, err := regexp.Compile(expression)
	if err != nil {
		return errorf("正则写错了：%v", err)
	}
	root := stringArg(args, "path")
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	start, resolveErr := reg.resolve(root)
	if resolveErr != nil {
		return errorf("%s", pkg.As(resolveErr).Message)
	}
	var hits []string
	total := 0
	walkErr := walkTextFiles(ctx, start, func(file string) bool {
		if total >= maxGrepHits {
			return false
		}
		raw, readErr := os.ReadFile(file)
		if readErr != nil || strings.ContainsRune(string(raw), 0) {
			return true
		}
		for index, line := range pkg.SplitLines(string(raw)) {
			if !re.MatchString(line) {
				continue
			}
			total++
			hits = append(hits, reg.relative(file)+":"+itoa(index+1)+": "+pkg.Truncate(strings.TrimSpace(line), 300))
			if total >= maxGrepHits {
				return false
			}
		}
		return true
	})
	if walkErr != nil {
		return errorf("搜索失败：%v", walkErr)
	}
	if len(hits) == 0 {
		return Result{Text: "没有匹配的文件内容"}
	}
	header := "命中 " + itoa(len(hits)) + " 处"
	if len(hits) >= maxGrepHits {
		header += "（已截断到上限 " + itoa(maxGrepHits) + " 处，请缩小范围）"
	}
	return Result{Text: header + "\n" + strings.Join(hits, "\n")}
}

// maxGrepHits 是搜索命中的上限。
const maxGrepHits = 200

// findTool 按文件名搜索。
type findTool struct{}

// Spec 工具声明。
func (t *findTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "find",
		Description: "按文件名在工作区里查找文件或目录，模式用通配符（如 *.go、**/*.ts）。",
		Schema: schemaOf(map[string]any{
			"pattern": "文件名通配符，例如 *.go 或 main.*",
			"path":    "可选，起始目录；默认工作区根",
		}, "pattern"),
	}
}

// Run 执行查找。
func (t *findTool) Run(ctx context.Context, reg *Registry, args map[string]any) Result {
	pattern := stringArg(args, "pattern")
	if pattern == "" {
		return errorf("pattern 不能为空")
	}
	root := stringArg(args, "path")
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	start, err := reg.resolve(root)
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	// 只取基名匹配：用户写 *.go 时想要的是「所有 go 文件」，
	// 而不是「工作区根下的 go 文件」。
	base := filepath.Base(pattern)
	matches := make([]string, 0, 64)
	walkErr := walkAll(ctx, start, func(path string, entry fs.DirEntry) bool {
		ok, matchErr := filepath.Match(base, entry.Name())
		if matchErr == nil && ok {
			target := reg.relative(path)
			if entry.IsDir() {
				target += "/"
			}
			matches = append(matches, target)
		}
		return len(matches) < maxFindHits
	})
	if walkErr != nil {
		return errorf("查找失败：%v", walkErr)
	}
	if len(matches) == 0 {
		return Result{Text: "没有匹配的文件"}
	}
	sort.Strings(matches)
	header := "匹配 " + itoa(len(matches)) + " 项"
	if len(matches) >= maxFindHits {
		header += "（已截断到上限）"
	}
	return Result{Text: header + "\n" + strings.Join(matches, "\n")}
}

// maxFindHits 是文件名匹配的上限。
const maxFindHits = 300

// walkTextFiles 递归遍历文本文件；visit 返回 false 即停止。
func walkTextFiles(ctx context.Context, root string, visit func(file string) bool) error {
	return walkAll(ctx, root, func(path string, entry fs.DirEntry) bool {
		if entry.IsDir() || !isTextPath(entry.Name()) {
			return true
		}
		return visit(path)
	})
}

// walkAll 递归遍历；visit 返回 false 即停止。单个目录读不了就跳过，不中断整趟。
func walkAll(ctx context.Context, root string, visit func(path string, entry fs.DirEntry) bool) error {
	keepGoing := true
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// 权限不足的目录不该让整次搜索失败。
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if entry.IsDir() {
			if path != root {
				if _, skip := skipDirs[strings.ToLower(entry.Name())]; skip {
					return fs.SkipDir
				}
			}
			return nil
		}
		if !visit(path, entry) {
			keepGoing = false
			return fs.SkipAll
		}
		return nil
	})
	if !keepGoing && err == nil {
		return nil
	}
	return err
}
