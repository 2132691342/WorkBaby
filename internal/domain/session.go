// 会话聚合根：会话本体、分支指针与运行状态。
package domain

import "WorkBaby/internal/pkg"

var (
	ErrSessionNotFound = pkg.New(1101, "会话不存在", "")
	ErrEntryNotFound   = pkg.New(1102, "消息不存在", "")
	ErrSessionBusy     = pkg.New(1103, "这个会话正在处理上一条消息", "")
	ErrEmptyContent    = pkg.New(1104, "说点什么再发送吧", "")
)

// SessionDO 会话聚合根。leaf_entry_id 指向当前位置，分支回溯只移动这一个字段。
type SessionDO struct {
	ID           string `gorm:"primaryKey;size:64" json:"id"`
	Title        string `gorm:"size:256" json:"title"`
	Workspace    string `gorm:"size:512" json:"workspace"`
	ProviderID   string `gorm:"size:64;index" json:"provider_id"`
	Model        string `gorm:"size:128" json:"model"`
	Permission   string `gorm:"size:16" json:"permission"`
	LeafEntryID  string `gorm:"size:64" json:"leaf_entry_id"`
	MessageCount int    `json:"message_count"`
	TotalTokens  int    `json:"total_tokens"`
	CreatedAt    int64  `gorm:"autoCreateTime:milli;index" json:"created_at"`
	UpdatedAt    int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (SessionDO) TableName() string { return "sessions" }

// CreateSessionREQ 新建会话入参；字段全部可选，缺省用全局设置兜底。
type CreateSessionREQ struct {
	Title      string `json:"title"`
	Workspace  string `json:"workspace"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
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

// SetPermissionREQ 切换权限档入参。
type SetPermissionREQ struct {
	Permission string `json:"permission"`
}

// SetWorkspaceREQ 切换工作目录入参。
type SetWorkspaceREQ struct {
	Workspace string `json:"workspace"`
}

// BranchREQ 从某条历史回溯入参。
type BranchREQ struct {
	EntryID string `json:"entry_id"`
}

// SessionVO 会话列表项。
type SessionVO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Workspace    string `json:"workspace"`
	ProviderID   string `json:"provider_id"`
	Model        string `json:"model"`
	Permission   string `json:"permission"`
	LeafEntryID  string `json:"leaf_entry_id"`
	MessageCount int    `json:"message_count"`
	TotalTokens  int    `json:"total_tokens"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// SessionDetailVO 会话详情：会话 + 已还原的线性消息。
type SessionDetailVO struct {
	Session SessionVO   `json:"session"`
	Messages []MessageVO `json:"messages"`
}

// ToVO 把 DO 转成出参。
func (s *SessionDO) ToVO() SessionVO {
	return SessionVO{
		ID:           s.ID,
		Title:        s.Title,
		Workspace:    s.Workspace,
		ProviderID:   s.ProviderID,
		Model:        s.Model,
		Permission:   s.Permission,
		LeafEntryID:  s.LeafEntryID,
		MessageCount: s.MessageCount,
		TotalTokens:  s.TotalTokens,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}
