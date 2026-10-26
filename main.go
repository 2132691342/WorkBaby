// Package main 是 WorkBaby 的入口：只做装配，业务全部委托 backend 包。
package main

import (
	"embed"
	"errors"
	"os"
	"strings"

	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/singleinstance"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// version 是应用版本，与 wails.json 的 productVersion 及 git tag 保持一致。
const version = "1.4.0"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 提权子进程：--path-admin <system|user|none> <dir;dir>，由引擎页 / 向导页的提级动作拉起，
	// 改完注册表即退出——必须在单实例守卫与 Wails 装配之前退出。
	if code, handled := pienv.RunPathAdminMode(os.Args); handled {
		os.Exit(code)
	}

	app := NewApp()

	// 单实例：二次启动一律把这次启动交给主实例——带文件就转交文件，
	// 不带就请它把窗口带出来（否则双击图标在托盘/隐藏状态下像是没反应）。
	dir := dataDir()
	webviewDir := ensureWebviewDir(dir)
	inst, ierr := singleinstance.Acquire(dir)
	if ierr != nil {
		if !errors.Is(ierr, singleinstance.ErrInstanceAlreadyRunning) {
			pkg.Errorf("workbaby: 单实例检查失败: %v", ierr)
			return
		}
		if err := singleinstance.SendPathToRunningInstance(dir, filePathFromArgs()); err != nil {
			// 主实例多半还没写完端口文件：不另开窗口，记 warn 后退出，再双击即可唤起。
			pkg.Warnf("workbaby: 转交到已运行实例失败: %v", err)
		}
		return
	}
	app.attachInstance(inst)

	if err := inst.StartListener(); err != nil {
		pkg.Warnf("workbaby: 实例通信不可用: %v", err)
	} else {
		go func() {
			for path := range inst.FileChannel() {
				app.handleSecondLaunch(path)
			}
		}()
	}

	// 首次启动就带着文件进来（双击 md/txt）：主实例没人可转交，
	// 只能自己等前端就绪后再把路径递过去。
	app.pendingFile = filePathFromArgs()

	err := wails.Run(&options.App{
		Title:     "WorkBaby",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		// Frameless：启用前端自绘标题栏（--wails-draggable:drag 声明拖拽区）。
		Frameless:        true,
		StartHidden:      hasArg("--hidden"),
		BackgroundColour: &options.RGBA{R: 242, G: 246, B: 252, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     app.startup,
		OnDomReady:    app.domReady,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind:          []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			// WebView2 profile 默认落在 %APPDATA% 下、名字像 exe 的几十 MB 缓存目录，
			// 挪进数据目录后卸载能一次清干净。
			WebviewUserDataPath: webviewDir,
		},
	})
	if err != nil {
		pkg.Errorf("wails.Run failed: %v", err)
		os.Exit(1)
	}
}

// hasArg 判断命令行里有没有某个标志。
// 目前只用于 --hidden：开机自启时只拉起托盘，不往屏幕上弹一个空窗口。
func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

// filePathFromArgs 取命令行中第一个存在的文件路径。
func filePathFromArgs() string {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if _, err := os.Stat(arg); err == nil {
			return arg
		}
	}
	return ""
}
