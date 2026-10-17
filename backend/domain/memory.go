// 记忆聚合根：跨会话保留的用户事实与偏好，进系统提示让助手「记得住」。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrMemoryEmpty    = pkg.New("9501", "记忆内容不能为空", "")
	ErrMemoryTooLong  = pkg.New("9502", "这条记忆太长了", "单条上限 500 字，长了就拆成几条")
	ErrMemoryNotFound = pkg.New("9503", "这条记忆不存在", "可能已经删掉了，刷新看看")
	ErrMemoryFull     = pkg.New("9504", "记忆装满了", "先删掉几条不再需要的")
)

// 记忆来源：用户自己写的，还是助手在对话里记下的。界面上要能分开看。
const (
	MemorySourceManual = "manual"
	MemorySourceAuto   = "auto"
)

// 单条长度与总量上限：记忆要进系统提示，不封顶就是「提示词被记忆吃满」，
// 表现为助手记住了几十条偏好却连当前问题都答不好。
const (
	MaxMemoryChars = 500
	MaxMemories    = 200
)

// MemoryDO 一条记忆。
type MemoryDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	Content   string `gorm:"type:text" json:"content"`
	Source    string `gorm:"size:16" json:"source"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (MemoryDO) TableName() string { return "memories" }

// MemoryVO 记忆列表项。
type MemoryVO struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AddMemoryREQ 新增记忆入参。
type AddMemoryREQ struct {
	Content string `json:"content"`
}

// UpdateMemoryREQ 更新记忆入参：content 为空表示只改开关。
type UpdateMemoryREQ struct {
	Content string `json:"content"`
	Enabled *bool  `json:"enabled"`
}

// ListMemoriesRESP 记忆列表出参。
type ListMemoriesRESP struct {
	Items []MemoryVO `json:"items"`
}
