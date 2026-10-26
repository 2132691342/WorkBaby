//go:build !windows

// 非 Windows 上没有 Node 的安装位置注册表可读。
package env

// nodeRegistryDirs 在非 Windows 上返回空。
func nodeRegistryDirs() []string { return nil }
