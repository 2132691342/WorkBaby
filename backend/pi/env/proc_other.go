//go:build !windows

// 非 Windows 平台的 PI 入口解析：只保留接口，实际解析在 proc_windows.go。

package env

import (
	"bufio"
	"io"
	"os/exec"
)

// hideWindow 在非 Windows 平台无操作。
func hideWindow(cmd *exec.Cmd) {}

// pumpPipe 逐行读子进程输出并回调。
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
