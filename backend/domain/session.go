// 会话聚合根：WorkBaby 的元数据与指向 PI 会话的指针。
// 消息真身在 PI 的 JSONL 里，ID 与 PiSessionID 分开：JSONL 文件名里的 uuid
// 要 new_session 之后才有，不能做提前标识。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrSessionNotFound = pkg.New("1101", "会话不存在", "")
	ErrEntryNotFound   = pkg.New("1102", "消息不存在", "")
	ErrSessionBusy     = pkg.New("1103", "这个会话正在处理上一条消息", "")
	ErrEmptyContent    = pkg.New("1104", "说点什么再发送吧", "")
)

// SessionDO 会话聚合根。
type SessionDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	Title      string `gorm:"size:256" json:"title"`
	Workspace  string `gorm:"size:512" json:"workspace"`
	ProviderID string `gorm:"size:64;index" json:"provider_id"`
	Model      string `gorm:"size:128" json:"model"`
	// ThinkingLevel 取 off/minimal/low/medium/high/xhigh/max；空 = 跟随 PI 默认。
	ThinkingLevel string `gorm:"size:16" json:"thinking_level"`
	Permission    string `gorm:"size:16" json:"permission"`
	// PiSessionID 与 PiSessionFile 指向 PI 的那条会话：谁都不是对方的副本。
	PiSessionID   string `gorm:"size:64;index" json:"pi_session_id"`
	PiSessionFile string `gorm:"size:1024" json:"pi_session_file"`
	// AllowedTools 是本会话「总是允许」的工具（JSON 数组），由审批桥写入 policy。
	AllowedTools string `gorm:"type:text" json:"-"`
	MessageCount int    `json:"message_count"`
	TotalTokens  int    `json:"total_tokens"`
	// Hidden 是子代理与历史导入的载体会话：全部机制复用普通会话，
	// 但不进用户会话列表——列表里出现一堆来路不明的会话就是乱账。
	Hidden    bool  `json:"hidden"`
	CreatedAt int64 `gorm:"autoCreateTime:milli;index" json:"created_at"`
	UpdatedAt int64 `gorm:"autoUpdateTime:milli;index" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (SessionDO) TableName() string { return "sessions" }

// CreateSessionREQ 新建会话入参；字段全部可选，缺省用全局设置兜底。
type CreateSessionREQ struct {
	Title      string `json:"title"`
	Workspace  string `json:"workspace"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Hidden     bool   `json:"hidden"`
	Permission string `json:"permission"`
}

// RenameSessionREQ 重命名入参。
type RenameSessionREQ struct {
	Title string `json:"title"`
}

// SetModelREQ 切换模型入参。
type SetModelREQ struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// SetThinkingREQ 切换思考档入参。
type SetThinkingREQ struct {
	Level string `json:"level"`
}

// SetPermissionREQ 切换权限档入参。
type SetPermissionREQ struct {
	Permission string `json:"permission"`
}

// SetWorkspaceREQ 切换工作目录入参。
type SetWorkspaceREQ struct {
	Workspace string `json:"workspace"`
}

// ForkREQ 从某条历史分叉入参。
type ForkREQ struct {
	EntryID string `json:"entry_id"`
}

// SessionVO 会话列表项。
type SessionVO struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Workspace     string `json:"workspace"`
	ProviderID    string `json:"provider_id"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinking_level"`
	Permission    string `json:"permission"`
	PiSessionID   string `json:"pi_session_id"`
	PiSessionFile string `json:"pi_session_file"`
	MessageCount  int    `json:"message_count"`
	TotalTokens   int    `json:"total_tokens"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// SessionDetailVO 会话详情：会话 + 从 JSONL 投影的消息。
type SessionDetailVO struct {
	Session  SessionVO   `json:"session"`
	Messages []MessageVO `json:"messages"`
}

// ToVO 把 DO 转成出参。
func (s *SessionDO) ToVO() SessionVO {
	return SessionVO{
		ID:            s.ID,
		Title:         s.Title,
		Workspace:     s.Workspace,
		ProviderID:    s.ProviderID,
		Model:         s.Model,
		ThinkingLevel: s.ThinkingLevel,
		Permission:    s.Permission,
		PiSessionID:   s.PiSessionID,
		PiSessionFile: s.PiSessionFile,
		MessageCount:  s.MessageCount,
		TotalTokens:   s.TotalTokens,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}
