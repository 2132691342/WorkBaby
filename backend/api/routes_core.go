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
	saved, err := s.svc.Settings.Save(req)
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
	if err := s.svc.SetWorkspace(req.Workspace); err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, domain.SetWorkspaceRESP{Workspace: s.svc.Workspace()})
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
		ActiveEngine:  domain.EngineNative,
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
	pkg.OK(c, map[string]any{"lines": pkg.TailLog(queryInt(c, "lines", 500))})
}

// enginesGet 引擎身份 + 布局 + 运行状态。
func (s *Server) enginesGet(c *gin.Context) {
	pkg.OK(c, map[string]any{
		"engines":     []domain.EngineIdentity{s.svc.EngineIdentity()},
		"layouts":     []domain.EngineLayoutVO{service.EngineLayout(s.svc.Layout)},
		"runtime":     s.svc.RuntimeStatus(),
		"instance":    s.svc.EngineInstance(),
		"detected_at": pkg.NowMs(),
	})
}

// enginesDetect 重新探测运行时（内置引擎恒就绪，这里只是刷新工具链状态）。
func (s *Server) enginesDetect(c *gin.Context) {
	s.svc.Redetect()
	pkg.OK(c, map[string]any{"runtime": s.svc.RuntimeStatus(), "apps": s.svc.Apps()})
}

// enginesRestart 中止当前运行：内置引擎没有需要重启的进程。
func (s *Server) enginesRestart(c *gin.Context) {
	s.svc.Chat.Stop()
	s.svc.Hub.Broadcast("engine:ready", nil)
	ok(c)
}

// enginesRepair 重新探测全部外部依赖（等价于一次全面刷新）。
func (s *Server) enginesRepair(c *gin.Context) {
	s.svc.Redetect()
	pkg.OK(c, map[string]any{"runtime": s.svc.RuntimeStatus(), "apps": s.svc.Apps()})
}

// healthGet 概览页的健康快照。
func (s *Server) healthGet(c *gin.Context) {
	pkg.OK(c, s.svc.Health())
}

// appsGet 应用页的行。
func (s *Server) appsGet(c *gin.Context) {
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
