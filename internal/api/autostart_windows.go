//go:build windows

// 开机自启：常驻型助手必须能在登录后自己回来，否则「关到托盘」等于用不了。
// 走 HKCU 而非 HKLM——不需要管理员权限，也不会在多用户机器上互相干扰。

package api

import (
	"os"
	"path/filepath"

	"WorkBaby/internal/pkg"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValue = "WorkBaby"
)

// AutoStartEnabled 报告当前是否已设置开机自启。
func (h *Handler) AutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, autoStartKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autoStartValue)
	return err == nil
}

// SetAutoStart 开关开机自启。开启时写入当前 exe 的绝对路径。
func (h *Handler) SetAutoStart(enabled bool) error {
	if !enabled {
		k, err := registry.OpenKey(registry.CURRENT_USER, autoStartKey, registry.SET_VALUE)
		if err != nil {
			return pkg.Wrap(2304, "写入注册表失败", err)
		}
		defer k.Close()
		if err := k.DeleteValue(autoStartValue); err != nil {
			return pkg.Wrap(2304, "取消开机自启失败", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return pkg.Wrap(2304, "获取程序路径失败", err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartKey, registry.SET_VALUE)
	if err != nil {
		return pkg.Wrap(2304, "写入注册表失败", err)
	}
	defer k.Close()
	cmd := `"` + filepath.Clean(exe) + `"`
	if err := k.SetStringValue(autoStartValue, cmd); err != nil {
		return pkg.Wrap(2304, "设置开机自启失败", err)
	}
	return nil
}
