package service

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/knowledge"
	"WorkBaby/backend/tool"
)

// Container 是全部编排服务的组合根，构造期一次性装配。
type Container struct {
	Env       *Env
	Sessions  *SessionService
	Chat      *ChatService
	Approvals *ApprovalService
	Providers *ProviderService
	Skills    *SkillService
	Files     *FileService
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
		Files:     NewFileService(env),
		Tools:     NewToolService(env),
		Knowledge: env.Knowledge,
		Settings:  settings,
	}, nil
}

// Bootstrap 首启兜底：收口上次运行残留的待决审批，再写默认设置。
// 返回被收口的审批清单交给前端补偿提示——进程退出时挂在等待里的那几条，
// 用户必须知道它们被按拒绝处理了，否则「助手那一步没做」永远查不到原因。
func (c *Container) Bootstrap() ([]domain.ApprovalVO, error) {
	expired, err := c.Approvals.ExpireStale()
	if err != nil {
		return nil, err
	}
	if err := c.Settings.MigrateLegacy(); err != nil {
		return nil, err
	}
	if err := c.Settings.SeedDefaults(); err != nil {
		return nil, err
	}
	return expired, nil
}
