// 全局 ID 生成：ULID 大写 + 领域前缀（单机单用户，不需要用户维度 id）。
package domain

// 主键前缀：ULID 带前缀便于日志与排障时一眼看出实体类别。
const (
	PrefixSession    = "SESSION"
	PrefixEntry      = "ENTRY"
	PrefixApproval   = "APPROVAL"
	PrefixProvider   = "PROVIDER"
	PrefixDoc        = "KNDOC"
	PrefixChunk      = "KNCHUNK"
	PrefixUsage      = "USAGE"
	PrefixMCPServer  = "MCPSRV"
	PrefixEval       = "EVAL"
	PrefixBaseline   = "BASE"
	PrefixMemory     = "MEM"
	PrefixRun        = "RUN"
	PrefixSnapshot   = "SNAP"
	PrefixCheckpoint = "CKPT"
)

// ContractVersion 前后端契约版本。改动 RESP 字段或接口路径必须同步改前端 types/api.ts 并 +1。
const ContractVersion = 6

// 会话条目类型。
const EntryTypeMessage = "message"

// 消息角色。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "toolResult"
)

// 会话权限档：面向新手只有三档，用一句话说明区别。
// 它驱动审批扩展的行为（specs/19），不由内核直接实现。
const (
	PermissionAuto     = "auto"      // 全部放行，不弹任何确认
	PermissionAskWrite = "ask_write" // 非只读工具都要确认
	PermissionReadonly = "readonly"  // 只放行只读工具，其余直接拒
)

// 上游协议类型：直接采用 PI 的协议 id，因为写进 models.json 的就是这几个值。
// 本地再定义一套自己的名字，映射层迟早与 PI 分叉。
const (
	APIOpenAICompletions = "openai-completions"
	APIOpenAIResponses   = "openai-responses"
	APIAnthropicMessages = "anthropic-messages"
	APIGoogle            = "google-generative-ai"
)

// APIAll 是全部合法协议 id，供校验与前端下拉使用。
var APIAll = []string{APIOpenAICompletions, APIOpenAIResponses, APIAnthropicMessages, APIGoogle}

// 审批状态与放行范围。
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalDenied   = "denied"

	ApprovalScopeOnce    = "once"    // 只放行这一次调用
	ApprovalScopeSession = "session" // 放行整个会话，同类操作不再问
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
