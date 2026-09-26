// Package tray 提供系统托盘能力：Windows 用 Win32 自研实现，其余平台空实现。
package tray

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Actions 托盘动作回调集合，由 Start 装配后交给平台实现 Run 调用。
type Actions struct {
	ShowMain func() // 显示主窗口
	Hide     func() // 隐藏主窗口（保留托盘常驻）
	Quit     func() // 退出应用
}

// Start 启动系统托盘：独立 goroutine 进入消息循环，不阻塞主流程。
func Start(ctx context.Context) {
	if ctx == nil {
		return
	}
	go Run(Actions{
		ShowMain: func() { showMainWindow(ctx) },
		Hide:     func() { hideWindowToTray(ctx) },
		Quit:     func() { wruntime.Quit(ctx) },
	})
}

func showMainWindow(ctx context.Context) {
	wruntime.WindowShow(ctx)
	wruntime.WindowUnminimise(ctx)
}

func hideWindowToTray(ctx context.Context) {
	wruntime.WindowHide(ctx)
}