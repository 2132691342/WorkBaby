// 服务容器：一次性装配全部服务与能力域，是依赖就绪的唯一入口。
package service

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/knowledge"
	"WorkBaby/backend/pi"
)

// Container 是全部编排服务的组合根。
type Container struct {
	Env         *Env
	Sessions    *SessionService
	Chat        *ChatService
	Approvals   *ApprovalService
	Providers   *ProviderService
	Skills      *SkillService
	Files       *FileService
	Tools       *ToolService
	Knowledge   *knowledge.Service
	Settings    *SettingsService
	Git         *GitService
	MCP         *MCPService
	Runs        *RunsService
	Checkpoints *CheckpointService
	Subagents   *SubagentService
	Evals       *EvalService
	Archive     *ArchiveService
	History     *HistoryService
	ArchMap     *ArchMapService
	Memory      *MemoryService
}

// New 装配全部服务；缺核心依赖即返回错误，不在运行期静默失效。
func New(env *Env) (*Container, error) {
	if err := env.MissingDeps(); err != nil {
		return nil, err
	}
	sessions := NewSessionService(env)
	approvals := NewApprovalService(env)
	providers := NewProviderService(env)
	chat := NewChatService(env, sessions, providers)

	// PI 监督器：进程配置由 PiEnv 提供（注入的 PATH 与可执行路径），
	// 事件交给按工作区划分的翻译器。
	sup := pi.NewSupervisor(
		func(workspace string) (pi.Config, error) {
			exe, _ := env.PiEnv.PiExe()
			return pi.Config{Exe: exe, Env: env.PiEnv.Env()}, nil
		},
		func(workspace string) pi.Handler {
			return chat.translatorOfWorkspace(workspace)
		},
	)
	env.SetPi(sup)

	observer := NewObserver()
	runs := NewRunsService(env)
	checkpoints := NewCheckpointService(env)
	evals := NewEvalService(env)
	archive := NewArchiveService(env, chat)
	memory := NewMemoryService(env)
	subagents := NewSubagentService(env, sessions, chat, observer)

	env.Attach(func(e *Env) {
		e.Runs, e.Checkpoints, e.Evals, e.Archive = runs, checkpoints, evals, archive
		e.Memory, e.Approvals = memory, approvals
	})
	env.SetSessions(sessions)
	// 旁听钩子：运行观测靠它采时间线与错误原因。
	env.ChatTap = observer.Forward

	return &Container{
		Env:         env,
		Sessions:    sessions,
		Chat:        chat,
		Approvals:   approvals,
		Providers:   providers,
		Skills:      NewSkillService(env),
		Files:       NewFileService(env),
		Tools:       NewToolService(env),
		Knowledge:   env.Knowledge,
		Settings:    NewSettingsService(env, env.Paths),
		Git:         NewGitService(env),
		MCP:         NewMCPService(env),
		Runs:        runs,
		Checkpoints: checkpoints,
		Subagents:   subagents,
		Evals:       evals,
		Archive:     archive,
		History:     NewHistoryService(env, sessions, chat),
		ArchMap:     NewArchMapService(env),
		Memory:      memory,
	}, nil
}

// Bootstrap 首启兜底：收口上次运行的残留，写入默认设置，同步 PI 配置。
func (c *Container) Bootstrap() ([]domain.ApprovalVO, error) {
	expired, err := c.Approvals.ExpireStale()
	if err != nil {
		return nil, err
	}
	// 收口上次残留的「执行中」运行记录：进程一退就没人收尾。
	if err := c.Runs.ReapStale(); err != nil {
		return nil, err
	}
	if err := c.Settings.MigrateLegacy(); err != nil {
		return nil, err
	}
	if err := c.Settings.SeedDefaults(); err != nil {
		return nil, err
	}
	// 把界面里的配置同步进 PI 的文件：每次启动都跑，幂等。
	c.SyncPiConfig()
	return expired, nil
}

// SyncPiConfig 把 WorkBaby 的配置写进 PI 能读的位置。
func (c *Container) SyncPiConfig() {
	if c.Env.PiEnv == nil {
		return
	}
	if err := c.Providers.SyncToPi(); err != nil {
		pkgWarn("bootstrap: 同步 models.json 失败", err)
	}
	if err := c.Settings.SyncToPi(); err != nil {
		pkgWarn("bootstrap: 同步 settings.json 失败", err)
	}
	// 记忆与技能每轮随提示带上：注入走 APPEND_SYSTEM.md 的受控标记块。
	if err := SyncAppendSystem(c.Env); err != nil {
		pkgWarn("bootstrap: 同步 APPEND_SYSTEM.md 失败", err)
	}
	// 知识库通过 MCP 暴露给 PI 的工具；它由程序维护，用户不能删。
	if err := c.Env.PiEnv.EnsureKnowledgeMCP(selfExe()); err != nil {
		pkgWarn("bootstrap: 写知识库 MCP 条目失败", err)
	}
	// 审批闸门与记忆写入工具都装在同一个扩展里。
	if err := ensureApprovalExtension(c.Env); err != nil {
		pkgWarn("bootstrap: 写审批扩展失败", err)
	}
}
