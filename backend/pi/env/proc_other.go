//go:build !windows

// 非 Windows 平台的 PI 入口解析：只保留接口，实际解析在 proc_windows.go。

package env

import "os/exec"

// hideWindow 在非 Windows 平台无操作。
func hideWindow(cmd *exec.Cmd) {}
