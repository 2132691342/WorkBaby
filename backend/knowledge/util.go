package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// chunkHash 给分块内容算指纹，用来去掉完全重复的块。
// 用 SHA-256 而不是 MD5：MD5 早已不适合做任何去重信任锚点。
func chunkHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type fileStat struct{ size int64 }

func statFile(path string) (fileStat, error) {
	st, err := os.Stat(path)
	if err != nil {
		return fileStat{}, err
	}
	return fileStat{size: st.Size()}, nil
}

// listDir 列出目录下一层的文件路径。
func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out, nil
}

// baseName 取不含扩展名的文件名作为文档标题。
func baseName(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}
