package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Provision 首启的后台装配：解压运行时 → 写 npmCommand → 落技能 → 声明插件 → 装插件。
// 全部在后台跑：用户先看到能点的窗口加进度条，而不是 115MB 解压期间没有响应的黑屏。
func (a *App) Provision() {
	a.setProvisioning(true)
	defer func() {
		a.setProvisioning(false)
		a.Hub.Broadcast("engine:status", nil)
	}()

	problems := a.Runtime.ProvisionRuntime(func(phase, message string) {
		a.ProvisionProgress("runtime", "install", phase, message)
	})
	a.mu.Lock()
	a.runtimeProblems = problems
	a.mu.Unlock()

	// npmCommand 必须在任何 npm 动作之前写好：顺序反了，引擎接下来的安装会走系统 npm。
	EnsureNpmCommand(a.Layout, a.Runtime.NpmCommand())
	a.Skills.Provision()
	a.Plugins.DeclareBuiltins()

	if !a.Runtime.PiInstalled() {
		// 首次启动：自动装 pi 本体。失败留给应用页的手动重试，
		// 不自动重试——离线机器上反复重试只会刷一屏同样的错误。
		if a.Layout.NodeReady() {
			if err := a.InstallPi("install"); err != nil {
				pkg.Warn("首启自动安装 pi 失败：%v", err)
			}
		}
	}
	// 无论刚装完还是早装好，内置插件都得补一遍：
	// 装 pi 与装插件是两个步骤，跳过任何一步都会让用户拿到一个空引擎。
	if a.Runtime.PiInstalled() {
		a.Plugins.ProvisionBuiltins()
	}
}

// RuntimeProblems 取运行时装配时遇到的问题（诊断面板用）。
func (a *App) RuntimeProblems() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.runtimeProblems...)
}

// RepairRuntime 修复运行时：停引擎 → 强制重解压三个运行时 → 重钉 npmCommand。
// 与首启装配共用进度上报与 provisioning 标记，界面上看到的是同一条进度线。
func (a *App) RepairRuntime() ([]string, error) {
	if a.Provisioning() {
		return nil, pkg.Err(pkg.CodeEngineBusy, "已有装配任务在跑，等它结束再修复")
	}
	if a.Chat.Stream.Active() {
		return nil, pkg.Err(pkg.CodeEngineBusy, "引擎正在执行任务，等这一轮结束再修复")
	}
	a.setProvisioning(true)
	defer func() {
		a.setProvisioning(false)
		a.Hub.Broadcast("engine:status", nil)
	}()
	// Restart 内部等引擎真退出再返回，运行时目录上的文件锁就解了。
	a.Chat.Restart()
	problems := a.Runtime.RepairRuntime(func(phase, message string) {
		a.ProvisionProgress("runtime", "repair", phase, message)
	})
	a.mu.Lock()
	a.runtimeProblems = problems
	a.mu.Unlock()
	EnsureNpmCommand(a.Layout, a.Runtime.NpmCommand())
	return problems, nil
}

// Logs 当日日志尾部。
func (a *App) Logs(maxLines int) []string {
	if maxLines <= 0 || maxLines > 4000 {
		maxLines = 500
	}
	return pkg.TailLog(maxLines)
}

// SaveSettings 保存设置；工作区变化会顺带重建引擎绑定。
func (a *App) SaveSettings(kv map[string]string) (map[string]string, error) {
	previous := a.Settings.Workspace()
	saved, err := a.Settings.Save(kv)
	if err != nil {
		return nil, err
	}
	if next := saved[KeyWorkspace]; next != previous {
		a.Chat.OnWorkspaceChanged()
	}
	return saved, nil
}

// DetectTargets 重新探测内置运行时与 pi 的版本。
func (a *App) DetectTargets() domain.RuntimeStatusVO {
	status := a.RuntimeStatus()
	a.Hub.Broadcast("engine:detected", map[string]any{
		"node_version": status.NodeVersion,
		"pi_version":   status.PiVersion,
	})
	return status
}

// describeRuntime 拼一句运行时摘要（诊断面板用）。
func (a *App) describeRuntime() string {
	status := a.RuntimeStatus()
	parts := []string{
		"node=" + orDash(status.NodeVersion),
		"git=" + orDash(status.GitVersion),
		"python=" + orDash(status.PythonVersion),
		"pi=" + orDash(status.PiVersion),
	}
	return strings.Join(parts, " ")
}

// orDash 空值显示成破折号，避免界面上出现「node=」这种半截信息。
func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}
