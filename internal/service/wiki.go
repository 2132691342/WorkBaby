// Package service Wiki 仓库导读：扫描工作区生成确定性导读（语言统计 / 入口文件 / 目录树 / 文件正文），不依赖 LLM。
package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// wikiSkipDirs 扫描时跳过的目录（依赖 / 构建 / 版本库 / 应用内部目录）。
var wikiSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true, "out": true,
	"target": true, "vendor": true, ".next": true, ".nuxt": true, "coverage": true,
	"__pycache__": true, ".idea": true, ".vscode": true, ".workbaby": true,
	"bin": true, "obj": true,
}

// wikiMaxFileLines 单文件统计行数上限（超大文件不逐行读摘要）。
const wikiMaxFileLines = 4000

// wikiMaxContentSize 文件页正文上限（字节）。
const wikiMaxContentSize = 512 * 1024

// wikiMaxTreeNodes 树节点总量上限（巨型仓库防列表爆炸）。
const wikiMaxTreeNodes = 4000

// WikiService 仓库导读。
type WikiService struct {
	// root 会话工作区根（api 层注入）。
	root func() string
}

// NewWikiService 构造。
func NewWikiService(root func() string) *WikiService {
	return &WikiService{root: root}
}

// langOf 扩展名 → 语言显示名（未识别留空 = 忽略项）。
func langOf(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".vue":
		return "Vue"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".md":
		return "Markdown"
	case ".json":
		return "JSON"
	case ".yaml", ".yml":
		return "YAML"
	case ".toml":
		return "TOML"
	case ".sql":
		return "SQL"
	case ".css", ".scss", ".less":
		return "CSS"
	case ".html":
		return "HTML"
	case ".sh", ".ps1", ".bat", ".cmd":
		return "Shell"
	}
	return ""
}

// isTextFile 是否按文本处理（无扩展名二进制跳过正文）。
func isTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if langOf(ext) != "" {
		return true
	}
	switch ext {
	case ".txt", ".log", ".ini", ".cfg", ".conf", ".env", ".gitignore", ".mod", ".sum":
		return true
	}
	return ext == ""
}

