package service

import (
	"path/filepath"
	"sync"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/sse"
)

// App 是业务装配根：所有 service 在这里造好并互相接线，api 层只拿它。
type App struct {
	// Version 应用版本
	Version string
	// Layout 目录布局
	Layout pkg.Layout
	// Store 设置库
	Store *repo.Store
	// DB SQLite 连接
	DB *repo.DB
	// Hub 事件广播
	Hub *sse.Hub

	// Settings 设置
	Settings *Settings
	// Sessions 会话回放
	Sessions *Sessions
	// Chat 对话
	Chat *Chat
	// Files 工作区文件
	Files *Files
	// Git git
	Git *Git
	// Models 模型与服务商
	Models *Models
	// Skills 技能
	Skills *Skills
	// Mcp MCP
	Mcp *Mcp
	// Web 联网能力
	Web *Web
	// Searcher 工作区预索引与检索
	Searcher *Searcher

	// mu 保护装配状态
	mu sync.Mutex
	// provisioning 是否有后台探测正在跑
	provisioning bool
}

// Options 是装配入参。
type Options struct {
	// Layout 目录布局
	Layout pkg.Layout
	// Store 设置库
	Store *repo.Store
	// DB SQLite 连接
	DB *repo.DB
	// Hub 事件广播
	Hub *sse.Hub
	// Version 应用版本
	Version string
}

// New 装配全部业务。
func New(opts Options) *App {
	settings := &Settings{Store: opts.Store, Layout: opts.Layout, Hub: opts.Hub}
	git := NewGit(settings)
	// 工作区行上的分支与改动数来自 git，装配时回填一次就够。
	settings.Git = git
	sessions := NewSessions(settings, opts.Store, opts.Layout)
	models := NewModels(opts.Layout, opts.DB)
	webKit := NewWeb(settings)
	searcher := NewSearcher(settings, opts.DB)
	sessionStore := agent.NewStore(filepath.Join(opts.Layout.DataDir, "sessions"), settings.Workspace)
	chat := NewChat(settings, sessions, models, opts.Hub, sessionStore)
	// 联网工具装在注册表上：模型能搜网与读网页的能力从这里来。
	chat.Tools.AddWebTools(webKit.Kit())
	// 预索引检索工具：模型能快速定位文件与符号。
	chat.Tools.AddIndexTool(searcher.Kit())
	// 改名要避让引擎正在写的会话文件：绑定状态只有 Chat 知道。
	sessions.Active = chat.CurrentFile
	app := &App{
		Version:  opts.Version,
		Layout:   opts.Layout,
		Store:    opts.Store,
		DB:       opts.DB,
		Hub:      opts.Hub,
		Settings: settings,
		Sessions: sessions,
		Chat:     chat,
		Files:    NewFiles(settings),
		Git:      git,
		Models:   models,
		Skills:   NewSkills(settings, opts.Store, opts.Layout, opts.Hub),
		Mcp:      NewMcp(settings, opts.Layout),
		Web:      webKit,
		Searcher: searcher,
	}
	return app
}

// Workspace 当前工作区。
func (a *App) Workspace() string { return a.Settings.Workspace() }

// CloseAction 关窗行为（shell 要读它决定收托盘还是退出）。
func (a *App) CloseAction() string { return a.Settings.CloseAction() }

// SetCloseAction 写关窗行为。
func (a *App) SetCloseAction(action string) error {
	_, err := a.Settings.SetCloseAction(action)
	return err
}

// MarkTrayHintShown 记住已经解释过「收进托盘」。
func (a *App) MarkTrayHintShown() { a.Settings.MarkTrayHintShown() }

// TrayHintPending 还没解释过收托盘。
func (a *App) TrayHintPending() bool { return a.Settings.TrayHintPending() }

// WorkspaceRows 工作区历史（托盘菜单要用）。
func (a *App) WorkspaceRows() []domain.WorkspaceRowVO { return a.Settings.Workspaces() }

// SetWorkspace 切工作区，并让会话绑定失效。
func (a *App) SetWorkspace(dir string) error {
	if _, err := a.Settings.SetWorkspace(dir); err != nil {
		return err
	}
	a.Chat.OnWorkspaceChanged()
	return nil
}

// Shutdown 退出收尾：中止还在跑的那一轮。
func (a *App) Shutdown() { a.Chat.Shutdown() }

// EngineInstance 是引擎运行实例的展示快照。
// 引擎在进程内，所以没有 PID 与启动时刻——只有「在不在忙」这一件事实。
func (a *App) EngineInstance() map[string]any {
	busy := a.Chat.Busy()
	return map[string]any{
		"engine_kind":  string(domain.EngineNative),
		"running":      true,
		"busy":         busy,
		"session_file": a.Chat.CurrentFile(),
		"work_dir":     a.Settings.Workspace(),
		"started_at":   pkg.NowMs(),
	}
}

// Redetect 重新探测外部工具链。内置引擎无需探测，这里只刷新缓存的版本号。
func (a *App) Redetect() {
	a.Hub.Broadcast("engine:detected", sse.EngineTag(map[string]any{
		"runtime": a.RuntimeStatus(),
	}))
}

// OpenDataDir 打开数据目录。
func (a *App) OpenDataDir() error { return OpenDataDir(a.Layout) }

// Provisioning 是否有后台探测在跑。
// 内置引擎没有外部探测，所以它恒为假——保留这个读数是为了让界面
// 的「正在检测」那一栏有统一的数据源。
func (a *App) Provisioning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provisioning
}
