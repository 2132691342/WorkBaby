// Package bootstrap 组合根：repo 实例的唯一装配点。
//
// api 层经 App 持有数据访问句柄，自身不再 import repo——业务数据访问必须经 service 层
// （check-boundaries 的 api-no-repo 断言）。App 只做构造与持有，不含任何业务逻辑。
package bootstrap

import (
	"WorkBaby/internal/repo"

	"gorm.io/gorm"
)

// App 持有全部 repo 实例；由 api.Handler.Startup 在数据库打开后构造一次。
type App struct {
	DB *gorm.DB

	SessRepo          *repo.ChatSessionRepo
	MsgRepo           *repo.MessageRepo
	ProvRepo          *repo.AiProviderRepo
	SetRepo           *repo.SystemSettingRepo
	UsageRepo         *repo.TokenUsageRepo
	BlocksRepo        *repo.MessageBlockRepo
	CheckpointRepo    *repo.AgentCheckpointRepo
	RunRecRepo        *repo.RunRecordRepo
	TodoRepo          *repo.SessionTodoRepo
	ApprovalRecRepo   *repo.ApprovalRecordRepo
	ApprovalGrantRepo *repo.ApprovalGrantRepo
	ChatTaskRepo      *repo.ChatTaskRepo
	TrustRepo         *repo.WorkspaceTrustRepo
	PetCfgRepo        *repo.PetConfigRepo
	PetSpriteRepo     *repo.PetSpriteRepo
	FolderRepo        *repo.FolderRepo
	FileRepo          *repo.FileRepo
	FileChangeRepo    *repo.FileChangeRepo
	ArtifactRepo      *repo.ArtifactRepo
	DashboardRepo     *repo.DashboardRepo
	KnowledgeDocRepo  *repo.KnowledgeDocRepo
	SkillRepo         *repo.SkillRepo
	AgentProfileRepo  *repo.AgentProfileRepo
	UserCommandRepo   *repo.UserCommandRepo
	UserHookRepo      *repo.UserHookRepo
	McpRepo           *repo.McpServerRepo
}

// New 构造组合根：打开数据库后调用一次，全部 repo 共享同一连接。
func New(db *gorm.DB) *App {
	return &App{
		DB:                db,
		SessRepo:          repo.NewChatSessionRepo(db),
		MsgRepo:           repo.NewMessageRepo(db),
		ProvRepo:          repo.NewAiProviderRepo(db),
		SetRepo:           repo.NewSystemSettingRepo(db),
		UsageRepo:         repo.NewTokenUsageRepo(db),
		BlocksRepo:        repo.NewMessageBlockRepo(db),
		CheckpointRepo:    repo.NewAgentCheckpointRepo(db),
		RunRecRepo:        repo.NewRunRecordRepo(db),
		TodoRepo:          repo.NewSessionTodoRepo(db),
		ApprovalRecRepo:   repo.NewApprovalRecordRepo(db),
		ApprovalGrantRepo: repo.NewApprovalGrantRepo(db),
		ChatTaskRepo:      repo.NewChatTaskRepo(db),
		TrustRepo:         repo.NewWorkspaceTrustRepo(db),
		PetCfgRepo:        repo.NewPetConfigRepo(db),
		PetSpriteRepo:     repo.NewPetSpriteRepo(db),
		FolderRepo:        repo.NewFolderRepo(db),
		FileRepo:          repo.NewFileRepo(db),
		FileChangeRepo:    repo.NewFileChangeRepo(db),
		ArtifactRepo:      repo.NewArtifactRepo(db),
		DashboardRepo:     repo.NewDashboardRepo(db),
		KnowledgeDocRepo:  repo.NewKnowledgeDocRepo(db),
		SkillRepo:         repo.NewSkillRepo(db),
		AgentProfileRepo:  repo.NewAgentProfileRepo(db),
		UserCommandRepo:   repo.NewUserCommandRepo(db),
		UserHookRepo:      repo.NewUserHookRepo(db),
		McpRepo:           repo.NewMcpServerRepo(db),
	}
}
