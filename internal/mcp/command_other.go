//go:build !windows

package mcp

import "os/exec"

// resolveCommand 非 Windows 平台无 .cmd 外壳；直接按 PATH 解析，找不到原样交给 exec。
func resolveCommand(name string) (string, []string) {
	if lp, err := exec.LookPath(name); err == nil {
		return lp, nil
	}
	return name, nil
}

// envWithPath 非 Windows 平台 PATH 大小写与 cmd.exe 兜底都不需要，原样返回。
func envWithPath(existing []string, _ []string) []string {
	return existing
}
