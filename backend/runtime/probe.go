// 内置运行时的探测缓存与系统兜底探测。
// 工具路径只解析一次（解压耗时不该摊到每次调用）；缓存可被 ResetProbe 清空——
// 首次解压失败后修好环境，「重新检测」要让工具路径也重新探测。
package runtime

import (
	"os/exec"
	"sync"
)

var (
	probeMu        sync.Mutex
	pythonProbed   bool
	pythonPath     string
	pythonErr      error
	pwshProbed     bool
	powershellPath string
	powershellErr  error
)

// ResetProbe 清空内置运行时探测缓存（设置页「重新检测」入口调用）。
func ResetProbe() {
	probeMu.Lock()
	pythonProbed, pwshProbed = false, false
	probeMu.Unlock()
}

// PythonSystemExe 返回系统 PATH 上的 Python，不含内置探测与缓存。
func PythonSystemExe() string {
	for _, name := range []string{"python.exe", "python"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return ""
}

// PowerShellSystemExe 返回系统 PATH 上的 PowerShell（pwsh 优先），不含内置探测与缓存。
func PowerShellSystemExe() string {
	for _, name := range []string{"pwsh.exe", "pwsh", "powershell.exe", "powershell"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return ""
}
