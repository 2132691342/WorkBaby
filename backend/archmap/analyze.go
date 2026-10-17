// Package archmap 本地静态分析项目的模块与依赖关系（specs/13）。
// 只读：正则解析 import，无语法树依赖；动态 import 漏解析由覆盖率兜住。
package archmap

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 扫描上限：巨型 monorepo 不该把页面挂死。
const (
	maxFiles       = 100000
	analyzeTimeout = 10 * time.Second
	maxDirNodes    = 300 // 图节点上限：再多渲染与可读性一起崩
)

// 需要跳过的目录：依赖与产物不是项目结构。
var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"out": true, "target": true, "vendor": true, ".idea": true,
	".vscode": true, "__pycache__": true, ".venv": true, "bin": true,
	"obj": true, ".next": true, ".nuxt": true,
}

// 可解析语言 → import 语句正则。
// JS/TS/Vue/Go 是桌面端项目的主力；其余语言只做文件级索引。
var (
	reJSImport  = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+(?:[\s\S]*?from\s*)?['"]([^'"]+)['"]`)
	reJSRequire = regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\s*\)`)
	reGoImport  = regexp.MustCompile(`(?m)^\s*(?:import\s+(?:\(\s*)?|[\w.]+\s+)"([^"]+)"`)
	reDynamic   = regexp.MustCompile(`import\(\s*['"` + "`" + `]`)
)

// fileStat 一个文件的行数与 import 目标。
type fileStat struct {
	rel     string
	lines   int
	imports []string // 规范化的相对模块路径
	dynamic bool
	skipped bool
}

// Analyze 分析 root，返回分层依赖图。
func Analyze(root string) (*domain.ArchGraph, error) {
	if !pkg.DirExists(root) {
		return nil, domain.ErrArchDirMissing
	}
	deadline := time.Now().Add(analyzeTimeout)
	g := &domain.ArchGraph{Root: root, Nodes: []domain.ArchNode{}, Edges: []domain.ArchEdge{}}

	var files []fileStat
	var entries []domain.ArchEntrypointVO
	timedOut := false

	var walk func(dir, rel string, depth int)
	walk = func(dir, rel string, depth int) {
		if len(files) >= maxFiles {
			return
		}
		if time.Now().After(deadline) {
			timedOut = true
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if len(files) >= maxFiles || timedOut {
				return
			}
			name := e.Name()
			if e.IsDir() {
				if skipDirs[name] || strings.HasPrefix(name, ".") {
					continue
				}
				sub := name
				if rel != "" {
					sub = rel + "/" + name
				}
				walk(filepath.Join(dir, name), sub, depth+1)
				continue
			}
			ext := strings.ToLower(filepath.Ext(name))
			relPath := name
			if rel != "" {
				relPath = rel + "/" + name
			}
			st := fileStat{rel: relPath}
			switch ext {
			case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".go":
				lines, imports, dyn := parseFile(filepath.Join(dir, name), ext)
				st.lines, st.imports, st.dynamic = lines, imports, dyn
				g.Coverage.Parsed++
			default:
				st.skipped = true
				g.Coverage.Skipped++
			}
			if st.dynamic {
				g.Coverage.Dynamic++
			}
			if ep := entrypointKind(name); ep != "" {
				entries = append(entries, domain.ArchEntrypointVO{Path: relPath, Kind: ep, Label: name})
			}
			files = append(files, st)
		}
	}
	walk(root, "", 0)

	if len(files) == 0 {
		return nil, domain.ErrArchNoCode
	}
	if timedOut {
		return nil, domain.ErrArchTimeout
	}

	// 文件 → 所属目录（模块）映射：import 的目标文件归并到它的目录。
	dirOf := func(rel string) string {
		i := strings.LastIndex(rel, "/")
		if i < 0 {
			return ""
		}
		return rel[:i]
	}
	known := map[string]bool{} // 存在的相对文件（含扩展名解析前的形态）
	for _, f := range files {
		known[f.rel] = true
	}

	type agg struct {
		files, lines int
	}
	dirs := map[string]*agg{}
	edges := map[[2]string]bool{}
	for _, f := range files {
		d := dirOf(f.rel)
		a := dirs[d]
		if a == nil {
			a = &agg{}
			dirs[d] = a
		}
		a.files++
		a.lines += f.lines
		for _, imp := range f.imports {
			target := resolveImport(imp, f.rel, known)
			if target == "" {
				continue
			}
			td := dirOf(target)
			if td == d {
				continue // 目录内依赖不进模块图（否则图变成毛线）
			}
			edges[[2]string{d, td}] = true
		}
	}

	ids := make([]string, 0, len(dirs))
	for d := range dirs {
		ids = append(ids, d)
	}
	sort.Strings(ids)
	if len(ids) > maxDirNodes {
		ids = ids[:maxDirNodes]
	}

	// 入度：被依赖次数（自环与重复边已去）。
	indeg := map[string]int{}
	for d := range dirs {
		indeg[d] = 0
	}
	for e := range edges {
		indeg[e[1]]++
	}

	// 深度：从入度 0 的节点（入口侧）BFS。环上的节点深度取 0——
	// 分层布局确定性优先，环在依赖图里本就是坏味道，标出来即可。
	adj := map[string][]string{}
	for e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
	}
	depth := map[string]int{}
	queue := []string{}
	for _, d := range ids {
		if indeg[d] == 0 {
			depth[d] = 0
			queue = append(queue, d)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if _, ok := dirs[next]; !ok {
				continue
			}
			if depth[next] < depth[cur]+1 {
				depth[next] = depth[cur] + 1
				queue = append(queue, next)
			}
		}
	}

	for _, d := range ids {
		a := dirs[d]
		label := filepath.Base(d)
		if d == "" {
			label = "(根目录)"
		}
		g.Nodes = append(g.Nodes, domain.ArchNode{
			ID: d, Label: label, Files: a.files, Lines: a.lines,
			Indegree: indeg[d], Depth: depth[d],
		})
	}
	for e := range edges {
		g.Edges = append(g.Edges, domain.ArchEdge{From: e[0], To: e[1]})
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	return g, nil
}

