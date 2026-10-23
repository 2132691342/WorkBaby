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
	key, err := openUserPathKey()
	if err != nil {
		return err
	}
	defer key.Close()
	current, kind, err := readPathValue(key)
	if err != nil {
		return pkg.Wrap("9720", "读不到用户 PATH", err)
	}
	if err := writePathIfChanged(key, current, kind, withPathFirst(current, dir)); err != nil {
		return err
	}
	broadcastSettingChange()
	pkg.Infof("env: 终端入口已置于用户 PATH 最前面：%s", dir)
	return nil
}

// removeUserPathEntry 从用户 PATH 移除目录并广播；没有该目录时不写。
func removeUserPathEntry(dir string) error {
	key, err := openUserPathKey()
	if err != nil {
		return err
	}
	defer key.Close()
	current, kind, err := readPathValue(key)
	if err != nil {
		return pkg.Wrap("9720", "读不到用户 PATH", err)
	}
	if err := writePathIfChanged(key, current, kind, withoutPathEntry(current, dir)); err != nil {
		return err
	}
	broadcastSettingChange()
	return nil
}

// appendMachinePathEntry 把目录追加到系统级 PATH 末尾并广播（需要管理员）。
func appendMachinePathEntry(dir string) error {
	key, err := openMachinePathKey()
	if err != nil {
		return pkg.Wrap("9723", "打不开系统环境变量（需要管理员授权）", err)
	}
	defer key.Close()
	current, kind, err := readPathValue(key)
	if err != nil {
		return pkg.Wrap("9723", "读不到系统级 PATH", err)
	}
	if err := writePathIfChanged(key, current, kind, withPathLast(current, dir)); err != nil {
		return pkg.Wrap("9723", "写不进系统级 PATH（需要管理员授权）", err)
	}
	broadcastSettingChange()
	return nil
}

// removeMachinePathEntry 从系统级 PATH 移除目录并广播（需要管理员）。
func removeMachinePathEntry(dir string) error {
	key, err := openMachinePathKey()
	if err != nil {
		return pkg.Wrap("9723", "打不开系统环境变量（需要管理员授权）", err)
	}
	defer key.Close()
	current, kind, err := readPathValue(key)
	if err != nil {
		return pkg.Wrap("9723", "读不到系统级 PATH", err)
	}
	if err := writePathIfChanged(key, current, kind, withoutPathEntry(current, dir)); err != nil {
		return pkg.Wrap("9723", "写不进系统级 PATH（需要管理员授权）", err)
	}
	broadcastSettingChange()
	return nil
}

// openUserPathKey 打开用户环境键（读写）。
func openUserPathKey() (registry.Key, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return 0, pkg.Wrap("9720", "打不开用户环境变量，改不了 PATH", err)
	}
	return key, nil
}

// openMachinePathKey 打开系统环境键（读写；无管理员权限时在此失败）。
func openMachinePathKey() (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
}

// readPathValue 读键下的 Path 原值与类型；值不存在返回空串与 EXPAND_SZ。
func readPathValue(key registry.Key) (string, uint32, error) {
	value, kind, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return "", 0, err
	}
	return value, kind, nil
}

// writePathIfChanged 按原值类型写回 PATH：REG_SZ 就还写 REG_SZ，改类型会让已有的 %VAR% 语义跟着变；
// 值没变就不动注册表。
func writePathIfChanged(key registry.Key, current string, kind uint32, next string) error {
	next = strings.TrimSpace(next)
	if strings.EqualFold(next, strings.TrimSpace(current)) {
		return nil
	}
	var err error
	if kind == registry.SZ {
		err = key.SetStringValue("Path", next)
	} else {
		err = key.SetExpandStringValue("Path", next)
	}
	return err
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
