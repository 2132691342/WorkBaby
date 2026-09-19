// Package main 是 WorkBaby 桌面应用的入口；只做装配，业务全部委托 internal 包。
package main

import (
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"WorkBaby/internal/singleinstance"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// 单实例：Mutex 检测 + 本地 TCP IPC。
	// 二次启动（含文件关联打开）把文件路径转交主实例后退出，避免多实例竞争托盘图标与 DB 锁。
	inst, ierr := singleinstance.Acquire()
	if ierr != nil {
		if !errors.Is(ierr, singleinstance.ErrInstanceAlreadyRunning) {
			log.Printf("workbaby: single instance check failed: %v", ierr)
			return
		}
		// 已有实例在跑：转交命令行文件路径（若有）后静默退出
		if path := filePathFromArgs(); path != "" {
			_ = singleinstance.SendPathToRunningInstance(path)
		}
		return
	}
	defer inst.Release()

	// 主实例：监听 IPC，收到路径后 emit app:open-file 给前端并唤起主窗口。
	if err := inst.StartListener(); err != nil {
		log.Printf("workbaby: ipc listener unavailable: %v", err)
	}
	go func() {
		for path := range inst.FileChannel() {
			if path == "" {
				continue
			}
			app.handleOpenFileFromIPC(path)
		}
	}()

	err := wails.Run(&options.App{
		Title:     "WorkBaby",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		// Frameless：启用前端自绘标题栏（wb-ui.css .titlebar + winctl），
		// 拖拽区用 --wails-draggable:drag 声明；Windows 下缩放手柄由 Wails 自带。
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 238, G: 244, B: 253, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
			// /files/** 走本地受管文件服务（媒体产物/工作区预览）；其余回退嵌入前端资源
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/files/") && app.Files != nil {
					app.Files.ServeHTTP(w, r)
					return
				}
				http.FileServer(http.FS(assets)).ServeHTTP(w, r)
			}),
		},
		OnStartup:     app.startup,
		OnDomReady:    app.domReady,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		// 双主机：业务 API 走 gin HTTP（app.startup 启动并注入端口）；
		// Wails 绑定仅保留系统能力（文件对话框 / 剪贴板 / 托盘）。
		Bind: []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatalf("wails.Run failed: %v", err)
	}
}

// filePathFromArgs 取命令行中第一个存在的文件路径（文件关联打开时由 Windows 传入）。
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
