//go:build !windows

// 非 Windows 上 bash 由系统提供，没有注册表可查。
package env

// gitRegistryDirs 在非 Windows 上恒为空。
func gitRegistryDirs() []string { return nil }
