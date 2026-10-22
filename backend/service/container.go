// 服务容器：一次性装配全部服务与能力域，是依赖就绪的唯一入口。
package service

import (
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// Container 是全部编排服务的组合根。
type Container struct {
	Env        *Env
	Sessions   *SessionService
	Chat       *ChatService
	Providers  *ProviderService
	Skills     *SkillService
	Extensions *ExtensionService
	Files      *FileService
	Tools      *ToolService
	Settings   *SettingsService
	Git        *GitService
	MCP        *MCPService
	Stats      *StatsService
	Engine     *EngineService
}

// New 装配全部服务；缺核心依赖即返回错误，不在运行期静默失效。
func New(env *Env) (*Container, error) {
	if err := env.MissingDeps(); err != nil {
		return nil, err
	}
	sessions := NewSessionService(env)
	providers := NewProviderService(env)
	chat := NewChatService(env, sessions, providers)
	env.Chat, env.Sessions, env.Providers = chat, sessions, providers

	// PI 监督器：进程配置由 PiEnv 提供，事件交给按工作区划分的翻译器。
	// 人设走 --append-system-prompt，不写用户的 APPEND_SYSTEM.md。
	sup := pi.NewSupervisor(
		func(workspace string) (pi.Config, error) {
			exe, args, err := env.PiEnv.PiCommand()
			if err != nil {
				return pi.Config{}, err
			}
			if env.PersonaPath != "" {
				// 人设参数排在 --mode 之前：pi.Config.Args 由客户端原样前置。
				args = append(args, "--append-system-prompt", env.PersonaPath)
			}
			return pi.Config{Exe: exe, Args: args, Env: env.PiEnv.Env()}, nil
		},
		func(workspace string) pi.Handler {
			return chat.translatorOfWorkspace(workspace)
		},
	)
	env.SetPi(sup)

	return &Container{
		Env:        env,
		Sessions:   sessions,
		Chat:       chat,
		Providers:  providers,
		Skills:     NewSkillService(env),
		Extensions: NewExtensionService(env),
		Files:      NewFileService(env),
		Tools:      NewToolService(env),
		Settings:   NewSettingsService(env, env.Paths),
		Git:        NewGitService(env),
		MCP:        NewMCPService(env),
		Stats:      NewStatsService(env),
		Engine:     NewEngineService(env),
	}, nil
}

// Bootstrap 首启兜底：写入默认设置、生成人设文件、扫一遍技能。
func (c *Container) Bootstrap() error {
	if err := c.Settings.SeedDefaults(); err != nil {
		return err
	}
	path, err := EnsurePersona(c.Env)
	if err != nil {
		pkg.Warnf("bootstrap: 生成人设文件失败: %v", err)
	}
	c.Env.PersonaPath = path
	c.Skills.Reload(c.Env.Workspace())
	return nil
}
