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
	// Runtime 内置运行时
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
	// provisioning 是否有后台装配正在跑
	provisioning bool
	// installBusy 哪个安装任务占着位
	installBusy string
	// runtimeProblems 运行时装配时遇到的问题
	runtimeProblems []string
}

// Options 是装配入参。
type Options struct {
	// Layout 沙箱布局
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
	rt := engine.New(opts.Layout)
	settings := &Settings{Store: opts.Store, Layout: opts.Layout, Hub: opts.Hub}
	sessions := NewSessions(settings, opts.Store, opts.Layout)
	chat := NewChat(settings, sessions, rt, opts.Hub)
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
		Git:      NewGit(settings, rt),
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
func (a *App) Shutdown() { a.Chat.Shutdown() }

// OpenDataDir 打开数据目录。
func (a *App) OpenDataDir() error { return OpenDataDir(a.Layout) }

// ProvisionProgress 发一帧装配进度。
func (a *App) ProvisionProgress(kind, action, phase, message string) {
	a.Hub.Broadcast("app:progress", sse.EngineTag(map[string]any{
		"progress_kind":    kind,
		"progress_action":  action,
		"progress_phase":   phase,
		"progress_message": message,
	}))
}

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
