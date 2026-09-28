package domain

// 主键前缀：ULID 带前缀便于日志与排障时一眼看出实体类别。
const (
	PrefixSession  = "SESSION"
	PrefixEntry    = "ENTRY"
	PrefixApproval = "APPROVAL"
	PrefixProvider = "PROVIDER"
	PrefixDoc      = "KNDOC"
	PrefixChunk    = "KNCHUNK"
	PrefixUsage    = "USAGE"
)

// ContractVersion 前后端契约版本。改动 RESP 字段必须同时改前端 types/api.ts。
const ContractVersion = 2

// 会话条目类型。
const EntryTypeMessage = "message"

// 消息角色。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// 会话权限档：面向新手只有三档，用一句话说明区别。
const (
	PermissionAsk      = "ask"       // 改文件与跑命令都问
	PermissionAutoEdit = "auto_edit" // 改文件不问，跑命令仍问
	PermissionYolo     = "yolo"      // 全自动
)

// 上游协议类型：一个 Provider 只是配置行，新增一家服务通常不需要新适配器。
const (
	APIOpenAI    = "openai"
	APIAnthropic = "anthropic"
	APIOllama    = "ollama"
)

// 审批状态与放行范围。
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalDenied   = "denied"

	ApprovalScopeOnce    = "once"
	ApprovalScopeSession = "session"
)

// 知识库文档状态。
const (
	DocPending = "pending"
	DocIndexed = "indexed"
	DocFailed  = "failed"
)

// 风险等级，只用于 UI 展示，不改变判定逻辑。
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)
