package service

import "WorkBaby/backend/service/platform"

// autoStartEnabled 以注册表为准：设置文件里写 1 而注册表里没有，等于没开。
func autoStartEnabled() bool { return platform.AutoStartEnabled() }

// SetAutoStart 打开或关闭开机自启。
func SetAutoStart(enabled bool, exePath string) error {
	return platform.SetAutoStart(enabled, exePath)
}
