// Package shell 是 Wails 外壳：窗口、托盘、单实例、绑定给前端的系统能力。
// 边界：本包之外（backend/ 其余部分）不知道 Wails 存在；业务只经回环 HTTP 与前端说话，
// 这里只提供「界面外面的那一层」——端口、目录选择框、窗控、托盘。
package shell

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"WorkBaby/backend/api"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service"
	"WorkBaby/backend/sse"
)

// Options 是外壳启动参数。main 把版本号与内嵌资源交进来，本包不自己读构建信息。
type Options struct {
	// Version 由构建时注入（-ldflags "-X main.version=…"）
	Version string
	// Assets 是打进二进制的前端产物
	Assets embed.FS
}

// 窗口的逻辑尺寸（CSS 像素）。布局收窄由前端容器查询负责，这里不设最小尺寸。
const (
	windowLogicalWidth  = 1280
	windowLogicalHeight = 820
)

// singleInstanceID 判「两次启动是不是同一个应用」的唯一标识。
const singleInstanceID = "com.workbaby.desktop"

// quitGrace 是退出时给引擎进程的收尾时间。
// 用户点退出时想要的是一秒内关掉；超过这个时间就直接终止整棵进程树。
const quitGrace = 1500 * time.Millisecond

// Run 装配并运行桌面应用，阻塞到退出。
func Run(opts Options) error {
	// 数据目录必须在 application.New 之前解析：WebView2 的缓存路径要交给它，
	// 那一步一旦拿到不可写的路径就会弹框退出。
	exePath, err := os.Executable()
	if err != nil {
		return pkg.WrapErr(pkg.CodeInternal, "读不到可执行文件路径", err)
	}
	layout, err := resolveLayout(exePath)
	if err != nil {
		fatalDialog("无法启动", pkg.As(err).Message, pkg.As(err).Detail)
		return err
	}
	pkg.SetLogDir(layout.LogDir)
	pkg.Info("启动：数据目录=%s 资产目录=%s 开发态=%v", layout.DataDir, layout.AssetsDir, layout.Dev)

	app := &App{
		version:     opts.Version,
		layout:      layout,
		startDone:   make(chan struct{}),
		startHidden: hasArg("--hidden"),
	}

	wailsApp := application.New(application.Options{
		Name:        "WorkBaby",
		Description: "能干活的个人 AI 助手",
		Services:    []application.Service{application.NewService(app)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(opts.Assets),
		},
		// 第二次启动在 New 内部完成「通知首个实例 → 退出」，走不到下面建窗。
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				app.showWindow()
			},
		},
		OnShutdown: func() { app.shutdown() },
		Windows: application.WindowsOptions{
			// WebView2 的缓存放进自己的数据目录（默认落在 %APPDATA%\<exe 名>），
			// 否则卸载时那个几十 MB 的目录没人清理——安装器不认识它。
			WebviewUserDataPath: layoutDataSub(layout, "webview"),
		},
	})

	// 建窗尺寸按屏幕缩放比放大后再声明：v3 把 Width/Height 当物理像素用，
	// 200% 缩放下声明 1280 只得到 640 的 CSS 空间。
	winWidth, winHeight := windowPhysicalSize()
	win := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "WorkBaby",
		Width:            winWidth,
		Height:           winHeight,
		Frameless:        true,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(248, 250, 252),
		URL:              "/",
		// 建窗时不显示：装配阶段显形会让用户看到一帧白屏。
		// 前端画完首屏会调 Ready()，由那里显示。
		Hidden: true,
		Windows: application.WindowsWindow{
			NonClientRegionSupport: true,
		},
	})
	app.win = win
	app.app = wailsApp

	// 关窗行为可配：收托盘（默认）或直接退出。收托盘是默认（正在跑的对话不该
	// 被一键弄死），但用户必须能关掉它，第一次收托盘要告诉他「我还在」。
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if app.quitting.Load() {
			return
		}
		if app.closeAction() == service.CloseActionQuit {
			// 不能只放行：窗口关掉后托盘还挂着，消息循环不会自己结束，进程就「关不掉」。
			e.Cancel()
			app.requestQuit()
			return
		}
		e.Cancel()
		if app.svc != nil && app.svc.TrayHintPending() {
			app.askCloseDecision()
			return
		}
		app.toTray()
	})

	// 托盘在装配前建好：用户可能一启动就把窗口关到托盘。
	app.tray = wailsApp.SystemTray.New()
	app.buildTrayMenu(wailsApp, win)

	return wailsApp.Run()
}

// windowPhysicalSize 算出建窗时要声明的物理像素尺寸。
func windowPhysicalSize() (int, int) {
	scale := displayScale()
	return windowLogicalWidth * scale, windowLogicalHeight * scale
}

