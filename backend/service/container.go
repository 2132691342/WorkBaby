// 服务容器：一次性装配全部服务与能力域，是依赖就绪的唯一入口。
package service

import (
	"WorkBaby/backend/pi"
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
}

// New 装配全部服务；缺核心依赖即返回错误，不在运行期静默失效。
func New(env *Env) (*Container, error) {
	if err := env.MissingDeps(); err != nil {
		return nil, err
	}
	sessions := NewSessionService(env)
	providers := NewProviderService(env)
	chat := NewChatService(env, sessions, providers)
	env.Chat, env.Sessions = chat, sessions

	// PI 监督器：进程配置由 PiEnv 提供（注入的 PATH 与可执行路径），
	// 事件交给按工作区划分的翻译器。人设走 --append-system-prompt，
	// 不写用户自己的 APPEND_SYSTEM.md：那是他的文件，不是我们的。
	sup := pi.NewSupervisor(
		func(workspace string) (pi.Config, error) {
			exe, _ := env.PiEnv.PiExe()
			cfg := pi.Config{Exe: exe, Env: env.PiEnv.Env()}
			if env.PersonaPath != "" {
				cfg.Args = []string{"--append-system-prompt", env.PersonaPath}
			}
			return cfg, nil
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
	}, nil
}

// Bootstrap 首启兜底：写入默认设置、生成人设文件、扫一遍技能。
func (c *Container) Bootstrap() error {
	if err := c.Settings.SeedDefaults(); err != nil {
		return err
	}
	path, err := EnsurePersona(c.Env)
	if err != nil {
		pkgWarn("bootstrap: 生成人设文件失败", err)
	}
	c.Env.PersonaPath = path
	c.Skills.Reload(c.Env.Workspace())
	return nil
}
