//go:build !windows

package tool

// killTree 非 Windows 上不做进程树终止（本应用仅面向 Windows，这里只为保持可编译）。
func killTree(pid int) error { return nil }
