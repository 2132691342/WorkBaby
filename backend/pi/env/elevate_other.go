//go:build !windows

// 提权与非 Windows：本期只考虑 Windows，这里只保证包能编过、行为是「不支持」。
package env

import "WorkBaby/backend/pkg"

// elevatePathAdmin 在非 Windows 上不支持。
func elevatePathAdmin(level string, dirs []string) error {
	return pkg.New("9723", "该平台不支持 PATH 级别切换", level)
}

// RunPathAdminMode 在非 Windows 上不命中任何标志。
func RunPathAdminMode(args []string) (int, bool) { return 0, false }
