package domain

import "WorkBaby/internal/pkg"

// MessageBlockKind 消息块类型：assistant 消息的持久化分块（消息块落库）。
type MessageBlockKind string

const (
	BlockThinking   MessageBlockKind = "thinking"    // 推理文本
	BlockText       MessageBlockKind = "text"        // 正文片段（模型在两次工具调用之间的叙述）
	BlockToolCall   MessageBlockKind = "tool_call"   // 工具调用（模型决策 + 参数）
	BlockToolResult MessageBlockKind = "tool_result" // 工具执行结果
	BlockArtifact   MessageBlockKind = "artifact"    // 工具产出的结构化数据（todo 快照等）
	BlockGenUI      MessageBlockKind = "genui"       // 生成式 UI 片段
	BlockSkill      MessageBlockKind = "skill"       // 本轮命中的 Skill（执行过程时间线首行）
)

// MessageBlockDO 消息块持久化实体：块即行，可索引、可按会话分页。
// 随会话级联清理（service 层删会话时同步删块）。
type MessageBlockDO struct {
	ID        string           `gorm:"primaryKey;size:64"  json:"id"`
	MessageID string           `gorm:"size:64;index:idx_block_message_seq" json:"message_id"`
	SessionID string           `gorm:"size:64;index:idx_block_session"     json:"session_id"`
	Seq       int64            `gorm:"index:idx_block_message_seq"         json:"seq"` // 消息内顺序
	Kind      MessageBlockKind `gorm:"size:32"                             json:"kind"`
	Payload   string           `gorm:"type:text"                           json:"payload"` // JSON 载荷
	CreatedAt int64            `gorm:"autoCreateTime:milli"                json:"created_at"`
}

// TableName 固定表名。
func (MessageBlockDO) TableName() string { return "message_blocks" }

// MessageBlockRESP 消息块出参。
type MessageBlockRESP struct {
	ID        string           `json:"id"`
	MessageID string           `json:"message_id"`
	Seq       int64            `json:"seq"`
	Kind      MessageBlockKind `json:"kind"`
	Payload   string           `json:"payload"`
	CreatedAt int64            `json:"created_at"`
}

// 包级错误变量；段位 5000。
var (
	ErrMessageBlockNotFound = pkg.New(5009, "message block not found", "")
	ErrMessageBlockInvalid  = pkg.New(5010, "message block invalid (kind or payload)", "")
)
