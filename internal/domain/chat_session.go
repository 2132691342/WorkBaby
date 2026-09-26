package domain

import "WorkBaby/internal/pkg"

// SessionStatus 会话状态（chat/session 枚举）。
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusArchived SessionStatus = "archived"
)

// SessionKind 会话类别：normal 主会话（默认，进侧栏列表）；
// side 辅助对话（右栏并行小会话，ParentID 指向主会话并继承其历史，不进侧栏）。
type SessionKind string

const (
	SessionKindNormal SessionKind = "normal"
	SessionKindSide   SessionKind = "side"
)

// ChatSessionDO 会话持久化实体。
type ChatSessionDO struct {
	ID          string `gorm:"primaryKey;size:64"  json:"id"`
	Name        string `gorm:"size:256"             json:"name"`
	UserID      string `gorm:"size:64;index"        json:"user_id"`
	ProviderID  string `gorm:"size:64;index"        json:"provider_id"`
	Model       string `gorm:"size:128"             json:"model"`
	WorkspaceID string `gorm:"size:64;index"        json:"workspace_id"`
	// WorkspacePath 会话绑定的外部工作目录（绝对路径；空 = 默认工作区）。
	// 绑定后：file/doc/archive/skillrun 工具的沙箱根、工作区面板、exec 的默认 cwd 都指向该目录；
	// 与 workspace_id（逻辑文件夹树关联 id，历史字段）语义无关。
	WorkspacePath string        `gorm:"size:512"             json:"workspace_path"`
	Status        SessionStatus `gorm:"size:16"              json:"status"`
	// Kind 会话类别（normal/side）；side 会话被侧栏列表过滤，仅在辅助对话面板中呈现。
	Kind SessionKind `gorm:"size:16;default:'normal';index" json:"kind"`
	// Pinned 置顶：侧栏列表最前（排序 pinned DESC → last_message_at DESC）。
	Pinned        bool  `gorm:"default:false" json:"pinned"`
	MessageCount  int   `gorm:"default:0"            json:"message_count"`
	LastMessageAt int64 `gorm:"default:0"            json:"last_message_at"`
	// ParentID / BranchPoint 构成会话树血缘（分叉可递归；根会话为空 / 0）。
	// PermissionMode 为会话级权限模式（空 = 跟随全局 agent.session_mode）。
	PermissionMode string `gorm:"size:32"              json:"permission_mode"`
	ParentID       string `gorm:"size:64;index"    json:"parent_id"`
	BranchPoint    int64  `gorm:"default:0"        json:"branch_point"`
	MetadataJSON   string `gorm:"type:text;column:metadata_json" json:"metadata_json"`
	CreatedAt      int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt      int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt      int64  `gorm:"default:0;index"     json:"deleted_at"` // 软删；unix milli
}

// TableName 固定表名。
func (ChatSessionDO) TableName() string { return "chat_sessions" }

