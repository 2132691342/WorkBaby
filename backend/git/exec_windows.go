//go:build windows

// Windows 上起 git 子进程必须 CREATE_NO_WINDOW：否则每次调用闪一个控制台窗口。

package git

import (
	"os/exec"
	"syscall"
)

// hideConsole 阻止 GUI 进程调用 git 时黑窗闪现。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
