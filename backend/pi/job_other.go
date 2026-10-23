//go:build !windows

// 非 Windows 平台没有 Job 对象：进程生命周期由父进程的 Stop 负责。
package pi

import "os/exec"

// attachToJob 在非 Windows 平台无操作。
func attachToJob(*exec.Cmd) {}
