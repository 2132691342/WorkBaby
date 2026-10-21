//go:build windows

// Windows 上把 npm shim 还原成 node + 入口脚本：经 cmd.exe 起进程会弹控制台窗口。

package env

import (
	"bufio"
	"io"
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

// pumpPipe 逐行读子进程输出并回调；读错误不致命（进程退出即可）。
func pumpPipe(r io.Reader, onLine func(string)) {
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for sc.Scan() {
			if onLine != nil {
				onLine(sc.Text())
			}
		}
	}()
}
