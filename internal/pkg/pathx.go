// 文件：路径工具——清洗 + 越界校验 + 临时目录。

package pkg

import (
	"os"
	"path/filepath"
	"strings"
)

// Clean 返回 path 的 Clean 结果；空串报 AppError。
func Clean(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", New(1001, "path is empty", "")
	}
	return filepath.Clean(path), nil
}

// Within 校验子路径是否在 root 内（防穿越）；root 与 path 都按绝对路径清洗后再判断。
func Within(root, sub string) error {
	r, err := filepath.Abs(root)
	if err != nil {
		return Wrap(1002, "abs root failed", err)
	}
	p, err := filepath.Abs(sub)
	if err != nil {
		return Wrap(1002, "abs sub failed", err)
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return Wrap(1003, "path escapes root", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return New(1003, "path escapes root", p)
	}
	return nil
}

// EnsureDir 创建目录（递归），已存在则 no-op。
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Wrap(1013, "mkdir failed", err)
	}
	return nil
}
