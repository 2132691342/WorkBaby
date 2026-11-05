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
	// fileWriteLimit 单次写入上限（5MB）
	fileWriteLimit = 5 << 20
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

// guard 归一路径并保证落在工作区内；越界直接拒。
// 相对路径（前端的斜杠约定）按工作区根拼，绝不拿进程 CWD 解析——那会偏到工作区外。
// 所有接受外部路径的接口都必须先过它，否则一个 ../../ 就能摸到任意文件。
func (f *Files) guard(path string) (string, error) {
	root := f.Settings.Workspace()
	if root == "" {
		return "", pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	target := strings.TrimSpace(path)
	if target == "" {
		target = root
	} else if !filepath.IsAbs(target) {
		// Join 自带 Clean：「src/../../etc」会被折成能被 Inside 拒掉的形状。
		target = filepath.Join(root, filepath.FromSlash(target))
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

// Save 保存文本文件：父目录不存在就一并建出；同名目录不能当文件写。
func (f *Files) Save(req domain.FileSaveREQ) error {
	target, err := f.guard(req.FilePath)
	if err != nil {
		return err
	}
	if pkg.IsDir(target) {
		return pkg.Err(pkg.CodeFileIO, "目标是一个目录，不能当文件保存", target)
	}
	if len(req.FileText) > fileWriteLimit {
		return pkg.Err(pkg.CodeFileIO, "内容超过 5MB 写入上限")
	}
	if err := pkg.EnsureDir(filepath.Dir(target)); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "父目录创建失败", err)
	}
	if err := os.WriteFile(target, []byte(req.FileText), 0o644); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "文件写入失败", err)
	}
	return nil
}

// Mkdir 新建目录；多级路径一并建出，已存在即拒。
func (f *Files) Mkdir(path string) error {
	target, err := f.guard(path)
	if err != nil {
		return err
	}
	if pkg.Exists(target) {
		return pkg.Err(pkg.CodeFileIO, "目标已存在", target)
	}
	if err := pkg.EnsureDir(target); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "目录创建失败", err)
	}
	return nil
}

// Delete 删除文件或目录（递归）；工作区根本身删不掉。
func (f *Files) Delete(path string) error {
	target, err := f.guard(path)
	if err != nil {
		return err
	}
	if target == f.Settings.Workspace() {
		return pkg.Err(pkg.CodeFileIO, "工作区根目录不能删除")
	}
	if !pkg.Exists(target) {
		return pkg.Err(pkg.CodeFileNotFound, "目标不存在", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "删除失败", err)
	}
	return nil
}

// Rename 改名或移动：源必须存在、目标必须不存在，两端都落在工作区内。
func (f *Files) Rename(req domain.FileRenameREQ) error {
	target, err := f.guard(req.FilePath)
	if err != nil {
		return err
	}
	dest, err := f.guard(req.NewFilePath)
	if err != nil {
		return err
	}
	if !pkg.Exists(target) {
		return pkg.Err(pkg.CodeFileNotFound, "源不存在", target)
	}
	if pkg.Exists(dest) {
		return pkg.Err(pkg.CodeFileIO, "目标已存在", dest)
	}
	if err := os.Rename(target, dest); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "改名失败", err)
	}
	return nil
}
