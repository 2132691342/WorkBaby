//go:build !windows

package git

import (
	"os/exec"
)

// hideConsole 非 Windows 平台无事可做；与 exec_windows.go 同构，按构建标签分流。
func hideConsole(cmd *exec.Cmd) {}
