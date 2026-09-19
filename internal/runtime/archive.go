package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/pkg"
)

// Extract 解压 zip / tar.gz 归档到 targetDir（创建/覆盖）。
// stripComponents 剥离前 N 层路径；路径穿越（zip slip）直接拒绝；
// maxFiles / maxBytes 做解压限额（防 zip 炸弹）。
func Extract(archive, targetDir string, stripComponents, maxFiles int, maxBytes int64) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return pkg.Wrap(1013, "mkdir target failed", err)
	}
	name := strings.ToLower(filepath.Base(archive))
	if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
		return extractTarGz(archive, targetDir, stripComponents, maxFiles, maxBytes)
	}
	return extractZip(archive, targetDir, stripComponents, maxFiles, maxBytes)
}

func extractZip(archive, targetDir string, strip, maxFiles int, maxBytes int64) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return pkg.Wrap(1015, "open zip failed", err)
	}
	defer zr.Close()
	count, total := 0, int64(0)
	for _, f := range zr.File {
		rel, skip := stripName(f.Name, strip)
		if skip || f.FileInfo().IsDir() {
			continue
		}
		out, err := safeResolve(targetDir, rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return pkg.Wrap(1013, "mkdir entry failed", err)
		}
		count++
		if count > maxFiles {
			return errors.New("archive exceeds maxFiles")
		}
		rc, err := f.Open()
		if err != nil {
			return pkg.Wrap(1015, "open zip entry failed", err)
		}
		n, werr := writeLimited(rc, out, maxBytes-total)
		rc.Close()
		if werr != nil {
			return werr
		}
		total += n
	}
	return nil
}

func extractTarGz(archive, targetDir string, strip, maxFiles int, maxBytes int64) error {
	f, err := os.Open(archive)
	if err != nil {
		return pkg.Wrap(1015, "open tar.gz failed", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return pkg.Wrap(1015, "open gzip failed", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count, total := 0, int64(0)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return pkg.Wrap(1015, "read tar entry failed", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // 只解常规文件；目录由父路径自动创建
		}
		rel, skip := stripName(hdr.Name, strip)
		if skip {
			continue
		}
		out, err := safeResolve(targetDir, rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return pkg.Wrap(1013, "mkdir entry failed", err)
		}
		count++
		if count > maxFiles {
			return errors.New("archive exceeds maxFiles")
		}
		n, werr := writeLimited(tr, out, maxBytes-total)
		if werr != nil {
			return werr
		}
		total += n
	}
	return nil
}

// stripName 归一化分隔符 + 剥离前 N 层路径；skip=true 表示条目整体落在被剥离前缀里。
func stripName(name string, strip int) (string, bool) {
	var parts []string
	for _, p := range strings.Split(filepath.ToSlash(name), "/") {
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	if len(parts) <= strip {
		return "", true
	}
	return filepath.Join(parts[strip:]...), false
}

// safeResolve 把归档内相对路径解析到 targetDir 下，路径穿越（../）直接拒绝。
func safeResolve(targetDir, rel string) (string, error) {
	out := filepath.Join(targetDir, rel)
	if err := pkg.Within(targetDir, out); err != nil {
		return "", errors.New("path traversal detected: " + rel)
	}
	return out, nil
}

// writeLimited 把 src 写入 out（创建/覆盖），超过 limit 返回错误。
func writeLimited(src io.Reader, out string, limit int64) (int64, error) {
	dst, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, pkg.Wrap(1015, "create entry failed", err)
	}
	defer dst.Close()
	r := io.Reader(src)
	if limit > 0 {
		r = io.LimitReader(src, limit)
	}
	n, err := io.Copy(dst, r)
	if err != nil {
		return n, pkg.Wrap(1015, "write entry failed", err)
	}
	if limit > 0 && n >= limit {
		return n, errors.New("archive exceeds maxBytes")
	}
	return n, nil
}
