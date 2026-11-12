package service

import (
	"runtime"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Bootstrap 组装首屏总包：界面一次请求拿到画首屏要用的全部事实。
func (a *App) Bootstrap() domain.BootBootstrapRESP {
	return domain.BootBootstrapRESP{
		AppVersion:   a.Version,
		AppContract:  ContractVersion,
		Engines:      []domain.EngineIdentity{a.EngineIdentity()},
		ActiveEngine: domain.EngineNative,
		Workspace:    a.Settings.Workspace(),
		Settings:     a.Settings.All(),
		Layouts:      []domain.EngineLayoutVO{EngineLayout(a.Layout)},
		Workspaces:   a.Settings.Workspaces(),
		Runtime:      a.RuntimeStatus(),
		Prefs:        a.Settings.Prefs(),
	}
}

// ContractVersion 是响应契约版本：字段形状一变就要 +1，界面据此识别过期。
const ContractVersion = 3

// EngineIdentity 当前引擎的身份事实。
// 引擎就是本进程：没有可执行入口、没有版本探测，装没装这件事不再存在。
func (a *App) EngineIdentity() domain.EngineIdentity {
	return domain.EngineIdentity{
		EngineKind:    domain.EngineNative,
		EngineLabel:   "WorkBaby 引擎",
		EngineVersion: a.Version,
		Installed:     true,
		Caps:          domain.NativeCaps,
	}
}

// RuntimeStatus 外部依赖的检测状态。
func (a *App) RuntimeStatus() domain.RuntimeStatusVO {
	status := RuntimeStatus()
	status.Detecting = a.Provisioning()
	return status
}

// Health 概览页首屏要用的全部事实。
func (a *App) Health() domain.HealthRESP {
	identity := a.EngineIdentity()
	health := domain.EngineHealthVO{
		EngineKind:    identity.EngineKind,
		EngineLabel:   identity.EngineLabel,
		EngineVersion: identity.EngineVersion,
		Installed:     true,
		Running:       true,
		Busy:          a.Chat.Busy(),
		SessionCount:  a.Sessions.Count(),
		SkillCount:    a.Skills.Count(),
		McpCount:      a.Mcp.Count(),
		ConfigFiles:   configFiles(a.Layout),
		Caps:          domain.NativeCaps,
	}
	return domain.HealthRESP{
		HealthEngines: []domain.EngineHealthVO{health},
		ActiveEngine:  domain.EngineNative,
		Workspace:     a.Settings.Workspace(),
		Runtime:       a.RuntimeStatus(),
		TotalSessions: health.SessionCount,
		TotalSkills:   health.SkillCount,
		TotalMcp:      health.McpCount,
		MemoryUsedMb:  memoryUsedMb(),
		DetectedAt:    pkg.NowMs(),
	}
}

// Apps 应用页的行：只有用户环境里的工具链。
// 引擎不再是一行外部依赖——它在进程里跑，没有「装没装」这件事。
func (a *App) Apps() []domain.AppRowVO {
	return []domain.AppRowVO{
		{
			AppKey:      "git",
			AppName:     "Git",
			AppVersion:  gitVersion(),
			AppPath:     pkg.LookPath("git"),
			Installed:   pkg.LookPath("git") != "",
			AppSource:   "path",
			InstallNote: "工作区的版本控制面板依赖它；不装也不影响对话与工具",
		},
		{
			AppKey:      "node",
			AppName:     "Node.js",
			AppVersion:  progVersion("node", "--version"),
			AppPath:     pkg.LookPath("node"),
			Installed:   pkg.LookPath("node") != "",
			AppSource:   "path",
			InstallNote: "只是部分项目的构建依赖；本应用自己不需要它",
		},
		{
			AppKey:      "python",
			AppName:     "Python",
			AppVersion:  progVersion("python", "--version"),
			AppPath:     pkg.LookPath("python"),
			Installed:   pkg.LookPath("python") != "",
			AppSource:   "path",
			InstallNote: "只是部分项目的运行依赖；本应用自己不需要它",
		},
	}
}

// progVersion 读一个外部程序的版本号；拿不到给空串。
func progVersion(name string, flag string) string {
	exe := pkg.LookPath(name)
	if exe == "" {
		return ""
	}
	res, err := pkg.RunProc(exe, []string{flag}, pkg.ProcOptions{TimeoutMs: 5_000})
	if err != nil || res.Code != 0 {
		return ""
	}
	return pkg.Truncate(trimVersion(res.Stdout), 40)
}

// trimVersion 去掉版本输出里的前缀与换行。
func trimVersion(text string) string {
	out := ""
	for _, r := range text {
		if r == '\n' || r == '\r' {
			break
		}
		out += string(r)
	}
	for _, prefix := range []string{"v", "Python ", "node "} {
		if len(out) >= len(prefix) && out[:len(prefix)] == prefix {
			out = out[len(prefix):]
		}
	}
	return out
}

// memoryUsedMb 本进程已用内存（MB）。取不到给 0。
func memoryUsedMb() int {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int(stats.Alloc / (1 << 20))
}
