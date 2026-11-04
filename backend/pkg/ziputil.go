package pkg

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// UnpackZip 解压 zip 到 target。zip 内是单一根目录时把它改名为 target，
// 这样 node-v24.21.0-win-x64.zip 解出来正好是 runtime\node。
// 先落 target.unpack 成功后一次改名：中途失败不留半个运行时，下次启动不会误判已就绪。
func UnpackZip(zipPath, target string) error {
	tmp := target + ".unpack"
	if err := os.RemoveAll(tmp); err != nil {
		return WrapErr(CodeProvision, "清理临时目录失败", err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return WrapErr(CodeProvision, "创建临时目录失败", err)
	}
	if err := extractAll(zipPath, tmp); err != nil {
		return err
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return WrapErr(CodeProvision, "读取临时目录失败", err)
	}
	if len(entries) == 1 && entries[0].IsDir() {
		if err := os.Rename(filepath.Join(tmp, entries[0].Name()), target); err != nil {
			return WrapErr(CodeProvision, "落位运行时失败", err)
		}
		os.RemoveAll(tmp)
		return nil
	}
	if err := os.Rename(tmp, target); err != nil {
		return WrapErr(CodeProvision, "落位运行时失败", err)
	}
	return nil
}

// extractAll 逐条目解压；写出错直接中断，不吞。
func extractAll(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return WrapErr(CodeProvision, "压缩包打不开："+filepath.Base(zipPath), err)
	}
	defer reader.Close()
	root, err := filepath.Abs(dest)
	if err != nil {
		return WrapErr(CodeProvision, "解压目标非法", err)
	}
	for _, item := range reader.File {
		target, err := safeJoin(root, item.Name)
		if err != nil {
			return err
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return WrapErr(CodeProvision, "建目录失败："+item.Name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return WrapErr(CodeProvision, "建目录失败："+item.Name, err)
		}
		src, err := item.Open()
		if err != nil {
			return WrapErr(CodeProvision, "读取压缩条目失败："+item.Name, err)
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			src.Close()
			return WrapErr(CodeProvision, "写入文件失败："+item.Name, err)
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		if closeErr := dst.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return WrapErr(CodeProvision, "写入文件失败："+item.Name, copyErr)
		}
	}
	return nil
}

// safeJoin 把 zip 内的相对路径拼到根下，并拒绝 zip-slip（../ 逃逸）。
func safeJoin(root, name string) (string, error) {
	joined := filepath.Join(root, strings.ReplaceAll(name, `\`, string(filepath.Separator)))
	if !Inside(root, joined) {
		return "", Err(CodeProvision, "压缩包内含越界路径", name)
	}
	return joined, nil
}
