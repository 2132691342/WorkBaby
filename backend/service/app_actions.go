package service

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Bootstrap 组装首屏总包：界面一次请求拿到画首屏要用的全部事实。
func (a *App) Bootstrap() domain.BootBootstrapRESP {
	settings := a.Settings.All()
	return domain.BootBootstrapRESP{
		AppVersion:   a.Version,
		AppContract:  ContractVersion,
		Engines:      []domain.EngineIdentity{a.EngineIdentity()},
		ActiveEngine: domain.EnginePi,
		Workspace:    a.Settings.Workspace(),
		Settings:     settings,
		Layouts:      []domain.EngineLayoutVO{EngineLayout(a.Layout)},
		Workspaces:   a.Settings.Workspaces(),
		Runtime:      a.RuntimeStatus(),
		Prefs:        a.Settings.Prefs(),
	}
}

// ContractVersion 是响应契约版本：字段形状一变就要 +1，界面据此识别过期。
const ContractVersion = 1

// EngineIdentity 当前引擎的身份事实。
func (a *App) EngineIdentity() domain.EngineIdentity {
	return domain.EngineIdentity{
		EngineKind:    domain.EnginePi,
		EngineLabel:   "pi",
		EngineExe:     a.Layout.PiEntry(),
		EngineVersion: a.Runtime.PiVersion(),
		Installed:     a.Runtime.PiInstalled(),
		Caps:          domain.PiCaps,
	}
}

// RuntimeStatus 内置运行时状态，额外带上「是否正在装配」。
func (a *App) RuntimeStatus() domain.RuntimeStatusVO {
	status := RuntimeStatus(a.Runtime)
	status.Provisioning = a.Provisioning()
	return status
}

// Health 概览页首屏要用的全部事实。
func (a *App) Health() domain.HealthRESP {
	instance := a.Chat.Instance()
	health := domain.EngineHealthVO{
		EngineKind:    domain.EnginePi,
		EngineLabel:   "pi",
		EngineExe:     a.Layout.PiEntry(),
		EngineVersion: a.Runtime.PiVersion(),
		Installed:     a.Runtime.PiInstalled(),
		Running:       instance.Running,
		Busy:          instance.Busy,
		ProcessId:     instance.Pid,
		StartedAt:     instance.StartedAt,
		CallCount:     instance.CallCount,
		LastError:     instance.LastError,
		SessionCount:  a.Sessions.Count(),
		SkillCount:    a.Skills.Count(),
		PluginCount:   a.Plugins.Count(),
		McpCount:      a.Mcp.Count(),
		ConfigFiles:   configFiles(a.Layout),
		Caps:          domain.PiCaps,
	}
	live := 0
	if instance.Running {
		live = 1
	}
	return domain.HealthRESP{
		HealthEngines: []domain.EngineHealthVO{health},
		ActiveEngine:  domain.EnginePi,
		Workspace:     a.Settings.Workspace(),
		Runtime:       a.RuntimeStatus(),
		TotalSessions: health.SessionCount,
		TotalSkills:   health.SkillCount,
		TotalPlugins:  health.PluginCount,
		TotalMcp:      health.McpCount,
		LiveEngines:   live,
		MemoryUsedMb:  memoryUsedMb(),
		DetectedAt:    pkg.NowMs(),
	}
}

// Apps 应用页的行：三个内置运行时 + pi 本体。
func (a *App) Apps() []domain.AppRowVO {
	layout := a.Layout
	piBusy := a.Plugins.Busy() || a.provisioningNow()
	return []domain.AppRowVO{
		{
			AppKey:      "node",
			AppName:     "内置 Node.js",
			AppVersion:  a.Runtime.NodeVersion(),
			AppPath:     layout.NodeExe(),
			Installed:   layout.NodeReady(),
			Builtin:     true,
			CanInstall:  false,
			InstallNote: "随安装包分发，首启自动解压",
		},
		{
			AppKey:      "git",
			AppName:     "内置 PortableGit",
			AppVersion:  a.Runtime.GitVersion(),
			AppPath:     layout.GitExe(),
			Installed:   layout.GitReady(),
			Builtin:     true,
			CanInstall:  false,
			InstallNote: "随安装包分发，首启自动解压",
		},
		{
			AppKey:      "python",
			AppName:     "内置 Python",
			AppVersion:  a.Runtime.PythonVersion(),
			AppPath:     layout.PythonExe(),
			Installed:   layout.PythonReady(),
			Builtin:     true,
			CanInstall:  false,
			InstallNote: "随安装包分发，首启自动解压",
		},
		{
			AppKey:       "pi",
			AppName:      "pi 引擎",
			AppVersion:   a.Runtime.PiVersion(),
			AppPath:      layout.PiEntry(),
			Installed:    a.Runtime.PiInstalled(),
			Builtin:      true,
			CanInstall:   !a.Runtime.PiInstalled() && layout.NodeReady(),
			CanUpdate:    a.Runtime.PiInstalled(),
			CanUninstall: a.Runtime.PiInstalled(),
			Busy:         piBusy,
			Blocker:      piBlocker(layout.NodeReady(), a.Runtime.PiInstalled()),
			InstallNote:  "用内置 npm 装进沙箱，不碰系统 node",
		},
	}
}

