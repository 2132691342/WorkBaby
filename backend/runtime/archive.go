// 归档解压：内置运行时只认 zip——Windows 原生格式，且官方 Python embeddable 与
// PowerShell 都只发 zip。穿越拒绝与体积上限是解压外部归档的唯一安全边界。
package runtime

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// newZipReader 把内存里的 zip 归档打开成逐条目读取器。
func newZipReader(data []byte) (*zip.Reader, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, pkg.Wrap(7001, "运行时压缩包格式不正确", err)
	}
	return reader, nil
}

// extractZip 解压 zip 到目标目录：官方 zip 都是平铺布局（可执行文件在压缩包根），
// 不剥层；体积上限复用 writeEntry 的总字节闸。
func extractZip(reader *zip.Reader, dest string) error {
	if err := pkg.EnsureDir(dest); err != nil {
		return err
	}
	var total int64
	for _, item := range reader.File {
		target, err := safeExtractPath(dest, item.Name)
		if err != nil {
			return err
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return pkg.Wrap(7001, "创建运行时目录失败", err)
			}
			continue
		}
		rc, err := item.Open()
		if err != nil {
			return pkg.Wrap(7001, "读取运行时压缩项失败", err)
		}
		err = writeEntry(target, rc, &total)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// safeExtractPath 拒绝路径穿越：这是解压外部归档的唯一安全边界。
func safeExtractPath(dest, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	// Windows 上 "/x" 不算 IsAbs（缺盘符），但它是带根路径，必须一并拒绝。
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") ||
		strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, `\`) {
		return "", pkg.New(7004, "压缩包里有不安全的路径", name)
	}
	target := filepath.Join(dest, cleaned)
	if !pkg.IsInside(dest, target) {
		return "", pkg.New(7004, "压缩包里有不安全的路径", name)
	}
	return target, nil
}

func writeEntry(target string, r io.Reader, total *int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return pkg.Wrap(7001, "创建运行时目录失败", err)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return pkg.Wrap(7001, "写入运行时文件失败", err)
	}
	defer out.Close()

	n, err := io.Copy(out, io.LimitReader(r, maxTotalBytes-*total))
	*total += n
	if err != nil {
		return pkg.Wrap(7001, "写入运行时文件失败", err)
	}
	if *total >= maxTotalBytes {
		return pkg.New(7001, "运行时压缩包体积超限", "")
	}
	return nil
}
