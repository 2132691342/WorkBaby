//go:build !windows

package pet

import "WorkBaby/internal/domain"

// ApplyHitRegion 非 Windows 平台暂未实现窗口区域裁剪（保留桌面壳原有行为）。
func ApplyHitRegion(_ []domain.PetHitRect) error { return nil }

// ClearHitRegion 非 Windows 平台空实现。
func ClearHitRegion() error { return nil }