// piBlocker 给出 pi 不能操作的原因；空表示没阻碍。
func piBlocker(nodeReady, installed bool) string {
	if !nodeReady {
		return "内置 Node.js 还没就绪，稍等片刻"
	}
	if installed {
		return ""
	}
	return ""
}

// provisioningNow 当前是否有后台装配在跑。
func (a *App) provisioningNow() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provisioning || a.installBusy != ""
}

// lockInstall 占住 pi 的安装位。
func (a *App) lockInstall(action string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.installBusy != "" {
		return pkg.Err(pkg.CodeProvision, "有安装任务正在进行，稍后再试")
	}
	a.installBusy = action
	return nil
}

// unlockInstall 释放安装位。
func (a *App) unlockInstall() {
	a.mu.Lock()
	a.installBusy = ""
	a.mu.Unlock()
}

// InstallPi 用内置 npm 装 pi 本体，进度经 SSE 上报。
func (a *App) InstallPi(action string) error {
	if err := a.lockInstall(action); err != nil {
		return err
	}
	defer a.unlockInstall()
	a.ProvisionProgress("pi", action, "start", "正在安装 pi 引擎")
	res, err := a.Runtime.PiInstall(func(line string) {
		a.ProvisionProgress("pi", action, "line", line)
	})
	if err != nil {
		message := pkg.As(err).Message
		a.ProvisionProgress("pi", action, "fail", message)
		return err
	}
	if res.Code != 0 {
		message := "npm 退出码非零"
		a.ProvisionProgress("pi", action, "fail", message)
		return pkg.Err(pkg.CodeProvision, "pi 安装失败", pkg.Truncate(res.Stderr, 2000))
	}
	a.ProvisionProgress("pi", action, "done", "pi "+a.Runtime.PiVersion()+" 就绪")
	// 装完立刻补 npmCommand 与内置插件声明：顺序反了会让首启的插件装到系统 npm 上。
	EnsureNpmCommand(a.Layout, a.Runtime.NpmCommand())
	a.Plugins.DeclareBuiltins()
	a.Hub.Broadcast("engine:status", nil)
	return nil
}

// UpdatePi 升级 pi：先停引擎、清包目录再装一遍，避免 npm 复用残留。
func (a *App) UpdatePi() error {
	if !a.Runtime.PiInstalled() {
		return pkg.Err(pkg.CodeProvision, "pi 还没装，先点安装")
	}
	if a.Chat.Stream.Active() {
		return pkg.Err(pkg.CodeProvision, "引擎正在执行任务，等这一轮结束再升级")
	}
	// 升级前先停引擎：Restart 内部等引擎真退出（Stop 有等待语义），锁文件就解了。
	a.Chat.Restart()
	// 清包目录：npm 见到同名包会复用残留文件，不清可能升不干净。
	if err := os.RemoveAll(a.Layout.PiPkgDir()); err != nil {
		return pkg.WrapErr(pkg.CodeProvision, "pi 包目录清理失败", err)
	}
	return a.InstallPi("update")
}

// UninstallPi 卸载 pi：先停引擎再删包目录。
func (a *App) UninstallPi() error {
	if err := a.lockInstall("uninstall"); err != nil {
		return err
	}
	defer a.unlockInstall()
	a.Chat.Restart()
	a.ProvisionProgress("pi", "uninstall", "start", "正在卸载 pi 引擎")
	if err := a.Runtime.PiUninstall(); err != nil {
		a.ProvisionProgress("pi", "uninstall", "fail", pkg.As(err).Message)
		return err
	}
	a.ProvisionProgress("pi", "uninstall", "done", "pi 已卸载")
	a.Hub.Broadcast("engine:status", nil)
	return nil
}

// RestartEngine 重启引擎子进程。
func (a *App) RestartEngine() { a.Chat.Restart() }

// memoryUsedMb 本进程已用内存（MB）。取不到给 0。
func memoryUsedMb() int {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int(stats.Alloc / (1 << 20))
}

// killAllOnExit 退出时兜底杀引擎进程树。
func killAllOnExit(pid int) {
	if pid <= 0 {
		return
	}
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Run()
}
