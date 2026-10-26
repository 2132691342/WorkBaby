// Package api 是业务 handler 与系统能力绑定所在；也是唯一允许 import wails runtime 的包。
// 它同时是组合根：Startup 是唯一的装配入口，顺序即依赖顺序。
package api

import (
	"context"
	"strings"
	"sync/atomic"

	"WorkBaby/backend/config"
	"WorkBaby/backend/db"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/service"
	"WorkBaby/backend/skill"
)

// Handler 持有全部业务服务与共享设施；port / startErr 跨 goroutine 并发读写，必须走原子量。
type Handler struct {
	Version string
	ctx     context.Context
	Paths   runtime.Paths
	Cfg     *config.Config
	Repo    *repo.Repo
	Skills  *skill.Registry
	// PiEnv 是 PI 的运行环境：运行时复用、PI 发现与配置互通都在它手里。
	PiEnv   *pienv.Env
	Emitter *service.Emitter
	Svc     *service.Container
	Log     *pkg.Logger

	port     atomic.Int64
	startErr atomic.Pointer[string]
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

	// 启动横幅：出问题时第一个要问的是「哪个版本、数据在哪、PI 的配置在哪」。
	pkg.Infof("startup: WorkBaby %s 启动（数据目录 %s，PI 配置目录 %s）", h.Version, paths.DataDir, paths.AgentDir)

	// 运行时：只读现状并按需解压，失败只告警不阻断启动，但原因必须写清楚。
	h.PiEnv = pienv.New(paths)
	reportRuntime(h.PiEnv)
	preparePiEnv(h.PiEnv)

	h.Skills = skill.New()
	h.Emitter = service.NewEmitter()
	env := &service.Env{
		Repo: h.Repo, Paths: paths, Cfg: cfg, Emitter: h.Emitter,
		Skills: h.Skills, PiEnv: h.PiEnv,
	}
	svc, err := service.New(env)
	if err != nil {
		return err
	}
	h.Svc = svc
	return svc.Bootstrap()
}

// preparePiEnv 在装了 PI 的机器上顺手修两件事：命令执行环境、终端入口。
// 都是幂等动作：配置已经对就什么都不写；用户只想用界面、不需要终端也不会受影响。
func preparePiEnv(e *pienv.Env) {
	if exe, _ := e.PiExe(); exe == "" {
		return
	}
	if st, err := e.EnsureShell(); err != nil {
		pkg.Warnf("startup: 修正命令执行环境失败: %v", err)
	} else if st.Applied {
		pkg.Infof("startup: 命令执行环境已改为 %s（%s）", st.Kind, st.Path)
	}
	st := e.TerminalStatusOf()
	if len(st.Others) > 0 {
		pkg.Warnf("startup: PATH 上还有其它 pi 入口（本应用入口优先）：%s", strings.Join(st.Others, "、"))
	}
	if st.ShadowedBy != "" {
		// 系统级 PATH 排在用户级前面，改不了：只能把冲突说清楚，让用户自己清掉旧入口。
		pkg.Warnf("startup: 终端里的 pi 会被 %s 抢先，请在系统环境变量里清掉那份旧安装", st.ShadowedBy)
		return
	}
	// 入口已存在或 PI 装在我们自己的前缀里时每次启动都刷：当前生效的 node / PI 漂移了入口必须跟着改。
	// pi 本来就在 PATH 上（用户自己的 npm 前缀）时不再生成一份——PATH 上不能有两个 pi。
	if pkg.FileExists(st.Shim) || st.OnPath || (e.OwnsInstall() && !e.PiOnPath()) {
		if _, err := e.EnsureTerminal(); err != nil {
			pkg.Warnf("startup: 生成终端入口失败: %v", err)
		}
	}
}

// reportRuntime 留痕这一轮选中的运行时：出问题时第一个要问的就是「用的哪份 node / git」。
func reportRuntime(e *pienv.Env) {
	tc := e.Toolchain()
	if tc.Node.Exe == "" {
		pkg.Warnf("startup: 没有可用的 Node（系统里没有合规版本，内置运行时也未就位）")
	} else {
		pkg.Infof("startup: Node %s（%s），Git %s",
			tc.Node.Version, pienv.ToolchainLabel(tc), firstNonEmptyString(tc.Git.Version, "未找到"))
	}
}

// firstNonEmptyString 取第一个非空值，只为日志兜底。
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
