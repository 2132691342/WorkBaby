// Package index 是大仓库的预索引与模糊检索。
// 它的存在理由：一次几千行的 grep 在大仓库里要几秒，
// 而模型每轮都要搜好几次——把「找文件」这件事预先算好，模型就只等结果。
package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DocKind 是索引文档的种类。
type DocKind string

const (
	// KindFile 一个文件
	KindFile DocKind = "file"
	// KindSymbol 一个符号
	KindSymbol DocKind = "symbol"
)

// Doc 是索引里的一条文档。
type Doc struct {
	// DocID 文档 ID
	DocID string `json:"doc_id"`
	// DocPath 相对工作区的路径
	DocPath string `json:"doc_path"`
	// DocKind 种类
	DocKind DocKind `json:"doc_kind"`
	// DocSymbol 符号名（文件类为空）
	DocSymbol string `json:"doc_symbol,omitempty"`
	// DocSymbolKind 符号种类
	DocSymbolKind string `json:"doc_symbol_kind,omitempty"`
	// DocLine 符号所在行号
	DocLine int `json:"doc_line,omitempty"`
	// DocBody 可检索正文
	DocBody string `json:"doc_body"`
	// DocSize 字节数
	DocSize int64 `json:"doc_size"`
	// DocMtimeMs 修改时刻（毫秒）
	DocMtimeMs int64 `json:"doc_mtime_ms"`
}

// 索引参数。这些是「什么值得进索引」的判断，不是用户可调的旋钮。
const (
	// MaxIndexedFileBytes 单个文件进索引的体积上限
	MaxIndexedFileBytes = 512 << 10
	// MaxBodyChars 每个文件留进索引的正文长度
	MaxBodyChars = 8000
	// MaxDocs 单次索引产出的文档上限
	MaxDocs = 20000
)

// skipDirs 是索引时跳过的目录。
var skipDirs = map[string]struct{}{
	"node_modules": {}, ".git": {}, "dist": {}, "build": {}, "out": {},
	".next": {}, ".nuxt": {}, "target": {}, "vendor": {}, "__pycache__": {},
	".venv": {}, "venv": {}, ".idea": {}, ".vscode": {}, "coverage": {},
	".cache": {}, ".turbo": {}, ".svelte-kit": {},
}

// binaryExts 是按二进制处理的扩展名。
var binaryExts = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".ico": {},
	".pdf": {}, ".zip": {}, ".gz": {}, ".tar": {}, ".7z": {}, ".rar": {},
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {}, ".class": {}, ".jar": {},
	".mp3": {}, ".mp4": {}, ".avi": {}, ".mov": {}, ".wav": {}, ".woff": {},
	".woff2": {}, ".ttf": {}, ".eot": {}, ".bin": {}, ".wasm": {},
}

// Build 遍历一个工作区，产出可检索的文档集。
// 它一次遍历同时收文件与符号：分开遍历等于把目录读两遍。
func Build(root string) ([]Doc, error) {
	docs := make([]Doc, 0, 512)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// 读不了的目录跳过：权限问题不该让整次索引失败。
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(docs) >= MaxDocs {
			return filepath.SkipAll
		}
		if entry.IsDir() {
			if path != root {
				if _, skip := skipDirs[strings.ToLower(entry.Name())]; skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isIndexable(entry.Name()) {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Size() > MaxIndexedFileBytes {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil || isBinary(raw) {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		body := string(raw)
		docs = append(docs, Doc{
			DocID:      docID(relative, ""),
			DocPath:    relative,
			DocKind:    KindFile,
			DocBody:    truncateBody(body),
			DocSize:    info.Size(),
			DocMtimeMs: info.ModTime().UnixMilli(),
		})
		for _, symbol := range extractSymbols(relative, body) {
			docs = append(docs, symbol)
		}
		return nil
	})
	if err != nil {
		return docs, err
	}
	return docs, nil
}

// docID 造文档 ID：路径加符号名，唯一且稳定。
func docID(path, symbol string) string {
	if symbol == "" {
		return "F:" + path
	}
	return "S:" + path + "::" + symbol
}

// isIndexable 判断文件该不该进索引。
func isIndexable(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if _, binary := binaryExts[ext]; binary {
		return false
	}
	// 没有扩展名的文件多半是配置或锁文件，噪声多。
	return ext != ""
}

// isBinary 判断内容是不是二进制。
func isBinary(raw []byte) bool {
	limit := min(len(raw), 8000)
	for index := range limit {
		if raw[index] == 0 {
			return true
		}
	}
	return false
}

// truncateBody 截断正文到上限。
func truncateBody(body string) string {
	runes := []rune(body)
	if len(runes) <= MaxBodyChars {
		return body
	}
	return string(runes[:MaxBodyChars])
}

// symbolPattern 抓取常见的声明式符号。
// 各语言写法不同，但「名字 + 括号/等号 + 类型」这个形状是共通的。
var symbolPatterns = []struct {
	// kind 符号种类
	kind string
	// pattern 匹配规则
	pattern *regexp.Regexp
}{
	{"func", regexp.MustCompile(`(?m)^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)},
	{"type", regexp.MustCompile(`(?m)^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"class", regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"interface", regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"struct", regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:type\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"fn", regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"def", regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"function", regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	{"const", regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)`)},
}

// extractSymbols 从文件正文里抽出符号。
func extractSymbols(path, body string) []Doc {
	out := make([]Doc, 0, 8)
	seen := map[string]struct{}{}
	for _, entry := range symbolPatterns {
		matches := entry.pattern.FindAllStringSubmatch(body, 40)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			name := match[1]
			if name == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, Doc{
				DocID:         docID(path, name),
				DocPath:       path,
				DocKind:       KindSymbol,
				DocSymbol:     name,
				DocSymbolKind: entry.kind,
				DocLine:       lineOf(body, match[0]),
				DocBody:       "",
			})
		}
	}
	return out
}

// lineOf 找一段文本在原文里的行号（从 1 数）。
func lineOf(body, needle string) int {
	index := strings.Index(body, needle)
	if index < 0 {
		return 0
	}
	return strings.Count(body[:index], "\n") + 1
}

// 索引时间戳的辅助：让上层能判断索引是否过期。
// ModifiedAt 取一个目录里最新的修改时刻。
func ModifiedAt(root string) int64 {
	var latest int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		if stamp := info.ModTime().UnixMilli(); stamp > latest {
			latest = stamp
		}
		return nil
	})
	return latest
}
