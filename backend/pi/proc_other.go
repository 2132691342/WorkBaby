//go:build !windows

// 非 Windows 平台的子进程启动：只保留接口，实际启动在 proc_windows.go。

package pi

import "os/exec"

// hideWindow 在非 Windows 平台无操作。
func hideWindow(cmd *exec.Cmd) {}
