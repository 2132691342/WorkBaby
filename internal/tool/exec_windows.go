//go:build windows

package tool

import (
	"os/exec"
	"syscall"
)

// hideConsole 阻止控制台弹窗：GUI 进程拉起控制台子进程时，
// 不设 CREATE_NO_WINDOW 每次工具调用都会闪一个黑框——「powershell 老弹窗」的根因。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// killTree 按进程树终止：只 kill 直接进程会留下孙进程继续占用文件与端口。
func killTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", itoa(pid))
	hideConsole(cmd)
	_ = cmd.Run()
	return nil
}
