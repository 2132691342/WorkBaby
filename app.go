// Package main 提供 App 结构体，承载 Wails 生命周期；通过嵌入 *api.Handler 自动继承系统能力绑定。
// 双主机架构（doc/01 §1）：业务 API 走 gin HTTP，Wails 绑定仅保留系统能力（对话框/剪贴板/托盘）。
package main

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/server"
	"WorkBaby/internal/tray"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定根对象；嵌入 *api.Handler 后系统能力绑定自动生效。
type App struct {
	ctx context.Context
	srv *server.Server // 双主机 HTTP 服务（gin + SSE）
	*api.Handler

	// IPC 收到文件路径但窗口 ctx 未就绪时暂存，startup 后补发
	pendingOpenFile string
	openFileMu      sync.Mutex
}

// NewApp 构造壳：Handler 内部按需注入，可单测不依赖 Wails runtime。
func NewApp() *App {
	return &App{Handler: api.NewHandler()}
}

// startup 注入 ctx；按 doc/01 §4 初始化序列执行：路径 → 日志 → 配置 → DB → 迁移 →
// repo/service → 启动 gin server → EmitReady(serverPort) → 前端建立 HTTP 连接。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.Handler.Startup(ctx); err != nil {
		// 初始化失败直接 panic：初始化序列内任何失败都意味着半残状态比不可用更危险。
		panic("app startup failed: " + err.Error())
	}

	// 双主机：启动 gin HTTP + SSE（127.0.0.1 随机端口），拿端口注入前端
	srv := server.New(a.Handler, a.Handler.Events(), a.Handler.EventLog())
	if _, err := srv.Start(); err != nil {
		panic("http server start failed: " + err.Error())
	}
	a.srv = srv
	a.Handler.EmitReady(srv.Port())
	tray.Start(a.ctx, a) // 系统托盘：显示主窗 / 召唤桌宠 / 隐藏 / 退出
	a.flushPendingOpenFile()
	if pkg.L != nil {
		pkg.L.Info("workbaby http ready", "port", srv.Port())
	}
}

// handleOpenFileFromIPC 处理二次启动实例转交的文件路径（文件关联）：
// 唤起主窗口并 emit app:open-file 事件；ctx 未就绪（极端时序）则暂存到 startup 后补发。
func (a *App) handleOpenFileFromIPC(path string) {
	if a.ctx == nil {
		a.openFileMu.Lock()
		a.pendingOpenFile = path
		a.openFileMu.Unlock()
		return
	}
	a.emitOpenFile(path)
}

// flushPendingOpenFile startup 后补发暂存的文件打开请求。
func (a *App) flushPendingOpenFile() {
	a.openFileMu.Lock()
	path := a.pendingOpenFile
	a.pendingOpenFile = ""
	a.openFileMu.Unlock()
	if path != "" && a.ctx != nil {
		a.emitOpenFile(path)
	}
}

// emitOpenFile 唤起主窗口并通知前端导入文件。
func (a *App) emitOpenFile(path string) {
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
	wruntime.EventsEmit(a.ctx, "app:open-file", path)
	if pkg.L != nil {
		pkg.L.Info("workbaby ipc open-file", "path", path)
	}
}

// domReady 在前端完成 DOM 初始化后补发端口事件，覆盖启动事件早于前端监听器的时序窗口。
func (a *App) domReady(ctx context.Context) {
	a.ctx = ctx
	if a.srv == nil {
		return
	}
	a.Handler.EmitReady(a.srv.Port())
}

// GetServerPort 返回 gin 监听端口（0 = 后端尚未就绪）。
// 前端启动引导优先走这个绑定而不是只等 app:ready 事件：事件是即发即忘的，
// 后端 startup 里发出的那一发可能早于前端监听器注册，首启（localStorage 无缓存）
// 就会永远等不到端口。同步取值没有时序窗口，可以从根上消除这个竞态。
func (a *App) GetServerPort() int {
	if a.srv == nil {
		return 0
	}
	return a.srv.Port()
}

// beforeClose 拦截窗口关闭：开启「关闭到托盘」时隐藏窗口保持常驻（返回 true = 阻止真正的退出）。
// 设置读取失败按退出处理（fail-closed，保证用户永远有路可走）。
func (a *App) beforeClose(ctx context.Context) bool {
	v, err := a.SettingValue(ctx, domain.SettingKeyTrayCloseToTray)
	if err == nil && strings.EqualFold(strings.TrimSpace(v), "true") {
		wruntime.WindowHide(ctx)
		return true
	}
	return false
}

// shutdown 镜像 startup 的反序：先关 HTTP/SSE，再关业务，最后 DB。
func (a *App) shutdown(ctx context.Context) {
	if a.srv != nil {
		a.srv.Shutdown(ctx)
	}
	a.Handler.Shutdown(ctx)
}
