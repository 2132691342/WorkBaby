package domain

// 本文件是 SQLite 持久层的行映射形状。
// 会话正文仍以 JSONL 文件为真相，这里只存「可查询的事实」：清单、统计、模型目录。

// SessionIndexDO 是会话索引行：会话文件之外的检索视图。
// 它是派生数据——删掉索引不会丢会话，重建索引能从文件恢复，
// 所以写入失败只降级检索能力，不影响对话本身。
type SessionIndexDO struct {
	// SessionID 会话 ID（会话文件头部的事实）
	SessionID string `gorm:"primaryKey;size:64" json:"session_id"`
	// SessionTitle 会话标题
	SessionTitle string `gorm:"size:512" json:"session_title"`
	// SessionFile 会话文件绝对路径
	SessionFile string `gorm:"size:1024;index" json:"session_file"`
	// WorkspacePath 所属工作区
	WorkspacePath string `gorm:"size:1024;index" json:"workspace_path"`
	// SessionCwd 会话绑定的工作目录
	SessionCwd string `gorm:"size:1024" json:"session_cwd"`
	// MessageCount 消息条数
	MessageCount int `json:"message_count"`
	// ToolCallCount 工具调用次数
	ToolCallCount int `json:"tool_call_count"`
	// TotalTokens 累计 token
	TotalTokens int64 `json:"total_tokens"`
	// CreatedAtMs 创建时刻（毫秒）
	CreatedAtMs int64 `gorm:"index" json:"created_at_ms"`
	// UpdatedAtMs 最近修改时刻（毫秒）
	UpdatedAtMs int64 `gorm:"index" json:"updated_at_ms"`
}

// TableName 表名。显式给出：表名是持久契约，不该由复数化规则推导。
func (SessionIndexDO) TableName() string { return "session_index" }

// UsageRecordDO 是一条用量记录：一轮回复的 token 账。
type UsageRecordDO struct {
	// UsageID 记录 ID
	UsageID string `gorm:"primaryKey;size:64" json:"usage_id"`
	// SessionID 所属会话
	SessionID string `gorm:"size:64;index" json:"session_id"`
	// WorkspacePath 所属工作区
	WorkspacePath string `gorm:"size:1024;index" json:"workspace_path"`
	// ProviderName 服务商名
	ProviderName string `gorm:"size:128;index" json:"provider_name"`
	// ModelId 模型 ID
	ModelId string `gorm:"size:128;index" json:"model_id"`
	// InputTokens 输入 token
	InputTokens int `json:"input_tokens"`
	// OutputTokens 输出 token
	OutputTokens int `json:"output_tokens"`
	// CacheReadTokens 缓存命中 token
	CacheReadTokens int `json:"cache_read_tokens"`
	// CacheWriteTokens 缓存写入 token
	CacheWriteTokens int `json:"cache_write_tokens"`
	// TotalTokens 合计 token
	TotalTokens int64 `json:"total_tokens"`
	// RecordedAtMs 记录时刻（毫秒）
	RecordedAtMs int64 `gorm:"index" json:"recorded_at_ms"`
}

// TableName 表名。
func (UsageRecordDO) TableName() string { return "usage_record" }

// ToolCallRecordDO 是一条工具调用记录：能力画像与耗时统计的来源。
type ToolCallRecordDO struct {
	// ToolCallID 调用 ID
	ToolCallID string `gorm:"primaryKey;size:64" json:"tool_call_id"`
	// SessionID 所属会话
	SessionID string `gorm:"size:64;index" json:"session_id"`
	// ToolName 工具名
	ToolName string `gorm:"size:128;index" json:"tool_name"`
	// ToolIsError 是否失败
	ToolIsError bool `json:"tool_is_error"`
	// DurationMs 执行耗时（毫秒）
	DurationMs int64 `json:"duration_ms"`
	// RecordedAtMs 记录时刻（毫秒）
	RecordedAtMs int64 `gorm:"index" json:"recorded_at_ms"`
}

// TableName 表名。
func (ToolCallRecordDO) TableName() string { return "tool_call_record" }

