//go:build !windows

// 非 Windows 平台不起控制台窗口：这里只保留统一接口，实际逻辑在 exec_windows.go。

package git

import (
	"os/exec"
)

// hideConsole 非 Windows 平台无事可做；与 exec_windows.go 同构，按构建标签分流。
func hideConsole(cmd *exec.Cmd) {}
