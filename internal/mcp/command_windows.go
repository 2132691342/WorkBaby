//go:build windows

package mcp

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveCommand 把命令名解析为可直接 CreateProcess 的可执行文件与其前置参数。Windows 上
// .cmd / .bat 外壳（npm / npx / uvx）直接 CreateProcess 会报 ERROR_BAD_EXE_FORMAT(193)，
// 因此与 internal/tool/exec 共用同一套兜底。
func resolveCommand(name string) (string, []string) {
	lp, err := exec.LookPath(name)
	if err != nil {
		return "cmd", []string{"/c", name}
	}
	if ext := strings.ToLower(filepath.Ext(lp)); ext == ".cmd" || ext == ".bat" {
		return "cmd", []string{"/c", lp}
	}
	return lp, nil
}

// envWithPath 在现有环境基础上把 dirs 前置到 PATH。Windows 变量名大小写不敏感，用
// strings.EqualFold 整体替换原 PATH 条目（os.Environ() 可能返回 "Path=" 而匹配不上 "PATH="，
// 按字面匹配会丢掉整条系统 PATH）；PATH 为空时退化为只用内置运行时目录。
func envWithPath(existing []string, dirs []string) []string {
	if len(dirs) == 0 {
		return existing
	}
	sep := string([]byte{0})
	if sep == "" {
		sep = ";"
	}
	// 找原 PATH 值
	var pathVal string
	for _, kv := range existing {
		if i := strings.Index(kv, "="); i > 0 && strings.EqualFold(kv[:i], "PATH") {
			pathVal = kv[i+1:]
			break
		}
	}
	extra := strings.Join(dirs, sep)
	if pathVal != "" {
		pathVal = extra + sep + pathVal
	} else {
		pathVal = extra
	}
	out := make([]string, 0, len(existing)+1)
	for _, kv := range existing {
		if i := strings.Index(kv, "="); i > 0 && strings.EqualFold(kv[:i], "PATH") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PATH="+pathVal)
}
