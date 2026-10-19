//go:build windows

package runtime

import (
	"os/exec"
	"syscall"
)

// hideWindow 让子进程不弹控制台窗口。
// GUI 进程拉起 node.exe / python.exe 这类控制台程序时不设 CREATE_NO_WINDOW，
// 每次探测都会闪一个黑框。
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
