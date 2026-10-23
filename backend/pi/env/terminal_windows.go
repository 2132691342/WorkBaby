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
	return expandPathValue(value)
}

// machinePathValue 读系统级 PATH（只读）：新开的终端先走系统级，再走用户级。
func machinePathValue() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("Path")
	if err != nil {
		return ""
	}
	return expandPathValue(value)
}

// prependUserPath 把目录放到用户 PATH 最前面并广播变更通知。
// 必须前置：系统里可能还有另一份 pi，只有排在最前面，终端敲到的才是本应用管的那份。
func prependUserPath(dir string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return pkg.Wrap("9720", "打不开用户环境变量，改不了 PATH", err)
	}
	defer key.Close()

	current, kind, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return pkg.Wrap("9720", "读不到用户 PATH", err)
	}
	next := withPathFirst(current, dir)
	if strings.EqualFold(next, strings.TrimSpace(current)) {
		return nil
	}
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
	pkg.Infof("env: 终端入口已置于用户 PATH 最前面：%s", dir)
	return nil
}

// withPathFirst 返回「dir 在最前、其余条目保持原顺序、重复项去掉」的 PATH 串。
func withPathFirst(current, dir string) string {
	target := normalizePathKey(dir)
	rest := make([]string, 0, 24)
	for _, entry := range strings.Split(current, ";") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if normalizePathKey(entry) == target {
			continue
		}
		rest = append(rest, entry)
	}
	if len(rest) == 0 {
		return dir
	}
	return dir + ";" + strings.Join(rest, ";")
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
