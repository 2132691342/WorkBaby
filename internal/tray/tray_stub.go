//go:build !windows

package tray

// Run 非 Windows 平台空实现（无系统托盘）。
func Run(_ Actions) {}
