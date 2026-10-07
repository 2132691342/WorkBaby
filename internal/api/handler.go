// Package api 是业务 handler 与系统能力绑定的所在；也是唯一允许 import wails runtime 的包。
package api

import (
	"context"
	"strings"

	"WorkBaby/internal/config"
	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/knowledge"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/service"
	"WorkBaby/internal/skill"
	"WorkBaby/internal/tool"
)

// Handler 持有全部业务服务与共享设施。
type Handler struct {
	Version   string
	ctx       context.Context
	Paths     runtime.Paths
	Cfg       *config.Config
	Repo      *repo.Repo
	Registry  *tool.Registry
	Skills    *skill.Registry
	Knowledge *knowledge.Service
	Emitter   *service.Emitter
	Svc       *service.Container
	Log       *pkg.Logger
	port      int
	startErr  string
}

// New 构造 handler；真正的资源装配在 Startup 里做。
func New(version string) *Handler { return &Handler{Version: version} }

// SetServerPort 记录本地 HTTP 端口。
func (h *Handler) SetServerPort(port int) { h.port = port }

// Port 返回已记录的本地 HTTP 端口，0 表示尚未启动。
func (h *Handler) Port() int { return h.port }

// ServerPort 返回本地 HTTP 端口。
// 前端在错过 app:ready 事件时靠它兜底：绑定方法是随时可调的，不存在事件竞态。
func (h *Handler) ServerPort() int { return h.port }

// SetStartupError 记录启动失败原因，供前端展示。
func (h *Handler) SetStartupError(err error) {
	if err == nil {
		return
	}
	h.startErr = err.Error()
	pkg.Errorf("startup: %v", err)
}

// StartupError 返回启动失败原因；空串表示一切正常。
func (h *Handler) StartupError() string { return h.startErr }

// Startup 是唯一装配入口，顺序即依赖顺序。
func (h *Handler) Startup(ctx context.Context) error {
	h.ctx = ctx
	paths, err := runtime.Resolve()
	if err != nil {
		return err
	}
	h.Paths = paths

	logger, err := pkg.InitLog(paths.LogDir)
	if err != nil {
		return err
	}
	h.Log = logger

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return err
	}
	h.Cfg = cfg
	if cfg.Workspace == "" {
		if home, herr := userHome(); herr == nil {
			cfg.Workspace = home
		}
	}

	gdb, err := db.Open(paths.DBPath)
	if err != nil {
		return err
	}
	h.Repo = repo.New(gdb)

	// 内置 Python：失败只告警不阻断启动，但必须把原因写清楚——
	// 只说「没有可用的 Python」，用户既不知道该放哪、也不知道是 LFS 没拉
	// 还是解压失败。
	if st := runtime.Status(paths); st.Exe == "" {
		pkg.Warnf("startup: 内置 Python 不可用：%s", st.Err)
	} else {
		pkg.Infof("startup: 内置 Python 已就绪（%s）", st.Version)
	}

	h.Registry = tool.New()
	if err := tool.RegisterBuiltins(h.Registry); err != nil {
		return err
	}
	if err := h.Registry.ValidateSchemas(); err != nil {
		return err
	}

	h.Knowledge = knowledge.New(h.Repo)
	toolDeps := service.NewToolDeps(paths, h.Knowledge)

	h.Skills = skill.New()
	for _, s := range skill.LoadFS(builtinSkills(), "skills", domain.SkillSourceBuiltin) {
		h.Skills.AddWithBody(s.Skill, s.Body)
	}
	for _, s := range skill.LoadDir(paths.SkillsDir, domain.SkillSourceGlobal) {
		h.Skills.Add(s)
	}
	if cfg.Workspace != "" {
		ws := cfg.Workspace + "/.workbaby/skills"
		for _, s := range skill.LoadDir(ws, domain.SkillSourceWorkspace) {
			h.Skills.Add(s)
		}
	}

	h.Emitter = service.NewEmitter()
	env := &service.Env{
		Repo: h.Repo, Paths: paths, Cfg: cfg, Emitter: h.Emitter,
		Registry: h.Registry, Skills: h.Skills, Knowledge: h.Knowledge, ToolDeps: toolDeps,
	}
	svc, err := service.New(env)
	if err != nil {
		return err
	}
	h.Svc = svc
	if err := svc.Bootstrap(); err != nil {
		return err
	}
	// 停用名单要在默认值落库之后读，否则首启会把用户的选择冲掉。
	if v, err := h.Repo.GetSetting(domain.SettingDisabledSkills); err == nil && v != "" {
		h.Skills.ApplyDisabled(strings.Split(v, ","))
	}
	return nil
}

// Ctx 暴露 Wails 生命周期上下文，供系统能力绑定使用。
func (h *Handler) Ctx() context.Context { return h.ctx }

// Shutdown 释放资源。
func (h *Handler) Shutdown() {
	if h.Repo != nil {
		if err := h.Repo.Close(); err != nil {
			pkg.Warnf("shutdown: 关闭数据库失败: %v", err)
		}
	}
	if h.Log != nil {
		_ = h.Log.Close()
	}
}
