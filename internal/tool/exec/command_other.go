//go:build !windows

package exec

import "os/exec"

// resolveCommand 非 Windows 平台无 .cmd 外壳与 shell 内建命令，直接按 PATH 解析。
func resolveCommand(name string) (string, []string) {
	if lp, err := exec.LookPath(name); err == nil {
		return lp, nil
	}
	return name, nil
}

// wrapShell 非 Windows 平台无 shell 外壳与内建命令，解析结果即最终命令。
func wrapShell(abs string) (string, []string) { return abs, nil }
