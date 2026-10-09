// 内置运行时的探测缓存与系统兜底探测。
// 工具路径只解析一次（解压耗时不该摊到每次调用），缓存可被 ResetProbe 清空。
package runtime

import (
	"os/exec"
	"sync"
)

var (
	probeMu        sync.Mutex // 只保护下面这些字段，不做任何 IO
	pythonProbed   bool
	pythonPath     string
	pythonErr      error
	pwshProbed     bool
	powershellPath string
	powershellErr  error

	// probeFlight 串行化「首次探测」：解压归档可能耗时数十秒，
	// 放在 probeMu 里会让缓存命中的读路径一起排队——那是每次工具调用都要走的路径。
	probeFlight sync.Mutex
)

// probeOnce 是首次探测的统一入口：缓存命中只碰 probeMu（微秒级），
// 未命中时在 probeFlight 闸门下用 double-check 保证只有一次真解压。
func probeOnce(probed *bool, path *string, perr *error, resolve func() (string, error)) (string, error) {
	probeMu.Lock()
	if *probed {
		p, e := *path, *perr
		probeMu.Unlock()
		return p, e
	}
	probeMu.Unlock()

	probeFlight.Lock()
	defer probeFlight.Unlock()
	probeMu.Lock()
	if *probed { // 等闸门期间别人已经做完
		p, e := *path, *perr
		probeMu.Unlock()
		return p, e
	}
	probeMu.Unlock()

	p, e := resolve()
	probeMu.Lock()
	*probed, *path, *perr = true, p, e
	probeMu.Unlock()
	return p, e
}

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
