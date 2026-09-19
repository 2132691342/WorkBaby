// Package archive 提供 zip/unzip 工具（archive_manager）。
//
// 路径强制限定 workspace 根内；解压做 zip-slip 防护 + 限额（复用 runtime 安全解压思路）。
package archive

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

const (
	maxFiles    = 2000      // 单次解压文件数上限
	maxBytes    = 200 << 20 // 单次解压总字节上限
	maxEntryKB  = 100       // 单文件解压大小上限（KB），超出报错
	zipEntryCap = 64 << 20  // zip 条目读取上限（zip 文件内单个 entry 解压上限）
)

// ArchiveTool zip 压缩 / 解压（workspace 相对路径）。
type ArchiveTool struct {
	resolve func(context.Context) string
}

// New 构造；resolver 按会话解析 workspace 根，defRoot 为回落根。
func New(resolver tool.RootResolver, defRoot string) *ArchiveTool {
	return &ArchiveTool{resolve: tool.ResolveRoot(resolver, defRoot)}
}

func (t *ArchiveTool) Name() string              { return "archive_manager" }
func (t *ArchiveTool) RiskLevel() tool.RiskLevel { return tool.RiskWriteLocal }
func (t *ArchiveTool) Description() string {
	return "Compress a file/directory to zip, or extract a zip into a directory (workspace-relative paths)."
}

func (t *ArchiveTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["operation", "source", "target"],
			"properties": {
				"operation": {"type": "string", "enum": ["zip", "unzip"], "description": "zip / unzip"},
				"source": {"type": "string", "description": "source path relative to the workspace"},
				"target": {"type": "string", "description": "target path relative to the workspace"}
			}
		}`),
	}
}

type archiveReq struct {
	Operation string `json:"operation"`
	Source    string `json:"source"`
	Target    string `json:"target"`
}

// Execute 执行 zip / unzip。
func (t *ArchiveTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req archiveReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "archive_manager args parse failed", err)}
	}
	root := t.resolve(ctx)
	src, err := t.safePath(root, req.Source)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	dst, err := t.safePath(root, req.Target)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	switch req.Operation {
	case "zip":
		if err := zipDir(src, dst); err != nil {
			return tool.ToolResult{Err: pkg.Wrap(4006, "zip failed", err)}
		}
		return tool.ToolResult{Content: "zipped: " + filepath.Base(dst)}
	case "unzip":
		if err := unzipTo(src, dst); err != nil {
			return tool.ToolResult{Err: pkg.Wrap(4006, "unzip failed", err)}
		}
		return tool.ToolResult{Content: "unzipped to: " + filepath.Base(dst)}
	default:
		return tool.ToolResult{Err: pkg.New(4004, "unknown operation (zip/unzip)", req.Operation)}
	}
}

// safePath 校验在 workspace 根内并绝对化。
func (t *ArchiveTool) safePath(workspaceRoot, path string) (string, error) {
	if path == "" {
		return "", tool.ErrPathEscape
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", pkg.Wrap(4007, "abs root failed", err)
	}
	candidate := path
	if !filepath.IsAbs(path) {
		candidate = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", pkg.Wrap(4007, "abs path failed", err)
	}
	if err := pkg.Within(root, abs); err != nil {
		return "", tool.ErrPathEscape
	}
	return abs, nil
}

// zipDir 把目录/文件压缩为 zip（跳过 .git / 隐藏文件避免打包内部状态）。
func zipDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != src {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			return addZipEntry(zw, rel, path, info)
		})
	}
	// 单文件打包：直接作为 zip 根下的文件
	return addZipEntry(zw, filepath.Base(src), src, fi)
}

func addZipEntry(zw *zip.Writer, name, path string, info os.FileInfo) error {
	if info.IsDir() {
		_, err := zw.Create(name + "/")
		return err
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// unzipTo 解压 zip 到目标目录（zip-slip 防护 + 限额）。
func unzipTo(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	count, total := 0, int64(0)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		count++
		if count > maxFiles {
			return fmt.Errorf("archive exceeds maxFiles %d", maxFiles)
		}
		out, err := safeJoin(dst, f.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		n, werr := writeLimited(rc, out, zipEntryCap, maxBytes-total)
		rc.Close()
		if werr != nil {
			return werr
		}
		total += n
	}
	return nil
}

// safeJoin 解压条目名拼到 dst 下并拒绝穿越。
func safeJoin(dst, name string) (string, error) {
	clean := filepath.Clean(filepath.Join(dst, filepath.FromSlash(name)))
	rel, err := filepath.Rel(dst, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal detected: %s", name)
	}
	return clean, nil
}

// writeLimited 写条目并限制字节数。
func writeLimited(rc io.Reader, out string, limit, budget int64) (int64, error) {
	if budget <= 0 {
		return 0, fmt.Errorf("archive exceeds maxBytes")
	}
	limit = min64(limit, budget)
	dst, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer dst.Close()
	n, err := io.Copy(dst, io.LimitReader(rc, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("entry exceeds size limit (%d KB)", maxEntryKB)
	}
	return n, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
