// 服务层内部共用件：文件树复制与路径比较。
package service

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// copyTree 递归复制目录（符号链接按普通文件处理，扩展目录里不该出现链接）。
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return pkg.CopyFile(src, dst)
	}
	if err := pkg.EnsureDir(dst); err != nil {
		return err
	}
	children, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := copyTree(filepath.Join(src, child.Name()), filepath.Join(dst, child.Name())); err != nil {
			return err
		}
	}
	return nil
}
