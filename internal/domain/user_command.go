// Package domain 是业务聚合根：每个聚合根一个文件，含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量。
//
// 本文件：UserCommand 聚合根：用户自定义斜杠命令（保存的提示词模板），
// 在 / 命令面板中与内置命令并列，选中即把提示词灌入输入框。
package domain

import (
	"gorm.io/gorm"

	"WorkBaby/internal/pkg"
)

// UserCommandDO 自定义命令持久化实体（user_commands 表是唯一真相源）。
type UserCommandDO struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	Name        string         `gorm:"size:64;uniqueIndex" json:"name"` // kebab-case，/面板中的命令名
	Prompt      string         `gorm:"type:text" json:"prompt"`         // 提示词模板（可含 $ARGUMENTS 占位）
	Description string         `gorm:"size:512" json:"description"`
	CreatedAt   int64          `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt   int64          `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 固定表名。
func (UserCommandDO) TableName() string { return "user_commands" }

// UserCommandRESP 出参。
type UserCommandRESP struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// UserCommandREQ 创建/更新入参（按 name upsert）。
type UserCommandREQ struct {
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

// 包级错误变量；错误码段位 8300（UserCommand 聚合根）。
var (
	ErrUserCommandNotFound = pkg.New(8301, "user command not found", "")
	ErrUserCommandInvalid  = pkg.New(8302, "user command invalid", "")
)
