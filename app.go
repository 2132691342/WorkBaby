// 应用入口：Wails 生命周期编排，本地 HTTP 启动、托盘、单实例与退出流程都在这里。
package main

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/server"
	"WorkBaby/backend/singleinstance"
	"WorkBaby/backend/tray"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定面：只暴露系统能力（对话框 / 自启 / 退出），业务全走本地 HTTP。
// handler 持有而不嵌入：嵌入会把 gin handler 一起绑成前端永远调不通的 JS 方法。
type App struct {
	handler     *api.Handler
	srv         *server.Server
	tray        *tray.Tray
	trayUp      atomic.Bool
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
	return &App{handler: api.New(version), startDone: make(chan struct{}), startWait: defaultStartWait}
}

// startup 装配业务。结束前一定关闭 startDone，让 domReady 有确定的等待点。
func (a *App) startup(ctx context.Context) {
	defer close(a.startDone)
	if err := a.handler.Startup(ctx); err != nil {
		a.handler.SetStartupError(err)
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
		a.handler.SetStartupError(pkg.New(2001, "启动超时，后端装配没有在预期时间内完成", ""))
		return false
	}
}

// domReady 启动内嵌 HTTP、广播端口、拉起托盘。
func (a *App) domReady(ctx context.Context) {
	if !a.awaitStartup() {
		// 只能走 Wails 事件总线：此时 HTTP/SSE 还不存在，走 Emitter 等于没发。
		pkg.Errorf("app: 业务装配未完成，前端将显示启动失败: %s", a.handler.StartupError())
		wruntime.EventsEmit(ctx, "app:startup-error", map[string]string{
			"message": a.handler.StartupError(),
		})
		wruntime.WindowShow(ctx)
		return
	}
	a.srv = server.New(a.handler)
	port, err := a.srv.Start()
	if err != nil {
		a.handler.SetStartupError(err)
		wruntime.EventsEmit(ctx, "app:startup-error", map[string]string{"message": err.Error()})
		return
	}
	pkg.Infof("app: 本地服务已启动，端口 %d", port)
	// 兜底通道：绑定方法随时可调，不存在事件竞态。
	a.handler.SetServerPort(port)
	// 监听器注册晚于本次派发就漏掉端口（表现是全接口 404），故下面还会重发几次。
	wruntime.EventsEmit(ctx, "app:ready", map[string]any{"server_port": port})
	go repeatReady(ctx, port, 20)

	a.tray = tray.New(tray.Menu{
		Title: "WorkBaby", ShowText: "打开窗口", QuitText: "退出",
		// 唤起必须同时 Show + Unminimise：隐藏和最小化是两种状态，只做一半就唤不回。
		OnShow: func() {
			wruntime.WindowShow(ctx)
			wruntime.WindowUnminimise(ctx)
		},
		OnNew: func() {
			wruntime.WindowShow(ctx)
			wruntime.WindowUnminimise(ctx)
			wruntime.EventsEmit(ctx, "app:new-session", nil)
		},
		OnQuit: func() { a.quit(ctx) },
	})
	a.trayUp.Store(true)
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

// toTrayHintDwell 收进托盘前留给「已收进托盘」提示的可见时间。
const toTrayHintDwell = 1100 * time.Millisecond

// trayExitWait 退出时等托盘把图标从通知区摘掉的时长上限。
const trayExitWait = 300 * time.Millisecond

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

// closeAllowed 报告这次关闭是否应当真正放行（拆出来是为了能单测）。
func (a *App) closeAllowed() bool { return a.quitting.Load() }

// closeToTray 报告「关闭按钮」是否收进托盘；用户可以关掉这个行为，那时关闭就是真退出。
func (a *App) closeToTray() bool {
	if a.handler == nil || a.handler.Repo == nil {
		return true
	}
	v, err := a.handler.Repo.GetSetting(domain.SettingMinimizeToTray)
	if err != nil {
		return true
	}
	return v != "false"
}

// beforeClose 拦截窗口关闭并收进托盘。只有「托盘已就绪 + 设置开着 + 不在退出流程」
// 才拦：托盘没就绪时拦截会把窗口藏进一个不存在的托盘；退出流程中必须放行，
// 否则托盘「退出」是空操作。
func (a *App) beforeClose(ctx context.Context) bool {
	if a.closeAllowed() || !a.closeToTray() || !a.trayUp.Load() {
		return false
	}
	// 先让界面提示一句再隐藏：窗口「凭空消失」最让用户困惑。
	wruntime.EventsEmit(ctx, "app:to-tray")
	time.AfterFunc(toTrayHintDwell, func() {
		if a.quitting.Load() {
			return // 提示期间用户改了主意（托盘退出）：窗口交给退出流程处理
		}
		wruntime.WindowHide(ctx)
	})
	return true
}

// quit 真正退出：先立标志位再请 Wails 关闭；不导出（带 context 的方法绑到 JS 上调不通）。
func (a *App) quit(ctx context.Context) {
	if !a.quitting.CompareAndSwap(false, true) {
		return
	}
	wruntime.Quit(ctx)
	// WebView2 卡死时优雅退出可能不回来，进程兜底结束；正常路径由 shutdown 收尾。
	go func() {
		time.Sleep(8 * time.Second)
		pkg.Warnf("app: 优雅退出超时，强制结束进程")
		os.Exit(0)
	}()
}

// ForceQuit 给前端调用的退出入口：启动失败 / 托盘异常时需要一条确定能走通的真退出路径。
func (a *App) ForceQuit() {
	ctx := a.handler.Ctx()
	if ctx == nil {
		os.Exit(0)
	}
	a.quit(ctx)
}

// ---- Wails 绑定面：系统能力转发（api 包是 wails 能力的实现处，这里只做薄转发）----

// OpenFileDialog 打开文件选择框（知识库导入、附件）。
func (a *App) OpenFileDialog(title, filter string) (string, error) {
	return a.handler.OpenFileDialog(title, filter)
}

// OpenDirectoryDialog 打开目录选择框（工作目录、知识库目录）。
func (a *App) OpenDirectoryDialog(title string) (string, error) {
	return a.handler.OpenDirectoryDialog(title)
}

// AutoStartEnabled 报告当前是否已设置开机自启。
func (a *App) AutoStartEnabled() bool { return a.handler.AutoStartEnabled() }

// SetAutoStart 开关开机自启。
func (a *App) SetAutoStart(enabled bool) error { return a.handler.SetAutoStart(enabled) }

// shutdown 释放资源：停服务、退托盘、放实例锁。
// 清理完直接结束进程：WebView2 偶尔不返回 Close，run 事件循环就永远等下去。
func (a *App) shutdown(ctx context.Context) {
	a.quitting.Store(true)
	if a.srv != nil {
		a.srv.Stop()
	}
	if a.tray != nil {
		a.tray.Stop()
		// 给消息循环时间把图标摘掉：进程抢在前面退出会留下摘不掉的幽灵图标。
		a.tray.WaitForExit(trayExitWait)
	}
	a.handler.Shutdown()
	if a.inst != nil {
		a.inst.Release()
	}
	os.Exit(0)
}

// handleSecondLaunch 处理二次启动转交：把窗口带出来；带文件路径时再递给前端打开。
// 装配没完成前 Wails 运行时不可用（空 ctx 调用会直接终止进程），先在 startDone 上等。
func (a *App) handleSecondLaunch(path string) {
	select {
	case <-a.startDone:
	case <-time.After(a.startWait):
		pkg.Warnf("app: 二次启动转交被跳过：装配未完成")
		return
	}
	ctx := a.handler.Ctx()
	if ctx == nil {
		return // 装配失败：窗口都不存在，无从唤起
	}
	wruntime.WindowShow(ctx)
	wruntime.WindowUnminimise(ctx)
	if path != "" {
		wruntime.EventsEmit(ctx, "app:open-file", map[string]any{"path": path})
	}
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