// resolveLayout 解析沙箱布局；exe 同级有 assets\ 就是安装态，否则找仓库根的 assets。
func resolveLayout(exePath string) (pkg.Layout, error) {
	exeDir := ""
	if dir, err := filepathDir(exePath); err == nil {
		exeDir = dir
	}
	layout := pkg.ResolveLayout(exeDir, false)
	// 开发态（go run / wails3 dev）exe 在临时目录里，资产在仓库根：
	// 从当前工作目录往上找 assets\runtime，找到就用它。
	if !pkg.IsDir(layout.AssetsDir + `\runtime`) {
		if repoRoot := findRepoRoot(); repoRoot != "" {
			dev := pkg.ResolveLayout(repoRoot, true)
			if pkg.IsDir(dev.AssetsDir + `\runtime`) {
				return dev, nil
			}
		}
	}
	if err := pkg.EnsureDir(layout.DataDir); err != nil {
		return layout, pkg.WrapErr(pkg.CodeFileNotFound, "数据目录不可写", err)
	}
	return layout, nil
}

// App 是 Wails 绑定面：只暴露系统能力，业务全走本地 HTTP。
// 不把 api 的 handler 嵌进来：嵌入会把它们的导出方法一起绑成前端永远调不通的 JS 方法。
type App struct {
	version string
	layout  pkg.Layout

	svc   *service.App
	srv   *api.Server
	store *repo.Store
	hub   *sse.Hub
	token string

	win  *application.WebviewWindow
	tray *application.SystemTray
	app  *application.App

	startDone   chan struct{}
	startOK     atomic.Bool
	startErr    string
	errMu       sync.Mutex
	startHidden bool
	revealed    atomic.Bool
	quitting    atomic.Bool
}

// ServiceStartup 是 Wails 的服务生命周期钩子：装配业务、启动后端。
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.startup(ctx)
	return nil
}

// startup 装配业务并起 HTTP；任何一步失败都把原因留给前端显示。
func (a *App) startup(ctx context.Context) {
	defer close(a.startDone)
	if err := a.assemble(); err != nil {
		ae := pkg.As(err)
		a.setStartErr(ae.Message)
		pkg.Error("业务装配失败 [%s] %s %s", ae.Code, ae.Message, ae.Detail)
		return
	}
	a.startOK.Store(true)
	pkg.Info("装配完成 workspace=%s", a.svc.Workspace())
	// 首启装配放到后台：端口与界面先就绪，解压与安装不阻塞用户。
	go a.svc.Provision()
}

// assemble 打开设置库、装配业务、起 HTTP。
func (a *App) assemble() error {
	store, err := repo.Open(a.layout.DbPath)
	if err != nil {
		return err
	}
	a.store = store
	a.hub = sse.New(1024)
	a.token = newToken()
	a.svc = service.New(service.Options{
		Layout:  a.layout,
		Store:   store,
		Hub:     a.hub,
		Version: a.version,
	})
	a.srv = api.New(a.svc, a.hub, a.token)
	if _, err := a.srv.Start(); err != nil {
		return err
	}
	// 开机自启的真实状态以注册表为准；设置文件里的记录只是展示用。
	return nil
}

// Endpoint 返回本地服务地址与令牌。等到服务真在监听才返回，
// 否则前端会拿到端口 0，把「后端没启动」当成永久失败。
func (a *App) Endpoint() (Endpoint, error) {
	select {
	case <-a.startDone:
	case <-time.After(20 * time.Second):
		a.setStartErr("后端装配超时")
		return Endpoint{}, fmt.Errorf("后端装配超时，请看设置页的日志")
	}
	if !a.startOK.Load() {
		return Endpoint{}, fmt.Errorf("%s", a.startMessage())
	}
	return Endpoint{
		Port:     a.srv.Port(),
		Token:    a.token,
		Version:  a.version,
		Contract: service.ContractVersion,
		BaseURL:  fmt.Sprintf("http://127.0.0.1:%d/api/v1", a.srv.Port()),
	}, nil
}

// setStartErr 记下装配失败原因。
func (a *App) setStartErr(message string) {
	a.errMu.Lock()
	a.startErr = message
	a.errMu.Unlock()
}

// startMessage 取装配失败原因。
func (a *App) startMessage() string {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	if a.startErr == "" {
		return "后端没有启动起来"
	}
	return a.startErr
}

// closeAction 关窗行为；装配未完成时按默认（收托盘）。
func (a *App) closeAction() string {
	if a.svc == nil {
		return service.CloseActionTray
	}
	return a.svc.CloseAction()
}

// hasArg 判断命令行里有没有某个标志。目前只用于 --hidden。
func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

// newToken 生成启动令牌；WB_DEV_TOKEN 只在本地调试时覆盖它。
func newToken() string {
	if dev := os.Getenv("WB_DEV_TOKEN"); dev != "" {
		return dev
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
