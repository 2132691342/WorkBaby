//go:build !windows

// 非 Windows 上没有用户 PATH 的注册表可写：直接返回进程 PATH。
package env

import "os"

// userPathValue 返回进程 PATH。
func userPathValue() string { return os.Getenv("PATH") }

// machinePathValue 在非 Windows 上没有系统级 PATH 可读。
func machinePathValue() string { return "" }

// prependUserPath 在非 Windows 上不改环境变量。
func prependUserPath(string) error { return nil }

// removeUserPathEntry 在非 Windows 上不改环境变量。
func removeUserPathEntry(string) error { return nil }
