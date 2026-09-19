// Package tray 提供系统托盘能力：Windows 用 Win32 自研实现，其余平台空实现。
// 通过 PetToggler 接口与 Wails runtime 解耦，由 main 注入上下文与桌宠切换能力。
package tray

import (
	"context"

	"WorkBaby/internal/pkg"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Actions 托盘动作回调集合，由 Start 装配后交给平台实现 Run 调用。
type Actions struct {
	ShowMain func() // 显示主窗口
	Pet      func() // 召唤 / 收起桌宠
	Hide     func() // 隐藏主窗口（保留托盘常驻）
	Quit     func() // 退出应用
}

// PetToggler 桌宠形态切换能力（*api.Handler 满足）。
type PetToggler interface {
	PetMode() bool
	PetToggleMode() (map[string]any, error)
}

// Start 启动系统托盘：独立 goroutine 进入消息循环，不阻塞主流程。
func Start(ctx context.Context, pet PetToggler) {
	if ctx == nil {
		return
	}
	go Run(Actions{
		ShowMain: func() { showMainWindow(ctx, pet) },
		Pet:      func() { togglePetWindow(ctx, pet) },
		Hide:     func() { hideWindowToTray(ctx, pet) },
		Quit:     func() { wruntime.Quit(ctx) },
	})
}

// showMainWindow 打开主窗口（若处于桌宠形态先退出桌宠）。
func showMainWindow(ctx context.Context, pet PetToggler) {
	if pet.PetMode() {
		if _, err := pet.PetToggleMode(); err != nil {
			pkg.L.Warn("tray: exit pet mode failed", "err", err.Error())
		}
	}
	wruntime.WindowShow(ctx)
	wruntime.WindowUnminimise(ctx)
}

// togglePetWindow 召唤桌宠 ⇄ 回到主窗口。
func togglePetWindow(ctx context.Context, pet PetToggler) {
	if _, err := pet.PetToggleMode(); err != nil {
		pkg.L.Warn("tray: toggle pet mode failed", "err", err.Error())
		return
	}
	wruntime.WindowShow(ctx)
	wruntime.WindowUnminimise(ctx)
}

// hideWindowToTray 隐藏主窗口（保留托盘常驻）。
func hideWindowToTray(ctx context.Context, pet PetToggler) {
	if pet.PetMode() {
		if _, err := pet.PetToggleMode(); err != nil {
			pkg.L.Warn("tray: exit pet mode before hide failed", "err", err.Error())
			return
		}
	}
	wruntime.WindowHide(ctx)
}
