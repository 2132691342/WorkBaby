// 应用入口：Wails 生命周期编排，本地 HTTP 启动、托盘、单实例与退出流程都在这里。
package main

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/server"
	"WorkBaby/internal/singleinstance"
	"WorkBaby/internal/tray"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定入口：业务全部委托给内嵌的 api.Handler。
type App struct {
	*api.Handler
	srv         *server.Server
	tray        *tray.Tray
	inst        *singleinstance.Instance
	quitting    atomic.Bool
	pendingFile string
	// startDone 在 OnStartup 结束时关闭。
	// Wails 把 OnStartup 放在独立 goroutine，OnDomReady 由 WebView2 导航回调触发，
	// 两者无顺序保证——首次启动解压内置 Python 要几秒，domReady 必然先到。
	startDone chan struct{}
	startOK   atomic.Bool
	// startWait 是等待装配的上限；做成字段是为了能单测里缩短它。
	startWait time.Duration
}

// NewApp 构造应用壳。
func NewApp() *App {
	return &App{Handler: api.New(version), startDone: make(chan struct{}), startWait: defaultStartWait}
}

// startup 装配业务。结束前一定关闭 startDone，让 domReady 有确定的等待点。
func (a *App) startup(ctx context.Context) {
	defer close(a.startDone)
	if err := a.Handler.Startup(ctx); err != nil {
		a.Handler.SetStartupError(err)
		return
	}
	a.startOK.Store(true)
	pkg.Infof("app: 业务装配完成")
}

// awaitStartup 等装配结束，最多等 startWait 上限。
// 装配本身失败会立刻返回；只有「卡住不动」才会走到超时。
func (a *App) awaitStartup() bool {
	select {
	case <-a.startDone:
		return a.startOK.Load()
	case <-time.After(a.startWait):
		a.Handler.SetStartupError(pkg.New(2001, "启动超时，后端装配没有在预期时间内完成", ""))
		return false
	}
}

// domReady 启动内嵌 HTTP、广播端口、拉起托盘。
func (a *App) domReady(ctx context.Context) {
	if !a.awaitStartup() {
		// 只能走 Wails 事件总线：此时 HTTP/SSE 还不存在，走 Emitter 等于没发。
		pkg.Errorf("app: 业务装配未完成，前端将显示启动失败: %s", a.Handler.StartupError())
		wruntime.EventsEmit(ctx, "app:startup-error", map[string]string{
			"message": a.Handler.StartupError(),
		})
		wruntime.WindowShow(ctx)
		return
	}
	a.srv = server.New(a.Handler)
	port, err := a.srv.Start()
	if err != nil {
		a.Handler.SetStartupError(err)
		wruntime.EventsEmit(ctx, "app:startup-error", map[string]string{"message": err.Error()})
		return
	}
	pkg.Infof("app: 本地服务已启动，端口 %d", port)
	// 前端监听器注册晚于本次派发就会漏掉端口，表现为「所有接口 404」，故短时间重发。
	wruntime.EventsEmit(ctx, "app:ready", map[string]any{"server_port": port})
	go repeatReady(ctx, port, 20)

	a.tray = tray.New(tray.Menu{
		Title: "WorkBaby", ShowText: "打开窗口", QuitText: "退出",
		OnShow: func() { wruntime.WindowShow(ctx) },
		OnNew:  func() { wruntime.EventsEmit(ctx, "app:new-session", nil) },
		OnQuit: func() { a.Quit(ctx) },
	})
	go a.tray.Start()

	// 双击 md/txt 启动时没人可转交，等前端就绪后再把路径递过去。
	if p := a.pendingFile; p != "" {
		a.pendingFile = ""
		go func() {
			time.Sleep(1200 * time.Millisecond)
			wruntime.EventsEmit(ctx, "app:open-file", map[string]any{"path": p})
		}()
	}
}

// 内置 Python 首次解压要几秒，装配等待留够余量；超时只判「卡死」，不误判「慢」。
const defaultStartWait = 45 * time.Second

// repeatReady 在启动后的头几秒里重复广播端口，直到前端接住。
// 这是对「事件先于监听器发出」的唯一可靠解法，成本可以忽略。
func repeatReady(ctx context.Context, port, times int) {
	for i := 0; i < times; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(300 * time.Millisecond):
			wruntime.EventsEmit(ctx, "app:ready", map[string]any{"server_port": port})
		}
	}
}

// closeAllowed 报告这次关闭是否应当真正放行。
// 拆出来是为了能单测：真正调用前要碰 Wails 上下文。
func (a *App) closeAllowed() bool { return a.quitting.Load() }

// closeToTray 报告「关闭按钮」是否收进托盘。用户可以关掉这个行为，
// 那时关闭就是真的退出——否则关掉后台驻留的人会发现程序怎么都退不掉。
func (a *App) closeToTray() bool {
	if a.Handler == nil || a.Handler.Repo == nil {
		return true
	}
	v, err := a.Handler.Repo.GetSetting(domain.SettingMinimizeToTray)
	if err != nil {
		return true
	}
	return v != "false"
}

// beforeClose 拦截窗口关闭并收进托盘。
// 已经在退出流程中时必须放行：Wails 的 Quit() 会先调这个回调，
// 无条件返回 true 会让「退出」变成空操作，进程留在后台还占着 exe。
func (a *App) beforeClose(ctx context.Context) bool {
	if a.closeAllowed() || !a.closeToTray() {
		return false
	}
	wruntime.WindowHide(ctx)
	return true
}

// Quit 真正退出：先立标志位再请 Wails 关闭，并留一道进程级兜底。
func (a *App) Quit(ctx context.Context) {
	if !a.quitting.CompareAndSwap(false, true) {
		return
	}
	wruntime.Quit(ctx)
	// 优雅退出在 WebView2 卡死时可能永远不回来，进程就留在后台占着 exe。
	// 这个定时器只兜底，正常路径由 shutdown 结束。
	go func() {
		time.Sleep(8 * time.Second)
		pkg.Warnf("app: 优雅退出超时，强制结束进程")
		os.Exit(0)
	}()
}

// ForceQuit 给前端调用的退出入口，不接收 context。
// 启动失败 / 托盘异常时前端需要一条确定能走通的真退出路径。
func (a *App) ForceQuit() {
	ctx := a.Handler.Ctx()
	if ctx == nil {
		os.Exit(0)
	}
	a.Quit(ctx)
}

// shutdown 释放资源：停服务、退托盘、放实例锁。
// 清理完直接结束进程：WebView2 偶尔不返回 Close，run 事件循环就永远等下去。
func (a *App) shutdown(ctx context.Context) {
	a.quitting.Store(true)
	if a.srv != nil {
		a.srv.Stop()
	}
	if a.tray != nil {
		a.tray.Stop()
	}
	a.Handler.Shutdown()
	if a.inst != nil {
		a.inst.Release()
	}
	os.Exit(0)
}

// handleOpenFileFromIPC 处理文件关联打开：把路径交给前端放进输入框。
func (a *App) handleOpenFileFromIPC(path string) {
	if a.Handler == nil {
		return
	}
	wruntime.WindowShow(a.Handler.Ctx())
	wruntime.EventsEmit(a.Handler.Ctx(), "app:open-file", map[string]any{"path": path})
}

// attachInstance 保存单实例句柄，供 shutdown 释放。
func (a *App) attachInstance(inst *singleinstance.Instance) { a.inst = inst }

// dataDir 取数据目录，供单实例锁使用。
func dataDir() string {
	paths, err := runtime.Resolve()
	if err != nil {
		return ""
	}
	return paths.DataDir
}
