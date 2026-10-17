// Package api 是业务 handler 与系统能力绑定的所在；也是唯一允许 import wails runtime 的包。
// 它同时是组合根：Startup 是唯一的装配入口，顺序即依赖顺序。
package api

import (
	"context"
	"strings"
	"sync/atomic"

	"WorkBaby/backend/config"
	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/knowledge"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/service"
	"WorkBaby/backend/skill"
)

// Handler 持有全部业务服务与共享设施；port / startErr 跨 goroutine 并发读写，必须走原子量。
type Handler struct {
	Version   string
	ctx       context.Context
	Paths     runtime.Paths
	Cfg       *config.Config
	Repo      *repo.Repo
	Skills    *skill.Registry
	Knowledge *knowledge.Service
	PiEnv     *pienv.Env
	Emitter   *service.Emitter
	Svc       *service.Container
	Log       *pkg.Logger
	// expiredApprovals 是本次启动收口的「上次没处理完的确认」。
	expiredApprovals []domain.ApprovalVO
	port             atomic.Int64
	startErr         atomic.Pointer[string]
}

// New 构造 handler；真正的资源装配在 Startup 里做。
func New(version string) *Handler { return &Handler{Version: version} }

// SetServerPort 记录本地 HTTP 端口。
func (h *Handler) SetServerPort(port int) { h.port.Store(int64(port)) }

// ServerPort 返回本地 HTTP 端口，0 表示尚未启动。
func (h *Handler) ServerPort() int { return int(h.port.Load()) }

// SetStartupError 记录启动失败原因，供前端展示。
func (h *Handler) SetStartupError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	h.startErr.Store(&msg)
	pkg.Errorf("startup: %v", err)
}

// ExpiredApprovals 返回本次启动被按拒绝收口的审批清单；空表示没有残留。
func (h *Handler) ExpiredApprovals() []domain.ApprovalVO { return h.expiredApprovals }

// StartupError 返回启动失败原因；空串表示一切正常。
func (h *Handler) StartupError() string {
	if p := h.startErr.Load(); p != nil {
		return *p
	}
	return ""
}

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

	// 启动横幅：出问题时第一个要问的是「哪个版本、数据在哪」，先记下来。
	pkg.Infof("startup: WorkBaby %s 启动（数据目录 %s）", h.Version, paths.DataDir)

	// 内置运行时：失败只告警不阻断启动，但原因必须写清楚——
	// 只说「不可用」，用户无从判断是包没嵌进来、解压失败还是被安全软件拦了。
	reportRuntime(paths)

	// PI 的运行环境：内置 Node 优先，PI 的发现与安装都在这一层。
	h.PiEnv = pienv.New(paths)

	h.Knowledge = knowledge.New(h.Repo)

	h.Skills = skill.New()
	// 内置技能落盘再登记：PI 与模型都是按真实路径读正文的。
	skill.InstallBuiltin(h.Skills, builtinSkills(), "skills", paths.BuiltinSkillsDir)
	for _, s := range skill.LoadDir(paths.SkillsDir, domain.SkillSourceGlobal) {
		h.Skills.Add(s)
	}
	skill.LoadWorkspace(h.Skills, cfg.Workspace)

	h.Emitter = service.NewEmitter()
	env := &service.Env{
		Repo: h.Repo, Paths: paths, Cfg: cfg, Emitter: h.Emitter,
		Skills: h.Skills, Knowledge: h.Knowledge, PiEnv: h.PiEnv,
	}
	svc, err := service.New(env)
	if err != nil {
		return err
	}
	h.Svc = svc
	expired, err := svc.Bootstrap()
	if err != nil {
		return err
	}
	h.expiredApprovals = expired
	// 停用名单要在默认值落库之后读，否则首启会把用户的选择冲掉。
	if v, err := h.Repo.GetSetting(domain.SettingDisabledSkills); err == nil && v != "" {
		h.Skills.ApplyDisabled(strings.Split(v, ","))
	}
	return nil
}

// reportRuntime 解压并留痕三套内置运行时。
// 提前做是为了让失败点落在启动日志里，而不是某次工具调用时。
func reportRuntime(paths runtime.Paths) {
	if st := runtime.NodeStatusOf(paths); st.Exe == "" {
		pkg.Warnf("startup: 内置 Node 不可用（将回退系统 node）：%s", st.Err)
	} else {
		pkg.Infof("startup: 内置 Node 已就绪（%s）", st.Version)
	}
	if st := runtime.Status(paths); st.Exe == "" {
		pkg.Warnf("startup: 内置 Python 不可用：%s", st.Err)
	} else {
		pkg.Infof("startup: 内置 Python 已就绪（%s）", st.Version)
	}
	if st := runtime.NodeStatusOf(paths); st.Exe == "" {
		pkg.Warnf("startup: 内置 Node 不可用（将回退系统 node）：%s", st.Err)
	} else {
		pkg.Infof("startup: 内置 Node 已就绪（%s）", st.Version)
	}
}

// Ctx 暴露 Wails 生命周期上下文，供系统能力绑定使用。
func (h *Handler) Ctx() context.Context { return h.ctx }

// Shutdown 释放资源：先收 PI 进程，再关库。
// 顺序不能反：进程还活着时关库，PI 那边任何一次落盘都会失败。
func (h *Handler) Shutdown() {
	if h.Svc != nil && h.Svc.Env.PiSupervisor() != nil {
		h.Svc.Env.PiSupervisor().StopAll()
	}
	if h.Repo != nil {
		if err := h.Repo.Close(); err != nil {
			pkg.Warnf("shutdown: 关闭数据库失败: %v", err)
		}
	}
	if h.Log != nil {
		_ = h.Log.Close()
	}
}
