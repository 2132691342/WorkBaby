//go:build !windows

package tool

import "os/exec"

// killTree 非 Windows 上不做进程树终止（本应用仅面向 Windows，这里只为保持可编译）。
func killTree(pid int) error { return nil }

// hideConsole 非 Windows 无控制台弹窗问题，空实现保持调用点统一。
func hideConsole(cmd *exec.Cmd) {}
