package service

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// Provision 是整个应用的后台预热：清历史残留 → 探测引擎 → 拉起引擎。
// 它不下载、不安装、不改用户的 pi 配置——环境是用户的，壳只负责用。
func (a *App) Provision() {
	a.setProvisioning(true)
	defer func() {
		a.setProvisioning(false)
		a.Hub.Broadcast("engine:status", nil)
	}()

	// 先清历史遗留的 SQLite 文件：升级上来的机器上它们还在，却没有任何代码读写它们。
	// 放在装配开头而不是启动同步路径上：删文件不该拖慢首屏。
	repo.CleanupLegacyDB(a.Layout.DataDir, a.Layout.StorePath)
	// 再撤掉 settings.json 里残留的 npmCommand：它指向已不存在的内置 npm，
	// 留着会让 pi 装包时直接失败，而失败点离原因很远。
	EnsureNpmCommand(a.Layout)

	if !a.Runtime.Installed() {
		a.Hub.Broadcast("chat:notify", map[string]any{
			"level":   "warn",
			"message": "没有检测到 pi。请先在你的终端里装好 pi，再在应用页点重新检测。",
		})
		return
	}
	pkg.Info("pi 入口=%s 来源=%s 版本=%s",
		a.Runtime.Exe(), a.Runtime.Resolve().Source, a.Runtime.PiVersion())

	// 装配收尾就把引擎预热起来：用户点开对话页时插件已加载完，
	// 而不是从那一刻才开始等十几个插件的冷启动。
	a.Chat.Warm()
}

// Logs 当日日志尾部。
func (a *App) Logs(maxLines int) []string {
	if maxLines <= 0 || maxLines > 4000 {
		maxLines = 500
	}
	return pkg.TailLog(maxLines)
}

// SaveSettings 保存设置；工作区与 pi 路径变化会顺带重建引擎绑定。
func (a *App) SaveSettings(kv map[string]string) (map[string]string, error) {
	previous := a.Settings.Workspace()
	previousPi := a.Settings.All()[KeyPiPath]
	saved, err := a.Settings.Save(kv)
	if err != nil {
		return nil, err
	}
	// 工作区变了引擎绑定就失效。
	if next := saved[KeyWorkspace]; next != previous {
		a.Chat.OnWorkspaceChanged()
	}
	// pi 换了个入口等于换了个引擎：旧子进程还在跑旧的可执行文件。
	if next := saved[KeyPiPath]; next != previousPi {
		a.Chat.Restart()
	}
	return saved, nil
}

// DetectTargets 重新探测外部依赖与 pi 的版本。
func (a *App) DetectTargets() domain.RuntimeStatusVO {
	status := a.RuntimeStatus()
	a.Hub.Broadcast("engine:detected", map[string]any{
		"node_version": status.NodeVersion,
		"git_version":  status.GitVersion,
		"pi_version":   status.PiVersion,
		"pi_source":    status.PiSource,
	})
	// 刚指定好路径的用户最想要的是「能用了吗」：探测到就顺手把引擎拉起来。
	if status.PiReady {
		a.Chat.Warm()
	}
	return status
}
