package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 文件接口的边界：只看工作区内、只读文本、列表有上限。
const (
	// fileListLimit 单次列目录的上限
	fileListLimit = 500
	// fileSearchLimit 搜索命中上限
	fileSearchLimit = 200
	// fileReadLimit 单次读取上限（1MB）
	fileReadLimit = 1 << 20
)

// Files 是工作区文件浏览：列目录、按名搜索、读文本。
type Files struct {
	// Settings 设置（工作区根）
	Settings *Settings
}

// NewFiles 造一个文件门面。
func NewFiles(settings *Settings) *Files {
	return &Files{Settings: settings}
}

// guard 保证路径落在工作区内；越界直接拒。
// 所有接受外部路径的读接口都必须先过它，否则前端一个 ../../../ 就能读到任意文件。
func (f *Files) guard(path string) (string, error) {
	root := f.Settings.Workspace()
	if root == "" {
		return "", pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	target := path
	if target == "" {
		target = root
	}
	if !pkg.Inside(root, target) {
		return "", pkg.Err(pkg.CodePathOutside, "路径不在工作区内", target)
	}
	return target, nil
}

// List 列一个目录：目录优先，条目数有上限。
func (f *Files) List(path string) (domain.FileListRESP, error) {
	dir, err := f.guard(path)
	if err != nil {
		return domain.FileListRESP{}, err
	}
	if !pkg.IsDir(dir) {
		return domain.FileListRESP{}, pkg.Err(pkg.CodeFileNotFound, "不是一个目录", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return domain.FileListRESP{}, pkg.WrapErr(pkg.CodeFileIO, "目录读取失败", err)
	}
	rows := make([]domain.FileEntryVO, 0, len(entries))
	for _, entry := range entries {
		row := domain.FileEntryVO{EntryName: entry.Name(), EntryIsDir: entry.IsDir()}
		if info, err := entry.Info(); err == nil {
			if !entry.IsDir() {
				row.SizeBytes = info.Size()
			}
			row.ModifiedAt = info.ModTime().UnixMilli()
		}
		rows = append(rows, row)
	}
	// 目录优先，其次按名字（大小写不敏感），这是资源管理器的习惯。
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].EntryIsDir != rows[j].EntryIsDir {
			return rows[i].EntryIsDir
		}
		return strings.ToLower(rows[i].EntryName) < strings.ToLower(rows[j].EntryName)
	})
	out := domain.FileListRESP{DirPath: dir, Entries: rows}
	if len(rows) > fileListLimit {
		out.Entries = rows[:fileListLimit]
		out.Truncated = true
	}
	return out, nil
}

// Search 按文件名递归搜索；跳过 node_modules / .git / dist 这些噪音目录。
func (f *Files) Search(query string, limit int) (domain.FileSearchRESP, error) {
	root := f.Settings.Workspace()
	if root == "" {
		return domain.FileSearchRESP{}, pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	out := domain.FileSearchRESP{SearchQuery: query}
	if needle == "" {
		return out, nil
	}
	if limit <= 0 || limit > fileSearchLimit {
		limit = fileSearchLimit
	}
	hits := make([]domain.FileHitVO, 0, limit)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// 权限不足的目录跳过：搜索不该因为一个系统目录整体失败。
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root && isNoiseDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(strings.ToLower(entry.Name()), needle) {
			return nil
		}
		hits = append(hits, domain.FileHitVO{FilePath: path, EntryName: entry.Name()})
		if len(hits) >= limit {
			out.Truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return out, pkg.WrapErr(pkg.CodeFileIO, "搜索失败", walkErr)
	}
	out.Hits = hits
	return out, nil
}

// isNoiseDir 判断搜索要跳过的目录。
func isNoiseDir(name string) bool {
	switch name {
	case "node_modules", ".git", "dist", "build", ".next", "target", "__pycache__", ".venv":
		return true
	}
	return false
}

// Read 读一个文本文件；二进制给占位说明而不是一屏乱码。
func (f *Files) Read(path string) (domain.FileReadRESP, error) {
	target, err := f.guard(path)
	if err != nil {
		return domain.FileReadRESP{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return domain.FileReadRESP{}, pkg.WrapErr(pkg.CodeFileNotFound, "文件不存在", err)
	}
	if info.IsDir() {
		return domain.FileReadRESP{}, pkg.Err(pkg.CodeFileNotFound, "这是一个目录", target)
	}
	out := domain.FileReadRESP{FilePath: target, SizeBytes: info.Size()}
	raw, err := os.ReadFile(target)
	if err != nil {
		return domain.FileReadRESP{}, pkg.WrapErr(pkg.CodeFileIO, "文件读取失败", err)
	}
	if len(raw) > fileReadLimit {
		raw = raw[:fileReadLimit]
		out.Truncated = true
	}
	// NUL 字节是「这是二进制」最可靠的判据，比猜扩展名准。
	if isBinary(raw) {
		out.IsBinary = true
		out.FileText = "（二进制文件，不在界面里预览）"
		return out, nil
	}
	out.FileText = string(raw)
	return out, nil
}

// isBinary 用前若干字节判断是否二进制。
func isBinary(raw []byte) bool {
	probe := raw
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if !utf8.Valid(probe) {
		return true
	}
	return strings.ContainsRune(string(probe), 0)
}
