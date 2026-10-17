// Package main 是 WorkBaby 的入口：只做装配，业务全部委托 backend 包。
package main

import (
	"embed"
	"errors"
	"io"
	"os"
	"strings"

	"WorkBaby/backend/db"
	"WorkBaby/backend/knowledge"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/singleinstance"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// version 是应用版本，与 wails.json 的 productVersion 及 git tag 保持一致。
const version = "1.3.0"

//go:embed all:frontend/dist
var assets embed.FS

// mcpKnowledgeFlag 让同一个 exe 充当知识库的 stdio MCP server：
// PI 在 mcp.json 里以「命令 = 本 exe，参数 = --mcp-knowledge」拉起它。
const mcpKnowledgeFlag = "--mcp-knowledge"

func main() {
	// MCP 模式在单实例检查之前：它是被内核当工具拉起来的子进程，
	// 不参与「唤起已有窗口」那套转交逻辑，也不能碰实例锁。
	if hasArg(mcpKnowledgeFlag) {
		serveKnowledgeMCP()
		return
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
func hasArg(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

// serveKnowledgeMCP 以 MCP 模式跑完一次会话就退出：不开窗口、不起 HTTP、不抢单实例锁。
// 它读的是同一个 SQLite 库（WAL 允许多进程读），所以资料在界面里加进来，助手这边立刻能查。
func serveKnowledgeMCP() {
	paths, err := runtime.Resolve()
	if err != nil {
		failMCP(err)
		return
	}
	gdb, err := db.Open(paths.DBPath)
	if err != nil {
		failMCP(err)
		return
	}
	r := repo.New(gdb)
	defer r.Close()

	svc := knowledge.New(r)
	// 诊断只走 stderr：stdout 是 JSON-RPC 通道，多写一行就破坏协议帧。
	if err := knowledge.NewMCPServer(svc, os.Stdin, os.Stdout).Serve(); err != nil && !errors.Is(err, io.EOF) {
		failMCP(err)
	}
}

func failMCP(err error) {
	_, _ = io.WriteString(os.Stderr, "workbaby-knowledge: "+err.Error()+"\n")
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
