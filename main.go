// Package main 是 WorkBaby 的入口：只做装配，业务全部委托 backend 包。
package main

import (
	"embed"
	"errors"
	"os"
	"strings"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/singleinstance"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// version 是应用版本，与 wails.json 的 productVersion 保持一致。
const version = "1.0.0"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// 单实例：二次启动一律把这次启动交给主实例——带文件就转交文件，
	// 不带就请它把窗口带出来（否则双击图标在托盘/隐藏状态下像是没反应）。
	dir := dataDir()
	inst, ierr := singleinstance.Acquire(dir)
	if ierr != nil {
		if !errors.Is(ierr, singleinstance.ErrInstanceAlreadyRunning) {
			pkg.Errorf("workbaby: 单实例检查失败: %v", ierr)
			return
		}
		if err := singleinstance.SendPathToRunningInstance(dir, filePathFromArgs()); err != nil {
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
		},
	})
	if err != nil {
		pkg.Errorf("wails.Run failed: %v", err)
		os.Exit(1)
	}
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
