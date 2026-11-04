// Package platform 收口 Windows 外壳能力：开机自启、资源管理器定位、系统默认程序打开。
// 它只依赖标准库与 pkg，不感知业务。
package platform

import (
	"os/exec"
	"syscall"

	"WorkBaby/backend/pkg"
)

// createNoWindow 与 pkg 里的同名常量对齐：这里直接开任务，避免为一个数字互相依赖。
const createNoWindow = 0x08000000

// openPath 用系统默认程序打开一个路径。
func openPath(path string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "打开路径失败", err)
	}
	// 只 Start 不 Wait：等它退出会把界面卡到用户关掉那个程序为止。
	go func() { _ = cmd.Wait() }()
	return nil
}

// revealPath 在资源管理器里定位一个路径。
func revealPath(path string) error {
	// 目录直接打开，文件走 /select 高亮。
	args := []string{"/select,", path}
	if pkg.IsDir(path) {
		args = []string{path}
	}
	cmd := exec.Command("explorer.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return pkg.WrapErr(pkg.CodeFileIO, "在资源管理器中定位失败", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// trimsQuotes 去掉命令行里可能带上的引号。
func trimsQuotes(value string) string {
	if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"') {
		return value[1 : len(value)-1]
	}
	return value
}
