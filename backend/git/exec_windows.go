//go:build windows

package git

import (
	"os/exec"
	"syscall"
)

// hideConsole 阻止 GUI 进程调用 git 时黑窗闪现。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
