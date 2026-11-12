package service

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/sse"
)

// App 是业务装配根：所有 service 在这里造好并互相接线，api 层只拿它。
type App struct {
	// Version 壳版本
	Version string
	// Layout 沙箱布局
	Layout pkg.Layout
	// Store 设置库
	Store *repo.Store
	// Hub 事件广播
	Hub *sse.Hub
	// Runtime 引擎入口
	Runtime *engine.Runtime

	// Settings 设置
	Settings *Settings
	// Sessions 会话
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
	// Plugins 插件
	Plugins *Plugins
	// Mcp MCP
	Mcp *Mcp

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
	// Hub 事件广播
	Hub *sse.Hub
	// Version 壳版本
	Version string
}

// New 装配全部业务。
func New(opts Options) *App {
	rt := engine.New(opts.Layout, func() string {
		// 手动路径存在壳自己的存储里：engine 不认识存储层，由装配根注入读取方式。
		if value, ok := opts.Store.Get(KeyPiPath); ok {
			return value
		}
		return ""
	})
	settings := &Settings{Store: opts.Store, Layout: opts.Layout, Hub: opts.Hub}
	git := NewGit(settings, rt)
	// 工作区行上的分支与改动数来自 git，装配时回填一次就够。
	settings.Git = git
	sessions := NewSessions(settings, opts.Store, opts.Layout)
	chat := NewChat(settings, sessions, rt, opts.Hub)
	// 改名要避让引擎正在写的会话文件：绑定状态只有 Chat 知道。
	sessions.Active = chat.currentFile
	app := &App{
		Version:  opts.Version,
		Layout:   opts.Layout,
		Store:    opts.Store,
		Hub:      opts.Hub,
		Runtime:  rt,
		Settings: settings,
		Sessions: sessions,
		Chat:     chat,
		Files:    NewFiles(settings),
		Git:      git,
		Models:   NewModels(opts.Layout),
		Skills:   NewSkills(settings, opts.Store, opts.Layout, opts.Hub),
		Plugins:  NewPlugins(rt, opts.Hub, chat),
		Mcp:      NewMcp(settings, opts.Layout),
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

// SetWorkspace 切工作区，并让引擎绑定失效。
func (a *App) SetWorkspace(dir string) error {
	if _, err := a.Settings.SetWorkspace(dir); err != nil {
		return err
	}
	a.Chat.OnWorkspaceChanged()
	return nil
}

// Shutdown 退出收尾：停引擎子进程。
// 先请它自己退，等不到就整棵杀：只请不退的话，pi 与它的 bash 子孙会留在后台，
// 占着包目录的锁，下次启动就撞上「目录被占用」。
func (a *App) Shutdown() {
	a.Chat.Shutdown()
	a.Chat.KillEngine()
}

// OpenDataDir 打开数据目录。
func (a *App) OpenDataDir() error { return OpenDataDir(a.Layout) }

// setProvisioning 标记后台装配状态。
func (a *App) setProvisioning(running bool) {
	a.mu.Lock()
	a.provisioning = running
	a.mu.Unlock()
}

// Provisioning 是否有后台装配在跑。
func (a *App) Provisioning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provisioning
}
