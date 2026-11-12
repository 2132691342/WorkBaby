package service

import (
	"runtime"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/engine"
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
const ContractVersion = 2

// EngineIdentity 当前引擎的身份事实。
func (a *App) EngineIdentity() domain.EngineIdentity {
	entry := a.Runtime.Resolve()
	return domain.EngineIdentity{
		EngineKind:    domain.EnginePi,
		EngineLabel:   "pi",
		EngineExe:     entry.Command,
		EngineScript:  entry.Script,
		EngineSource:  entry.Source,
		EngineVersion: a.Runtime.PiVersion(),
		Installed:     entry.Command != "",
		Caps:          domain.PiCaps,
	}
}

// RuntimeStatus 外部依赖的检测状态。
func (a *App) RuntimeStatus() domain.RuntimeStatusVO {
	status := RuntimeStatus(a.Runtime)
	status.Detecting = a.Provisioning()
	return status
}

// Health 概览页首屏要用的全部事实。
func (a *App) Health() domain.HealthRESP {
	instance := a.Chat.Instance()
	identity := a.EngineIdentity()
	health := domain.EngineHealthVO{
		EngineKind:    domain.EnginePi,
		EngineLabel:   "pi",
		EngineExe:     identity.EngineExe,
		EngineVersion: identity.EngineVersion,
		Installed:     identity.Installed,
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

// Apps 应用页的行：pi（硬依赖）与 node / git / python（能力加成）。
// 全部是「用户环境里有什么」，壳不代装任何一个。
func (a *App) Apps() []domain.AppRowVO {
	entry := a.Runtime.Resolve()
	settings := a.Settings.All()
	manual := strings.TrimSpace(settings[KeyPiPath])
	return []domain.AppRowVO{
		{
			AppKey:      "pi",
			AppName:     "pi 引擎",
			AppVersion:  a.Runtime.PiVersion(),
			AppPath:     piPathOf(entry),
			Installed:   entry.Command != "",
			Required:    true,
			AppSource:   entry.Source,
			CanLocate:   true,
			Blocker:     piBlocker(entry.Command != "", entry.Source, manual, entry.NodeMissing),
			InstallNote: piInstallNote(entry.NodeMissing),
		},
		{
			AppKey:      "node",
			AppName:     "Node.js",
			AppVersion:  a.Runtime.NodeVersion(),
			AppPath:     pkg.LookPath("node"),
			Installed:   pkg.LookPath("node") != "",
			AppSource:   "path",
			InstallNote: "pi 的启动脚本需要 node；从 nodejs.org 装一个即可",
		},
		{
			AppKey:      "git",
			AppName:     "Git",
			AppVersion:  a.Runtime.GitVersion(),
			AppPath:     pkg.LookPath("git"),
			Installed:   pkg.LookPath("git") != "",
			AppSource:   "path",
			InstallNote: "工作区的版本控制与 pi 的 bash 工具都依赖它",
		},
		{
			AppKey:      "python",
			AppName:     "Python",
			AppVersion:  a.Runtime.PythonVersion(),
			AppPath:     pkg.LookPath("python"),
			Installed:   pkg.LookPath("python") != "",
			AppSource:   "path",
			InstallNote: "部分技能要用它跑脚本；用不上可以不装",
		},
	}
}

// piBlocker 给出 pi 不可用的原因；空表示没阻碍。
// 「pi 在但 node 缺」必须单独说：那种情况下用户看到「没装 pi」会去重装一个
// 本来就装好的东西，而真正缺的那一环从头到尾没被提到。
func piBlocker(installed bool, source, manual string, needsNode bool) string {
	switch {
	case installed:
		return ""
	case needsNode:
		return "已找到 pi 的入口脚本，但机器上没有 node，它跑不起来"
	case manual != "":
		return "已指定的路径不可用：" + manual
	default:
		return "没有在 PATH 与 npm 全局目录里找到 pi"
	}
}

// piInstallNote 应用页上那一行说明：缺 node 与缺 pi 是两条不同的下一步。
func piInstallNote(needsNode bool) string {
	if needsNode {
		return "从 nodejs.org 装一个 Node.js（LTS 即可），装完重启本应用"
	}
	return "本应用不代装 pi：请先在你的终端里装好 pi，再回来点重新检测"
}

// piPathOf 界面上展示的 pi 位置：脚本形态给脚本本身，否则给可执行入口。
func piPathOf(entry engine.Entry) string {
	if entry.Script != "" {
		return entry.Script
	}
	if entry.ScriptPath != "" {
		return entry.ScriptPath
	}
	return entry.Command
}

// SetPiPath 写手动指定的 pi 路径；空串表示回到自动探测。
// 写盘后不自动拉起引擎：路径对不对要让用户点「重新检测」看到结果。
func (a *App) SetPiPath(path string) (domain.RuntimeStatusVO, error) {
	trimmed := strings.TrimSpace(path)
	// 用户选的是文件时取其所在目录的判定交给 engine：这里只确保路径存在性被校验。
	if trimmed != "" && !pkg.Exists(trimmed) {
		return domain.RuntimeStatusVO{}, pkg.Err(pkg.CodeFileNotFound, "路径不存在", trimmed)
	}
	if _, err := a.Settings.Save(map[string]string{KeyPiPath: trimmed}); err != nil {
		return domain.RuntimeStatusVO{}, err
	}
	a.Hub.Broadcast("engine:status", nil)
	return a.RuntimeStatus(), nil
}

// LocatePi 让用户手动指定 pi 的位置（壳调系统文件选择框后回填）。
func (a *App) LocatePi(path string) (domain.RuntimeStatusVO, error) {
	return a.SetPiPath(path)
}

// RestartEngine 重启引擎子进程。
func (a *App) RestartEngine() { a.Chat.Restart() }

// memoryUsedMb 本进程已用内存（MB）。取不到给 0。
func memoryUsedMb() int {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int(stats.Alloc / (1 << 20))
}
