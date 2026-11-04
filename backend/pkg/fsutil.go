package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NowMs 是全项目唯一的时间戳形态：毫秒 number。禁止 ISO 串进 DTO。
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// EnsureDir 建目录（含父级）；已存在不算错误。
func EnsureDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// Exists 判断路径存在。探测一律用它，不跑 where / which——那样会闪控制台。
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir 判断路径是不是目录。
func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// ReadJSON 读一个 JSON 文件到 out；文件缺失或坏掉都返回 nil，绝不抛。
// 配置类文件由用户可手改，读坏了等同于「还没配过」，让界面重写一份比报错有用。
func ReadJSON(path string, out any) any {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil
	}
	return out
}

// WriteJSON 原子写一个 JSON 文件（临时文件 + 改名），并自动建父目录。
func WriteJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(raw, '\n'))
}

// WriteFileAtomic 先写同目录临时文件再改名：中途断电不会留下半个文件。
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wbtmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Inside 判断 child 是否落在 parent 目录内（含自身之外的子路径）。
// 所有「拼路径给用户读写的接口」都必须先过它，否则 ../ 就能越界。
func Inside(parent, child string) bool {
	p, err1 := filepath.Abs(parent)
	ch, err2 := filepath.Abs(child)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(p, ch)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// Truncate 按字符数截断，给标题和日志行用。
func Truncate(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}
