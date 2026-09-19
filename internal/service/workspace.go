package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// hiddenWorkspacePrefixes 工作区文件面板隐藏的内部目录。
var hiddenWorkspacePrefixes = []string{"local/", ".workbaby/", ".index/"}

// WorkspaceService 会话工作区文件面板：列目录 + 读取（沙箱）+ 内容类型推断。
//
// 面板根与工具链共用一套解析：会话绑定了外部目录就展示外部目录，
// 否则展示默认工作区（workspacesRoot/{sessionID}）——否则「选了目录
// 面板却是空的、Agent 却在外部目录干活」这种精神分裂会直接劝退用户。
type WorkspaceService struct {
	workspacesRoot string              // {home}/workspaces
	resolve        func(string) string // 会话 → 工作区根；nil 或返回空 = 默认根
}

// NewWorkspaceService 构造；resolve 可空（全部走默认根）。
func NewWorkspaceService(workspacesRoot string, resolve func(sessionID string) string) *WorkspaceService {
	return &WorkspaceService{workspacesRoot: workspacesRoot, resolve: resolve}
}

// Dir 返回某会话的工作区根目录。
func (s *WorkspaceService) Dir(sessionID string) string {
	if s.resolve != nil {
		if p := s.resolve(sessionID); p != "" {
			return p
		}
	}
	return filepath.Join(s.workspacesRoot, sessionID)
}

// ListFiles 递归列出工作区全部文件（相对路径排序；过滤内部隐藏目录）。
func (s *WorkspaceService) ListFiles(_ context.Context, sessionID string) ([]domain.WorkspaceFileItem, error) {
	root := s.Dir(sessionID)
	if _, err := os.Stat(root); err != nil {
		return []domain.WorkspaceFileItem{}, nil // 目录不存在 = 空工作区
	}
	var out []domain.WorkspaceFileItem
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 单项失败跳过，不阻断整个面板
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isHiddenWorkspacePath(rel) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		out = append(out, domain.WorkspaceFileItem{
			Path:       rel,
			Name:       d.Name(),
			Ext:        extWithDot(d.Name()),
			Kind:       workspaceKind(d.Name()),
			Size:       info.Size(),
			URL:        "/files/workspace/" + sessionID + "?path=" + rel,
			ModifiedAt: info.ModTime().UnixMilli(),
		})
		return nil
	})
	if err != nil {
		return nil, pkg.Wrap(1210, "list workspace files failed", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// ReadFile 读取工作区单文件内容（路径沙箱防穿越）。
func (s *WorkspaceService) ReadFile(_ context.Context, sessionID, path string) ([]byte, string, error) {
	root := s.Dir(sessionID)
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := pkg.Within(root, full); err != nil {
		return nil, "", err
	}
	bs, err := os.ReadFile(full)
	if err != nil {
		return nil, "", pkg.Wrap(1211, "read workspace file failed", err)
	}
	return bs, workspaceContentType(path), nil
}

// listDirIgnore 工作区文件树默认隐藏的目录：VCS 元数据 / 依赖 / 构建产物是噪音，
// 且 node_modules 之类动辄数千项。隐藏不等于不可访问——用户在输入框里仍可让 agent 读。
var listDirIgnore = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, ".workbaby": true,
	"node_modules": true, "__pycache__": true, ".venv": true, "venv": true,
	"dist": true, "target": true, ".next": true, ".nuxt": true, ".gradle": true,
}

// dirListCap 单层目录条目上限：超过即截断并置 Truncated，防一次拉回几千项。
const dirListCap = 500

// ListDir 懒加载列工作区单层真实磁盘目录（区别于 ListFiles 的托管产物清单）。
// 路径相对工作区根并做穿越校验；出参 root 为工作区根绝对路径，供前端拼「添加到聊天」的绝对引用。
func (s *WorkspaceService) ListDir(_ context.Context, sessionID, dir string) (domain.WorkspaceListRESP, error) {
	root := s.Dir(sessionID)
	full := root
	if dir = strings.Trim(strings.TrimSpace(dir), "/"); dir != "" {
		full = filepath.Join(root, filepath.FromSlash(dir))
	}
	if err := pkg.Within(root, full); err != nil {
		return domain.WorkspaceListRESP{}, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.WorkspaceListRESP{Items: []domain.WorkspaceEntry{}}, nil
		}
		return domain.WorkspaceListRESP{}, pkg.Wrap(1210, "read workspace dir failed", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di // 文件夹优先
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	out := make([]domain.WorkspaceEntry, 0, len(entries))
	truncated := false
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || (e.IsDir() && listDirIgnore[name]) {
			continue
		}
		if len(out) >= dirListCap {
			truncated = true
			break
		}
		var size int64
		if !e.IsDir() {
			if info, ierr := e.Info(); ierr == nil {
				size = info.Size()
			}
		}
		p := prefix + name
		if e.IsDir() {
			p += "/"
		}
		out = append(out, domain.WorkspaceEntry{
			Path:  p,
			Name:  name,
			IsDir: e.IsDir(),
			Size:  size,
			Ext:   strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), "."),
			Kind:  workspaceKind(name),
		})
	}
	return domain.WorkspaceListRESP{Items: out, Truncated: truncated, Root: root}, nil
}

