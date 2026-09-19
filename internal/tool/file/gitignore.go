package file

// .gitignore 感知：检索与遍历前先问它，避免把构建产物、依赖树、生成物当源码翻。
// 支持日常仓库够用的子集：空行与 # 注释跳过、! 反向规则（后匹配者优先）、
// 以 / 结尾只匹配目录、含 / 的模式相对 .gitignore 所在目录锚定、通配支持 * / ? / **。
// 只读工作区根与检索起点的 .gitignore，不处理嵌套与非转义细节。

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ignoreRule 单条 .gitignore 规则（模式已编译）。
type ignoreRule struct {
	re       *regexp.Regexp
	negate   bool
	dirOnly  bool
	hasSlash bool
	anchored bool
}

// ignoreMatcher 一个 .gitignore 的规则集；nil 表示无规则（调用方无需判空）。
type ignoreMatcher struct {
	rules []ignoreRule
}

// matcherRoot 一个 .gitignore 的规则集与它作用的相对基准。
type matcherRoot struct {
	matcher *ignoreMatcher
	base    string // 判定时先剥掉的前缀（相对 root 的斜杠路径）
}

// IgnoreChecker 组合工作区根与检索起点的 .gitignore（构造期解析一次，之后只读，可并发用）。
type IgnoreChecker struct {
	matchers []matcherRoot
}

// NewIgnoreChecker 加载 root 与 base（可为空 / 相同）下的 .gitignore；都缺失返回 nil。
func NewIgnoreChecker(root, base string) *IgnoreChecker {
	seen := map[string]bool{}
	var ms []matcherRoot
	add := func(dir, base string) {
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		if m := loadIgnore(dir); m != nil {
			ms = append(ms, matcherRoot{matcher: m, base: base})
		}
	}
	add(root, "")
	if base != "" && base != root {
		if rel, err := filepath.Rel(root, base); err == nil && !strings.HasPrefix(rel, "..") {
			add(base, filepath.ToSlash(rel))
		}
	}
	if len(ms) == 0 {
		return nil
	}
	return &IgnoreChecker{matchers: ms}
}

// Ignored rel 相对工作区根（/ 分隔）；被任一 .gitignore 命中即忽略。
func (c *IgnoreChecker) Ignored(rel string, isDir bool) bool {
	if c == nil || rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, mr := range c.matchers {
		sub := rel
		if mr.base != "" {
			if sub == mr.base {
				continue // 基准目录自身不判
			}
			if !strings.HasPrefix(sub, mr.base+"/") {
				continue
			}
			sub = strings.TrimPrefix(sub, mr.base+"/")
		}
		if mr.matcher.Ignored(sub, isDir) {
			return true
		}
	}
	return false
}

// loadIgnore 读取 dir/.gitignore；文件不存在或全为空规则返回 nil。
func loadIgnore(dir string) *ignoreMatcher {
	if dir == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	m := &ignoreMatcher{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := ignoreRule{}
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = strings.TrimPrefix(line, "!")
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if strings.HasPrefix(line, "/") {
			r.anchored = true
		}
		r.hasSlash = strings.Contains(strings.TrimPrefix(line, "/"), "/")
		re, err := globRe(line)
		if err != nil {
			continue
		}
		r.re = re
		m.rules = append(m.rules, r)
	}
	if len(m.rules) == 0 {
		return nil
	}
	return m
}

// Ignored 判断相对路径是否被忽略。
// 多条规则命中时后匹配者胜出——`dist/` + `!dist/keep.txt` 才能按预期工作。
func (m *ignoreMatcher) Ignored(rel string, isDir bool) bool {
	if m == nil || rel == "" {
		return false
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	ignored := false
	for _, r := range m.rules {
		if r.matches(rel, isDir) {
			ignored = !r.negate
		}
	}
	return ignored
}

// matches 规则是否命中该路径。
func (r ignoreRule) matches(rel string, isDir bool) bool {
	segs := strings.Split(rel, "/")
	if r.dirOnly && !isDir {
		return r.matchesAncestor(segs)
	}
	if !r.hasSlash && !r.anchored {
		// 无斜杠模式：匹配任意层级的同名项
		for _, s := range segs {
			if r.re.MatchString(s) {
				return true
			}
		}
		return false
	}
	if r.anchored {
		return r.re.MatchString(rel) || r.matchesAncestor(segs)
	}
	// 含斜杠但未锚定：可从任意深度起匹配（git 语义）
	for i := 0; i < len(segs); i++ {
		if r.re.MatchString(strings.Join(segs[i:], "/")) {
			return true
		}
	}
	return r.matchesAncestor(segs)
}

// matchesAncestor 祖先目录被忽略时其后代一并忽略：`node_modules/` 要能盖住 node_modules/a/b.js。
func (r ignoreRule) matchesAncestor(segs []string) bool {
	for i := 1; i < len(segs); i++ {
		if r.re.MatchString(strings.Join(segs[:i], "/")) || r.re.MatchString(segs[i-1]) {
			return true
		}
	}
	return false
}
