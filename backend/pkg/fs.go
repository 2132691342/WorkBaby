// 文件与目录的最小工具集：存在性、读写、复制、扩展名与临时文件名。
package pkg

import (
	"os"
	"path/filepath"
	"strings"
)

// FileExists 判断路径存在且是文件。
func FileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// DirExists 判断路径存在且是目录。
func DirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// WriteText 原子写文本文件并自动创建父目录：先写同目录临时文件再 rename，
// 磁盘写满或进程中途退出时原文件保持完整（助手写的是用户的真实工程文件）。
func WriteText(p, content string) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Wrap("1006", "创建目录失败", err)
	}
	tmp, err := os.CreateTemp(dir, ".wb-write-*")
	if err != nil {
		return Wrap("1007", "写入文件失败", err)
	}
	tmpName := tmp.Name()
	discard := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		discard()
		return Wrap("1007", "写入文件失败", err)
	}
	if err := tmp.Close(); err != nil {
		discard()
		return Wrap("1007", "写入文件失败", err)
	}
	_ = os.Chmod(tmpName, 0o644)
	if err := os.Rename(tmpName, p); err != nil {
		discard()
		return Wrap("1007", "写入文件失败", err)
	}
	return nil
}

// CopyFile 复制文件到 dst（自动建父目录）。背景图与导入的技能都要留自己的副本：
// 直接引用用户选中的原文件，原文件一删一挪引用就断。
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return Wrap("1005", "读取文件失败", err)
	}
	defer in.Close()
	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return Wrap("1007", "写入文件失败", err)
	}
	if _, err := out.ReadFrom(in); err != nil {
		_ = out.Close()
		return Wrap("1007", "写入文件失败", err)
	}
	if err := out.Close(); err != nil {
		return Wrap("1007", "写入文件失败", err)
	}
	return nil
}

// EnsureDir 保证目录存在。
func EnsureDir(p string) error {
	if err := os.MkdirAll(p, 0o755); err != nil {
		return Wrap("1006", "创建目录失败", err)
	}
	return nil
}

// Ext 取小写扩展名（不含点）；无扩展名时返回空串。
func Ext(p string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
}