// resolveEntry 相对路径 → 工作区内绝对路径（Within 沙箱防穿越）。
func (s *WorkspaceService) resolveEntry(sessionID, path string) (string, error) {
	root := s.Dir(sessionID)
	full := filepath.Join(root, filepath.FromSlash(strings.Trim(strings.TrimSpace(path), "/")))
	if err := pkg.Within(root, full); err != nil {
		return "", err
	}
	return full, nil
}

// RenameEntry 工作区内同级重命名（文件/目录均可；不做跨目录移动，避免扩大沙箱语义）。
func (s *WorkspaceService) RenameEntry(_ context.Context, sessionID, path, newName string) error {
	if newName == "" || newName == "." || newName == ".." || strings.ContainsAny(newName, `/\`) {
		return pkg.New(1212, "invalid new name", newName)
	}
	full, err := s.resolveEntry(sessionID, path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(full); err != nil {
		return pkg.Wrap(1213, "workspace entry not found", err)
	}
	dst := filepath.Join(filepath.Dir(full), newName)
	if _, err := os.Stat(dst); err == nil {
		return pkg.New(1214, "target name already exists", newName)
	}
	if err := os.Rename(full, dst); err != nil {
		return pkg.Wrap(1215, "rename workspace entry failed", err)
	}
	return nil
}

// CopyEntry 复制文件为同级副本（"name (1).ext" 就近递增）；返回新相对路径。
// 目录不支持副本（克制：目录级操作交给 Agent 工具），NotDir 时明确报错。
func (s *WorkspaceService) CopyEntry(_ context.Context, sessionID, path string) (string, error) {
	full, err := s.resolveEntry(sessionID, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", pkg.Wrap(1213, "workspace entry not found", err)
	}
	if info.IsDir() {
		return "", pkg.New(1216, "copy directory is not supported", path)
	}
	dst := full
	n := 1
	for {
		dst = filepath.Join(filepath.Dir(full), copyName(info.Name(), n))
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		n++
	}
	src, err := os.Open(full)
	if err != nil {
		return "", pkg.Wrap(1217, "open workspace file failed", err)
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", pkg.Wrap(1217, "create workspace copy failed", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return "", pkg.Wrap(1217, "copy workspace file failed", err)
	}
	rel, err := filepath.Rel(s.Dir(sessionID), dst)
	if err != nil {
		return "", pkg.Wrap(1217, "resolve copy path failed", err)
	}
	return filepath.ToSlash(rel), nil
}

// copyName 生成第 n 份副本文件名："a.txt" → "a (1).txt"。
func copyName(name string, n int) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (%d)%s", base, n, ext)
}

// DeleteEntry 删除文件/目录（目录递归；禁止删除工作区根本身）。
func (s *WorkspaceService) DeleteEntry(_ context.Context, sessionID, path string) error {
	root := s.Dir(sessionID)
	full, err := s.resolveEntry(sessionID, path)
	if err != nil {
		return err
	}
	if filepath.Clean(full) == filepath.Clean(root) {
		return pkg.New(1218, "cannot remove workspace root", path)
	}
	if _, err := os.Stat(full); err != nil {
		return pkg.Wrap(1213, "workspace entry not found", err)
	}
	if err := os.RemoveAll(full); err != nil {
		return pkg.Wrap(1219, "remove workspace entry failed", err)
	}
	return nil
}

// workspaceKind 按扩展名分类（对齐前端 WorkspaceFile.kind）。
func workspaceKind(name string) string {
	switch extWithDot(name) {
	case ".html", ".htm":
		return "html"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".pdf":
		return "pdf"
	case ".md", ".txt", ".json", ".csv", ".yaml", ".yml", ".xml",
		".js", ".ts", ".java", ".py", ".go", ".rs", ".css", ".log":
		return "text"
	case ".mp4", ".mov", ".webm":
		return "video"
	default:
		return "file"
	}
}

// workspaceContentType 按扩展名推断 Content-Type（预览/内联渲染）。
func workspaceContentType(path string) string {
	switch extWithDot(path) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".md", ".txt", ".json", ".csv", ".yaml", ".yml", ".xml",
		".js", ".ts", ".java", ".py", ".go", ".rs", ".css", ".log":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// extWithDot 返回含点小写扩展名（如 ".html"）。
func extWithDot(name string) string {
	return strings.ToLower(filepath.Ext(name))
}

// isHiddenWorkspacePath 过滤内部隐藏目录前缀。
func isHiddenWorkspacePath(rel string) bool {
	for _, p := range hiddenWorkspacePrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}
