package service

import (
	"WorkBaby/internal/knowledge"
	"WorkBaby/internal/tool"
)

// Container 是全部编排服务的组合根，构造期一次性装配。
type Container struct {
	Env       *Env
	Sessions  *SessionService
	Chat      *ChatService
	Approvals *ApprovalService
	Providers *ProviderService
	Skills    *SkillService
	Tools     *ToolService
	Knowledge *knowledge.Service
	Settings  *SettingsService
}

// New 装配全部服务；缺核心依赖即返回错误，不在运行期静默失效。
func New(env *Env) (*Container, error) {
	if err := env.MissingDeps(); err != nil {
		return nil, err
	}
	sessions := NewSessionService(env)
	approvals := NewApprovalService(env)
	providers := NewProviderService(env)
	settings := NewSettingsService(env, env.Paths)
	// 知识库检索作为内置工具暴露给内核；没装配知识库时跳过。
	if env.Knowledge != nil && env.Registry != nil {
		if err := env.Registry.Register(&tool.KnowledgeTool{Search: env.Knowledge.Search}); err != nil {
			return nil, err
		}
	}
	return &Container{
		Env:       env,
		Sessions:  sessions,
		Chat:      NewChatService(env, sessions, approvals, providers),
		Approvals: approvals,
		Providers: providers,
		Skills:    NewSkillService(env),
		Tools:     NewToolService(env),
		Knowledge: env.Knowledge,
		Settings:  settings,
	}, nil
}

// Bootstrap 首启兜底：写默认设置。
func (c *Container) Bootstrap() error { return c.Settings.SeedDefaults() }