// ProviderDO 是服务商行：接口地址、协议与凭据。
type ProviderDO struct {
	// ProviderName 服务商名（主键）
	ProviderName string `gorm:"primaryKey;size:128" json:"provider_name"`
	// BaseUrl 接口根地址
	BaseUrl string `gorm:"size:1024" json:"base_url"`
	// ApiProtocol 协议标识
	ApiProtocol string `gorm:"size:64" json:"api_protocol"`
	// AuthKind 认证方式；恒为 api_key
	AuthKind string `gorm:"size:32" json:"auth_kind"`
	// ApiKeySecret 密钥原文；只在本进程内流转，界面读到的是掩码
	ApiKeySecret string `gorm:"size:1024" json:"-"`
	// ProviderSort 排序位
	ProviderSort int `json:"provider_sort"`
	// CreatedAtMs 创建时刻（毫秒）
	CreatedAtMs int64 `json:"created_at_ms"`
	// UpdatedAtMs 最近修改时刻（毫秒）
	UpdatedAtMs int64 `json:"updated_at_ms"`
}

// TableName 表名。
func (ProviderDO) TableName() string { return "provider" }

// ProviderModelDO 是模型行：一条服务商下的一个可用模型。
type ProviderModelDO struct {
	// ProviderName 所属服务商（复合主键）
	ProviderName string `gorm:"primaryKey;size:128" json:"provider_name"`
	// ModelId 模型 ID（复合主键）
	ModelId string `gorm:"primaryKey;size:128" json:"model_id"`
	// ModelName 界面上的名字
	ModelName string `gorm:"size:256" json:"model_name"`
	// ApiProtocol 单独覆盖协议
	ApiProtocol string `gorm:"size:64" json:"api_protocol"`
	// ContextWindow 上下文窗口 token 数
	ContextWindow int `json:"context_window"`
	// MaxTokens 单次输出上限
	MaxTokens int `json:"max_tokens"`
	// SupportsReasoning 是否支持思考
	SupportsReasoning bool `json:"supports_reasoning"`
	// InputKinds 支持的输入类型，逗号分隔
	InputKinds string `gorm:"size:128" json:"input_kinds"`
	// ModelSort 排序位
	ModelSort int `json:"model_sort"`
	// CreatedAtMs 创建时刻（毫秒）
	CreatedAtMs int64 `json:"created_at_ms"`
}

// TableName 表名。
func (ProviderModelDO) TableName() string { return "provider_model" }

// SearchDocDO 是全文检索文档：索引器把工作区文件摊平成可搜的行。
// 只存可检索字段，不存正文——正文仍以文件为准，索引随时可以重建。
type SearchDocDO struct {
	// DocID 文档 ID（复合主键）
	DocID string `gorm:"primaryKey;size:128" json:"doc_id"`
	// WorkspacePath 所属工作区
	WorkspacePath string `gorm:"size:1024;index" json:"workspace_path"`
	// DocPath 相对工作区的路径
	DocPath string `gorm:"size:1024" json:"doc_path"`
	// DocKind 种类：file / symbol
	DocKind string `gorm:"size:32;index" json:"doc_kind"`
	// DocSymbol 符号名
	DocSymbol string `gorm:"size:256;index" json:"doc_symbol"`
	// DocSymbolKind 符号种类：func / type / var
	DocSymbolKind string `gorm:"size:32" json:"doc_symbol_kind"`
	// DocLine 符号所在行号
	DocLine int `json:"doc_line"`
	// DocBody 可检索正文摘要
	DocBody string `gorm:"type:text" json:"doc_body"`
	// DocSize 字节数
	DocSize int64 `json:"doc_size"`
	// DocMtimeMs 文件修改时刻（毫秒）
	DocMtimeMs int64 `gorm:"index" json:"doc_mtime_ms"`
	// DocHitCount 命中次数，用于频次排序
	DocHitCount int `json:"doc_hit_count"`
	// DocLastHitMs 最近命中时刻（毫秒）
	DocLastHitMs int64 `json:"doc_last_hit_ms"`
}

// TableName 表名。
func (SearchDocDO) TableName() string { return "search_doc" }

// PlanStateDO 是计划模式的持久状态。
// 它必须是持久事实：重启应用后界面要立刻知道当前是只读计划态还是自由态。
type PlanStateDO struct {
	// SessionID 所属会话（主键）
	SessionID string `gorm:"primaryKey;size:64" json:"session_id"`
	// PlanEnabled 是否处于只读计划模式
	PlanEnabled bool `json:"plan_enabled"`
	// PlanUpdatedAtMs 状态变更时刻（毫秒）
	PlanUpdatedAtMs int64 `json:"plan_updated_at_ms"`
}

// TableName 表名。
func (PlanStateDO) TableName() string { return "plan_state" }