// Entrypoints 列入口文件：main / index / App / 配置。
func Entrypoints(root string) ([]domain.ArchEntrypointVO, error) {
	if !pkg.DirExists(root) {
		return nil, domain.ErrArchDirMissing
	}
	var out []domain.ArchEntrypointVO
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if k := entrypointKind(d.Name()); k != "" {
			out = append(out, domain.ArchEntrypointVO{Path: filepath.ToSlash(rel), Kind: k, Label: d.Name()})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// parseFile 读一个源文件：行数 + import 目标 + 是否有动态引用。
func parseFile(path, ext string) (int, []string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, false
	}
	defer f.Close()
	var imports []string
	dynamic := false
	lines := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines++
		line := sc.Text()
		if reDynamic.MatchString(line) {
			dynamic = true
		}
		for _, m := range reJSImport.FindAllStringSubmatch(line, -1) {
			imports = append(imports, m[1])
		}
		for _, m := range reJSRequire.FindAllStringSubmatch(line, -1) {
			imports = append(imports, m[1])
		}
		if ext == ".go" {
			for _, m := range reGoImport.FindAllStringSubmatch(line, -1) {
				if !strings.Contains(m[1], ".") {
					continue // 标准库没有点号，项目内 import 必有模块路径
				}
				imports = append(imports, m[1])
			}
		}
	}
	return lines, imports, dynamic
}

// resolveImport 把 import 目标解析成项目内相对文件路径；解析不出（包名 /
// 别名 / 绝对路径）返回空——覆盖率里会体现。
func resolveImport(imp, fromFile string, known map[string]bool) string {
	imp = strings.TrimSpace(imp)
	if imp == "" || strings.HasPrefix(imp, "http") || strings.HasPrefix(imp, "node:") {
		return ""
	}
	dir := ""
	if i := strings.LastIndex(fromFile, "/"); i >= 0 {
		dir = fromFile[:i]
	}
	base := imp
	if strings.HasPrefix(imp, ".") || strings.HasPrefix(imp, "/") {
		// 相对 / 绝对路径：拼到当前目录
		joined := filepath.ToSlash(filepath.Join(strings.TrimPrefix(dir, ""), imp))
		base = strings.TrimPrefix(joined, "./")
	} else if strings.HasPrefix(imp, "@/") || strings.HasPrefix(imp, "~/") {
		// 常见别名：@/ 指向 src 或根。先试 src/ 再试根。
		for _, prefix := range []string{"src/", ""} {
			cand := prefix + imp[2:]
			if m := matchFile(cand, known); m != "" {
				return m
			}
		}
		return ""
	} else {
		return "" // 裸包名：node_modules / 外部依赖，不是项目内模块
	}
	return matchFile(base, known)
}

// matchFile 按「带扩展名 / 目录 index / 常见扩展名」的顺序找真实文件。
func matchFile(base string, known map[string]bool) string {
	candidates := []string{
		base,
		base + ".ts", base + ".tsx", base + ".js", base + ".jsx", base + ".mjs",
		base + ".vue", base + ".go",
		base + "/index.ts", base + "/index.tsx", base + "/index.js", base + "/index.vue",
	}
	for _, c := range candidates {
		if known[c] {
			return c
		}
	}
	return ""
}

// entrypointKind 识别入口文件；空表示不是。
func entrypointKind(name string) string {
	switch name {
	case "main.go", "main.ts", "main.js", "main.py", "index.ts", "index.js",
		"index.html", "App.vue", "app.go", "mod.go":
		return "main"
	case "package.json", "go.mod", "tsconfig.json", "vite.config.ts", "wails.json":
		return "config"
	default:
		return ""
	}
}
