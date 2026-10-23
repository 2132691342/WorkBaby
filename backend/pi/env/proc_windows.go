//go:build windows

// Windows 上把 npm shim 还原成 node + 入口脚本：经 cmd.exe 起进程会弹控制台窗口。

package env

import (
	"os/exec"
	"syscall"
)

// hideWindow 让子进程不弹控制台窗口。
func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
