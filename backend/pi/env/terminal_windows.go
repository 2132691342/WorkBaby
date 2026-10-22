//go:build windows

// 用户级 PATH 的读写：只动 HKCU\Environment，不碰系统级 PATH。
package env

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"WorkBaby/backend/pkg"
)

// userPathValue 读用户级 PATH：进程启动时拿到的那份是旧的，
// 判断「终端里能不能敲 pi」必须看注册表里现在这份。
func userPathValue() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("Path")
	if err != nil {
		return ""
	}
	return value
}

// addUserPath 把目录追加到用户 PATH 末尾并广播变更通知。
func addUserPath(dir string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return pkg.Wrap("9720", "打不开用户环境变量，改不了 PATH", err)
	}
	defer key.Close()

	current, kind, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return pkg.Wrap("9720", "读不到用户 PATH", err)
	}
	if pathHasEntry(current, dir) {
		return nil
	}
	next := strings.TrimRight(current, ";")
	if next != "" {
		next += ";"
	}
	next += dir
	// 原值是 REG_SZ 就还写 REG_SZ：改类型会让已有的 %VAR% 语义跟着变。
	if kind == registry.SZ {
		err = key.SetStringValue("Path", next)
	} else {
		err = key.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return pkg.Wrap("9720", "写不进用户 PATH", err)
	}
	broadcastSettingChange()
	return nil
}

// broadcastSettingChange 告诉系统环境变量变了：新开的终端才会带上新 PATH。
func broadcastSettingChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	name, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW")
	_, _, _ = proc.Call(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(name)), smtoAbortIfHung, 3000, uintptr(unsafe.Pointer(&result)))
}
