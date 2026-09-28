//go:build !windows

// 非 Windows 平台不支持开机自启：本项目只面向 Windows，这里给稳定的空实现，
// 免得调用方要为「本来就不可能发生的平台」写分支。

package api

import "WorkBaby/internal/pkg"

var ErrAutoStartUnsupported = pkg.New(2304, "当前系统不支持开机自启", "")

// AutoStartEnabled 恒为 false。
func (h *Handler) AutoStartEnabled() bool { return false }

// SetAutoStart 恒返回不支持。
func (h *Handler) SetAutoStart(enabled bool) error { return ErrAutoStartUnsupported }
