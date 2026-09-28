//go:build windows

package tool

import "os/exec"

// killTree 按进程树终止：只 kill 直接进程会留下孙进程继续占用文件与端口。
func killTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", itoa(pid))
	_ = cmd.Run()
	return nil
}
