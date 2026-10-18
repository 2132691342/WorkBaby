// Package service 是业务编排层：会话、对话、模型服务、技能、插件与设置。
// 它不感知 HTTP 与桌面壳，能力域全部经接口注入；Agent 内核是外部 PI 进程。
package service

import (
	"sync"

	"WorkBaby/backend/config"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/skill"
)

// Env 是各服务共享的运行环境，构造期一次性注入。
type Env struct {
	Repo    *repo.Repo
	Paths   runtime.Paths
	Cfg     *config.Config
	Emitter *Emitter
	Skills  *skill.Registry
	// PiEnv 是 PI 的运行环境（运行时复用 / PI 安装 / 配置互通）。
	PiEnv *pienv.Env
	// Pi 是 PI 进程监督器：一个工作区一个进程。
	Pi *pi.Supervisor
	// Chat 在容器装配期填入：会话服务要问它「这条会话在跑吗」。
	Chat     *ChatService
	Sessions *SessionService
	// PersonaPath 是附加系统提示文件，spawn PI 时用 --append-system-prompt 传入。
	PersonaPath string

	mu sync.RWMutex
}

// SetPi 注入 PI 监督器。
func (e *Env) SetPi(s *pi.Supervisor) {
	e.mu.Lock()
	e.Pi = s
	e.mu.Unlock()
}

// PiSupervisor 取 PI 监督器。
func (e *Env) PiSupervisor() *pi.Supervisor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Pi
}

// Attach 填入后续装配的服务句柄。
func (e *Env) Attach(fn func(*Env)) { e.mu.Lock(); fn(e); e.mu.Unlock() }

// Env 自检：缺核心依赖即启动失败，避免运行期静默失效。
func (e *Env) MissingDeps() error {
	switch {
	case e.Repo == nil:
		return pkg.New("2001", "数据仓储未装配", "")
	case e.Emitter == nil:
		return pkg.New("2001", "事件出口未装配", "")
	case e.Cfg == nil:
		return pkg.New("2001", "配置未装配", "")
	case e.PiEnv == nil:
		return pkg.New("2001", "PI 运行环境未装配", "")
	default:
		return nil
	}
}

// Workspace 返回当前的默认工作目录：配置优先，空则用用户主目录。
func (e *Env) Workspace() string {
	if e.Cfg != nil && e.Cfg.Workspace != "" {
		return e.Cfg.Workspace
	}
	if v, _ := e.Repo.GetSetting(domain.SettingWorkspace); v != "" {
		return v
	}
	return ""
}