// ChatSessionREQ 创建/更新请求。
type ChatSessionREQ struct {
	Name          string `json:"name"`
	ProviderID    string `json:"provider_id"`
	Model         string `json:"model"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspacePath string `json:"workspace_path"`
}

// ChatSessionRenameREQ 仅重命名（PATCH 语义）。
type ChatSessionRenameREQ struct {
	Name string `json:"name"`
}

// ChatSessionPermissionREQ 切换会话工具权限模式（restricted/default/auto_edit/yolo；空 = 跟随全局）。
type ChatSessionPermissionREQ struct {
	Mode string `json:"mode"`
}

// SessionPinREQ 置顶/取消置顶入参。
type SessionPinREQ struct {
	Pinned bool `json:"pinned"`
}

// SessionArchiveREQ 归档/取消归档入参（status 在 active/archived 间切换；归档自动取消置顶）。
type SessionArchiveREQ struct {
	Archived bool `json:"archived"`
}

// ChatSessionModelREQ 切换会话使用的 Provider/模型（切换后输入框参数与温度展示跟随该模型配置）。
// ProviderID 与 Model 至少提供一个；只给 Model 时按名字回查 provider。
type ChatSessionModelREQ struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// SetSessionAgentREQ 切换会话 Agent 入参（前端 → /chat/sessions/:id/agent）。
// Agent 合法值：""（清除）/"default"/"coding"/"research"/"writer"。
type SetSessionAgentREQ struct {
	Agent string `json:"agent"`
}

// ChatSessionRESP 出参。
type ChatSessionRESP struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	UserID         string        `json:"user_id"`
	ProviderID     string        `json:"provider_id"`
	Model          string        `json:"model"`
	WorkspaceID    string        `json:"workspace_id"`
	WorkspacePath  string        `json:"workspace_path"`
	Active         bool          `json:"active"`
	MessageCount   int           `json:"message_count"`
	LastMessageAt  int64         `json:"last_message_at"`
	MetadataJSON   string        `json:"metadata_json"`
	Status         SessionStatus `json:"status"`
	Kind           SessionKind   `json:"kind"`
	Pinned         bool          `json:"pinned"`
	PermissionMode string        `json:"permission_mode"`
	ParentID       string        `json:"parent_id"`
	BranchPoint    int64         `json:"branch_point"`
	CreatedAt      int64         `json:"created_at"`
	UpdatedAt      int64         `json:"updated_at"`
}

// ContextSegment 上下文占用的一个分段（/context 的分段口径）。
type ContextSegment struct {
	Key    string `json:"key"`    // system | memory | tools | history
	Title  string `json:"title"`  // 展示名
	Tokens int    `json:"tokens"` // 估算 token（rune/4 近似，与 agent.EstimateTokens 同口径）
	Ratio  int    `json:"ratio"`  // 占上下文窗口的千分比（避免前端浮点误差）
}

// ContextUsageRESP 会话上下文占用快照（GET /chat/usage/context）。Segments 之和 + Free
// 恰为 ContextWindow；Estimated=true 表示未拿到 provider 实测值（前端需标注「估算」）。
type ContextUsageRESP struct {
	SessionID     string           `json:"session_id"`
	Model         string           `json:"model"`
	ContextWindow int              `json:"context_window"`
	ContextBudget int              `json:"context_budget"` // 压缩触发预算（窗口 × 压缩比例；0 = 未启用）
	UsedTokens    int              `json:"used_tokens"`
	FreeTokens    int              `json:"free_tokens"`
	UsedRatio     int              `json:"used_ratio"` // 千分比
	Segments      []ContextSegment `json:"segments"`
	MessageCount  int              `json:"message_count"`
	ToolCount     int              `json:"tool_count"`
	Estimated     bool             `json:"estimated"`
}

// 包级错误变量；段位 5000-5999（Harness/Agent/会话编排）。
var (
	ErrSessionNotFound = pkg.New(5001, "session not found", "")
	ErrSessionBusy     = pkg.New(5002, "session is busy (concurrent run)", "")
	ErrSessionInvalid  = pkg.New(5003, "session invalid (missing provider/model)", "")
	// ErrRunNotActive steering 注入前提不满足：该会话当前没有运行中的 run（前端退回普通发送）。
	ErrRunNotActive = pkg.New(5010, "no active run for session", "")
)

// SendStreamREQ 流式发送入参；temperature/thinkingEffort 为请求级采样参数
// （三层合并的最上层，nil/空 = 走 provider 级与全局默认）。
type SendStreamREQ struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	// FileIDs 随消息发送的受管文件（图片会转成多模态 part，前提是模型支持视觉）。
	FileIDs        []string `json:"file_ids,omitempty"`
	Temperature    *float64 `json:"temperature,omitempty"`
	ThinkingEffort string   `json:"thinking_effort,omitempty"` // off/low/medium/high
}

// SendStreamResult 绑层发往 Run 的最小结果。
type SendStreamResult struct {
	RunID          string `json:"run_id"`
	SessionID      string `json:"session_id"`
	UserMsgID      string `json:"user_message_id"`
	AssistantMsgID string `json:"assistant_message_id"`
}

// EffectiveParamsRESP 当前会话实际生效的参数快照（输入框与设置页的唯一数据源）。
// From 标注取值层级：provider | default | builtin；请求级覆盖由前端本地持有，不在此列。
type EffectiveParamsRESP struct {
	SessionID         string  `json:"session_id"`
	ProviderID        string  `json:"provider_id"`
	Model             string  `json:"model"`
	Temperature       float64 `json:"temperature"`
	TemperatureFrom   string  `json:"temperature_from"`
	ThinkingEffort    string  `json:"thinking_effort"`
	ThinkingFrom      string  `json:"thinking_from"`
	ContextWindow     int     `json:"context_window"`
	ContextWindowFrom string  `json:"context_window_from"`
	CompressionRatio  float64 `json:"compression_ratio"`
	CompressionFrom   string  `json:"compression_from"`
	ContextBudget     int     `json:"context_budget"` // 触发压缩的 token 阈值 = context_window × ratio
	MaxInputChars     int     `json:"max_input_chars"`
}

// SteerREQ run 进行中插入一条用户指令的入参（steering / follow-up 注入缝）。
type SteerREQ struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// SteerResultRESP 插入回执；前端据此提示「已插入，下一轮生效」。
type SteerResultRESP struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Queued    bool   `json:"queued"`
}

// ForkSessionREQ 从指定消息处分叉为新会话的入参。
type ForkSessionREQ struct {
	MessageID string `json:"message_id"`
	Name      string `json:"name,omitempty"`
}

// BatchDeleteResult 批量删除的结果；前端用于 Toast 区分成功/部分失败。
type BatchDeleteResult struct {
	Ok     []string `json:"ok"`
	Failed []string `json:"failed"`
}

// SessionListRESP 会话分页列表（与前端 Page<Session> 契约对齐）。
type SessionListRESP struct {
	Items []ChatSessionRESP `json:"items"`
	Total int               `json:"total"`
}

// SessionHitDTO 会话检索命中项（repo → service 传输；不落库、不出 API）。
type SessionHitDTO struct {
	SessionID string `json:"session_id"`
	HitCount  int    `json:"hit_count"`
	LastSeq   int64  `json:"last_seq"`
}

// SessionSearchREQ 跨会话检索入参（/resume 快速检索）。
// Scope 控制范围：""/"all" 标题+正文，"title" 仅标题，"content" 仅正文。
type SessionSearchREQ struct {
	Query string `json:"query"`
	Scope string `json:"scope,omitempty"`
	Limit int    `json:"limit,omitempty"`
}
