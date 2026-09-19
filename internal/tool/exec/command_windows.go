//go:build windows

package exec

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveCommand 把命令名解析为可直接 CreateProcess 的可执行文件与前置参数。
// Windows 上 .cmd/.bat 外壳（npm/npx 等）与 dir/type 等 cmd 内建命令必须包一层 cmd /c。
// 白名单校验仍针对原始 command，包 shell 只是执行细节，不放宽白名单。
func resolveCommand(name string) (string, []string) {
	lp, err := exec.LookPath(name)
	if err != nil {
		return "cmd", []string{"/c", name}
	}
	return wrapShell(lp)
}

// wrapShell 已解析到具体可执行文件后的收尾：.cmd / .bat 需包 cmd /c 才能 CreateProcess。
func wrapShell(abs string) (string, []string) {
	if ext := strings.ToLower(filepath.Ext(abs)); ext == ".cmd" || ext == ".bat" {
		return "cmd", []string{"/c", abs}
	}
	return abs, nil
}
