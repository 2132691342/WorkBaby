//go:build windows

// Windows 上起 PI 子进程：入口不走 shell，并给进程设 CREATE_NO_WINDOW。

package pi

import (
	"os/exec"
	"syscall"
)

// hideWindow 让子进程不弹控制台窗口。
// GUI 进程拉起控制台程序时不设 CREATE_NO_WINDOW，每次启动都会闪一个黑框。
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
