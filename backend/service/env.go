// Package service 是业务编排层：会话、对话、审批、模型服务、技能与设置。
// 它不感知 HTTP 与桌面壳，能力域全部经接口注入；Agent 内核是外部 PI 进程。
package service

import (
	"sync"

	"WorkBaby/backend/config"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/knowledge"
	"WorkBaby/backend/pi"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/skill"
)

// Env 是各服务共享的运行环境，构造期一次性注入。
type Env struct {
	Repo      *repo.Repo
	Paths     runtime.Paths
	Cfg       *config.Config
	Emitter   *Emitter
	Skills    *skill.Registry
	Knowledge *knowledge.Service
	// PiEnv 是 PI 的运行环境（内置 Node / PI 安装 / 配置互通）。
	PiEnv *pienv.Env
	// Pi 是 PI 进程监督器：一个工作区一个进程。
	Pi *pi.Supervisor

	// mu 保护下面这些可变句柄：它们在装配期分步填入，
	// 而 chat 的 goroutine 可能在装配完成后立刻读取。
	mu          sync.RWMutex
	Runs        *RunsService
	Checkpoints *CheckpointService
	Evals       *EvalService
	Archive     *ArchiveService
	Memory      *MemoryService
	Approvals   *ApprovalService
	// sessions 在容器装配期填入：运行观测的分叉要复用它。
	sessions *SessionService
	// ChatTap 是对聊事件的旁听钩子（可选）：运行观测靠它采时间线与错误原因。
	ChatTap func(sessionID, event string, data any)
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

// Sessions 取会话服务（运行观测分叉时要它），在容器装配期填入。
func (e *Env) Sessions() *SessionService {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sessions
}

// SetSessions 注入会话服务句柄。
func (e *Env) SetSessions(s *SessionService) {
	e.mu.Lock()
	e.sessions = s
	e.mu.Unlock()
}

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

// PermissionOf 把会话权限档规整成合法取值。
func PermissionOf(v string) string {
	switch v {
	case domain.PermissionAuto, domain.PermissionReadonly:
		return v
	default:
		return domain.PermissionAskWrite
	}
}