// Overview 全仓扫描：语言统计 + 入口 + 目录树。
func (s *WikiService) Overview() (*domain.WikiOverviewRESP, error) {
	root := s.root()
	if root == "" {
		return nil, domain.ErrWikiNotWorkspace
	}
	if _, err := os.Stat(root); err != nil {
		return nil, domain.ErrWikiNotWorkspace
	}
	resp := &domain.WikiOverviewRESP{Root: root, Name: filepath.Base(root)}
	langFiles := map[string]int{}
	langLines := map[string]int{}

	var walk func(dir string, node *domain.WikiNodeRESP, depth int) error
	walk = func(dir string, node *domain.WikiNodeRESP, depth int) error {
		if resp.TotalFiles > wikiMaxTreeNodes {
			return nil
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil // 单目录失败不阻断整体
		}
		sort.Slice(ents, func(i, j int) bool {
			if (ents[i].IsDir()) != (ents[j].IsDir()) {
				return ents[i].IsDir()
			}
			return ents[i].Name() < ents[j].Name()
		})
		for _, e := range ents {
			if e.IsDir() {
				if wikiSkipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				child := &domain.WikiNodeRESP{Name: e.Name(), Path: relPath(root, dir, e.Name()), Type: "dir"}
				if depth < 4 {
					if err := walk(filepath.Join(dir, e.Name()), child, depth+1); err != nil {
						return err
					}
				}
				for _, c := range child.Children {
					child.Lines += c.Lines
				}
				node.Children = append(node.Children, child)
				resp.TotalFiles++ // 目录占位计数（与文件一起控总量）
				continue
			}
			lang := langOf(filepath.Ext(e.Name()))
			if lang == "" && !isTextFile(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() > 2*1024*1024 {
				continue
			}
			bs, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			lines := strings.Count(string(bs), "\n") + 1
			rel := relPath(root, dir, e.Name())
			sum := fileSummary(e.Name(), string(bs))
			node.Children = append(node.Children, &domain.WikiNodeRESP{
				Name: e.Name(), Path: rel, Type: "file", Lines: lines, Lang: lang, Summary: sum,
			})
			resp.TotalFiles++
			resp.TotalLines += lines
			if lang != "" {
				langFiles[lang]++
				langLines[lang] += lines
			}
		}
		return nil
	}
	tree := &domain.WikiNodeRESP{Name: resp.Name, Path: "", Type: "dir"}
	if err := walk(root, tree, 0); err != nil {
		return nil, pkg.Wrap(domain.ErrWikiNotWorkspace.Code, "wiki scan", err)
	}
	resp.Tree = tree.Children

	for lang, files := range langFiles {
		resp.Langs = append(resp.Langs, domain.WikiLangRESP{Lang: lang, Files: files, Lines: langLines[lang]})
	}
	sort.Slice(resp.Langs, func(i, j int) bool { return resp.Langs[i].Lines > resp.Langs[j].Lines })

	resp.Entries = s.entries(root)
	return resp, nil
}

// entries 入口文件：README / 构建清单 / 主程序文件（存在即收录，各自带摘要）。
func (s *WikiService) entries(root string) []domain.WikiFileSummaryRESP {
	priority := []string{
		"README.md", "readme.md", "CLAUDE.md", "AGENTS.md",
		"go.mod", "package.json", "Cargo.toml", "pom.xml", "pyproject.toml",
	}
	out := make([]domain.WikiFileSummaryRESP, 0, 8)
	for _, name := range priority {
		bs, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		out = append(out, domain.WikiFileSummaryRESP{
			Path:    name,
			Lang:    langOf(filepath.Ext(name)),
			Lines:   strings.Count(string(bs), "\n") + 1,
			Summary: fileSummary(name, string(bs)),
		})
	}
	// 主程序：Go main.go / Python main.py 等
	for _, name := range []string{"main.go", "main.py", "main.rs", "index.ts", "app.ts"} {
		bs, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		out = append(out, domain.WikiFileSummaryRESP{
			Path: name, Lang: langOf(filepath.Ext(name)),
			Lines: strings.Count(string(bs), "\n") + 1, Summary: fileSummary(name, string(bs)),
		})
	}
	return out
}

// Page 目录页或文件页。
func (s *WikiService) Page(reqPath string) (*domain.WikiPageRESP, error) {
	root := s.root()
	if root == "" {
		return nil, domain.ErrWikiNotWorkspace
	}
	clean := filepath.Clean(strings.ReplaceAll(reqPath, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "..") {
		return nil, domain.ErrWikiPageNotFound
	}
	full := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Stat(full)
	if err != nil {
		return nil, domain.ErrWikiPageNotFound
	}
	page := &domain.WikiPageRESP{Path: filepath.ToSlash(clean)}
	if info.IsDir() {
		page.Type = "dir"
		ents, err := os.ReadDir(full)
		if err != nil {
			return nil, domain.ErrWikiPageNotFound
		}
		sort.Slice(ents, func(i, j int) bool {
			if (ents[i].IsDir()) != (ents[j].IsDir()) {
				return ents[i].IsDir()
			}
			return ents[i].Name() < ents[j].Name()
		})
		for _, e := range ents {
			if wikiSkipDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			item := domain.WikiFileSummaryRESP{Path: page.Path + "/" + e.Name()}
			if e.IsDir() {
				item.Summary = "（目录）"
				page.Children = append(page.Children, item)
				continue
			}
			lang := langOf(filepath.Ext(e.Name()))
			if lang == "" && !isTextFile(e.Name()) {
				continue
			}
			bs, err := os.ReadFile(filepath.Join(full, e.Name()))
			if err != nil {
				continue
			}
			item.Lang = lang
			item.Lines = strings.Count(string(bs), "\n") + 1
			item.Summary = fileSummary(e.Name(), string(bs))
			page.Children = append(page.Children, item)
		}
		return page, nil
	}
	page.Type = "file"
	page.Lang = langOf(filepath.Ext(full))
	bs, err := os.ReadFile(full)
	if err != nil {
		return nil, domain.ErrWikiPageNotFound
	}
	page.Lines = strings.Count(string(bs), "\n") + 1
	page.Summary = fileSummary(filepath.Base(full), string(bs))
	if len(bs) > wikiMaxContentSize {
		page.Content = string(bs[:wikiMaxContentSize])
		page.Truncated = true
	} else {
		page.Content = string(bs)
	}
	return page, nil
}

// fileSummary 一句话摘要：首部连续注释行 > package/imports 后的首个声明行 > 首个非空行。
func fileSummary(name, content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) > wikiMaxFileLines {
		lines = lines[:wikiMaxFileLines]
	}
	var comment []string
	inHeaderComment := true
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if inHeaderComment {
			switch {
			case strings.HasPrefix(t, "//"):
				comment = append(comment, strings.TrimPrefix(t, "//"))
				continue
			case strings.HasPrefix(t, "/*"):
				t = strings.TrimPrefix(t, "/*")
				t = strings.TrimSuffix(t, "*/")
				comment = append(comment, strings.TrimSpace(t))
				continue
			case strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!"):
				comment = append(comment, strings.TrimPrefix(t, "#"))
				continue
			}
		}
		inHeaderComment = false
		if len(comment) > 0 {
			break
		}
		if strings.HasPrefix(t, "package ") || strings.HasPrefix(t, "import") ||
			strings.HasPrefix(t, "use ") || strings.HasPrefix(t, "using ") {
			continue
		}
		comment = append(comment, t)
		break
	}
	sum := strings.TrimSpace(strings.Join(comment, " "))
	if len([]rune(sum)) > 160 {
		sum = string([]rune(sum)[:160]) + "…"
	}
	if sum == "" {
		sum = name
	}
	return sum
}

// relPath 相对路径（统一斜杠）。
func relPath(root, dir, name string) string {
	rel, err := filepath.Rel(root, filepath.Join(dir, name))
	if err != nil {
		return name
	}
	return filepath.ToSlash(rel)
}
