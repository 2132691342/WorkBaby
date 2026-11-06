package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// bootstrap 首屏总包。
func (s *Server) bootstrap(c *gin.Context) {
	pkg.OK(c, s.svc.Bootstrap())
}

// settingsGet 读全部设置。
func (s *Server) settingsGet(c *gin.Context) {
	pkg.OK(c, s.svc.Settings.All())
}

// settingsSave 保存设置。
func (s *Server) settingsSave(c *gin.Context) {
	var req domain.SaveSettingsREQ
	if !bind(c, &req) {
		return
	}
	saved, err := s.svc.SaveSettings(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, saved)
}

// settingsWorkspace 切工作区。
func (s *Server) settingsWorkspace(c *gin.Context) {
	var req domain.SetWorkspaceREQ
	if !bind(c, &req) {
		return
	}
	workspace, err := s.svc.Settings.SetWorkspace(req.Workspace)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	// 工作区变了引擎绑定就失效：这里是唯一需要重建的入口。
	s.svc.Chat.OnWorkspaceChanged()
	pkg.OK(c, domain.SetWorkspaceRESP{Workspace: workspace})
}

// workspaceForget 从历史里去掉一个目录。
func (s *Server) workspaceForget(c *gin.Context) {
	var req domain.WorkspaceForgetREQ
	if !bind(c, &req) {
		return
	}
	rows, err := s.svc.Settings.ForgetWorkspace(req.WorkspacePath)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, domain.WorkspaceListRESP{
		Workspace:     s.svc.Workspace(),
		WorkspaceRows: rows,
		ActiveEngine:  domain.EnginePi,
	})
}

// workspacesGet 工作区清单：当前工作区 + 历史（带仓库与会话概况）。
func (s *Server) workspacesGet(c *gin.Context) {
	pkg.OK(c, s.svc.Settings.WorkspaceList())
}

// prefsGet 应用偏好。
func (s *Server) prefsGet(c *gin.Context) {
	pkg.OK(c, s.svc.Settings.Prefs())
}

// prefsAutoStart 开关开机自启。
func (s *Server) prefsAutoStart(c *gin.Context) {
	var req domain.PrefsToggleREQ
	if !bind(c, &req) {
		return
	}
	exePath, err := currentExePath()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	if err := service.SetAutoStart(req.PrefEnabled, exePath); err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, s.svc.Settings.Prefs())
}

// prefsRestore 「启动时恢复上次会话」开关。
func (s *Server) prefsRestore(c *gin.Context) {
	var req domain.PrefsToggleREQ
	if !bind(c, &req) {
		return
	}
	prefs, err := s.svc.Settings.SetRestoreSession(req.PrefEnabled)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, prefs)
}

// prefsCloseAction 关窗行为。
func (s *Server) prefsCloseAction(c *gin.Context) {
	var req domain.CloseActionREQ
	if !bind(c, &req) {
		return
	}
	prefs, err := s.svc.Settings.SetCloseAction(req.CloseAction)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, prefs)
}

// prefsOpenDataDir 打开数据目录。
func (s *Server) prefsOpenDataDir(c *gin.Context) {
	if err := s.svc.OpenDataDir(); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// logs 当日日志尾部。
func (s *Server) logs(c *gin.Context) {
	lines := s.svc.Logs(queryInt(c, "lines", 500))
	pkg.OK(c, map[string]any{"lines": lines})
}

// enginesGet 引擎身份 + 布局 + 存活实例。
func (s *Server) enginesGet(c *gin.Context) {
	instance := s.svc.Chat.Instance()
	pkg.OK(c, map[string]any{
		"engines":     []domain.EngineIdentity{s.svc.EngineIdentity()},
		"layouts":     []domain.EngineLayoutVO{service.EngineLayout(s.svc.Layout)},
		"runtime":     s.svc.RuntimeStatus(),
		"instance":    instance,
		"detected_at": pkg.NowMs(),
	})
}

// enginesDetect 重新探测运行时版本。
func (s *Server) enginesDetect(c *gin.Context) {
	pkg.OK(c, s.svc.DetectTargets())
}

// enginesRestart 重启引擎子进程。
func (s *Server) enginesRestart(c *gin.Context) {
	s.svc.RestartEngine()
	ok(c)
}

// enginesRepair 强制重解压三个内置运行时并重钉 npmCommand。
func (s *Server) enginesRepair(c *gin.Context) {
	problems, err := s.svc.RepairRuntime()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, domain.RuntimeRepairRESP{RepairProblems: problems})
}

// healthGet 概览页的健康快照。
func (s *Server) healthGet(c *gin.Context) {
	pkg.OK(c, s.svc.Health())
}

// appsGet 应用页的行。
func (s *Server) appsGet(c *gin.Context) {
	pkg.OK(c, map[string]any{"apps": s.svc.Apps()})
}

// appsInstall 安装 pi 本体。
func (s *Server) appsInstall(c *gin.Context) {
	if err := s.svc.InstallPi("install"); err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"apps": s.svc.Apps()})
}

// appsUpdate 升级 pi 本体。
func (s *Server) appsUpdate(c *gin.Context) {
	if err := s.svc.UpdatePi(); err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"apps": s.svc.Apps()})
}

// appsUninstall 卸载 pi 本体。
func (s *Server) appsUninstall(c *gin.Context) {
	if err := s.svc.UninstallPi(); err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"apps": s.svc.Apps()})
}

// configFiles 可查看的配置文件清单。
func (s *Server) configFiles(c *gin.Context) {
	pkg.OK(c, map[string]any{"entries": s.svc.ConfigFiles()})
}

// configText 配置文件原文。
func (s *Server) configText(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	out, err := s.svc.ConfigText(name)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// systemReveal 在资源管理器里定位路径。
func (s *Server) systemReveal(c *gin.Context) {
	var req domain.OpenTargetREQ
	if !bind(c, &req) {
		return
	}
	if err := service.Reveal(req.TargetPath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// systemOpen 用系统默认程序打开路径。
func (s *Server) systemOpen(c *gin.Context) {
	var req domain.OpenTargetREQ
	if !bind(c, &req) {
		return
	}
	if err := service.OpenPath(req.TargetPath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}
