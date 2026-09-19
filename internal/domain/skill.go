// Package domain 是业务聚合根：每个聚合根一个文件，含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量。
//
// 本文件：Skill 聚合根：结构化 prompt 模板 + 工具白名单。
package domain

import (
	"gorm.io/gorm"

	"WorkBaby/internal/pkg"
)

// SkillSourceKind Skill 来源。
type SkillSourceKind string

const (
	SkillSourceKindBuiltin  SkillSourceKind = "builtin"  // assets/skills/ 编译期内置
	SkillSourceKindDownload SkillSourceKind = "download" // git/zip 安装
	SkillSourceKindCustom   SkillSourceKind = "custom"   // 用户自建
)

// SkillScript Skill 内置脚本（scripts 挂成 run_skill_script 工具）。
type SkillScript struct {
	Name     string `json:"name"`
	Language string `json:"language"` // javascript | python | powershell
	Code     string `json:"code"`
}

// SkillDO Skill 持久化实体（skills 表是唯一真相源；SKILL.md 只是导入/导出格式）。
type SkillDO struct {
	ID           string          `gorm:"primaryKey;size:64" json:"id"`
	Name         string          `gorm:"size:64;uniqueIndex" json:"name"` // kebab-case，唯一
	Description  string          `gorm:"size:512" json:"description"`
	WhenToUse    string          `gorm:"type:text" json:"when_to_use"` // 触发关键词（每行一个，中英）
	Body         string          `gorm:"type:text" json:"body"`        // Markdown 正文（prompt）
	AllowedTools string          `gorm:"type:text" json:"-"`           // JSON 数组；空 = 不限制
	ScriptsJSON  string          `gorm:"type:text" json:"-"`           // SkillScript 数组 JSON；空 = 无脚本
	Frontmatter  string          `gorm:"type:text" json:"-"`           // 原始 YAML
	SourceKind   SkillSourceKind `gorm:"size:16" json:"source_kind"`
	SourceRef    string          `gorm:"size:512" json:"source_ref"`
	Version      string          `gorm:"size:32" json:"version"`
	Enabled      bool            `gorm:"default:true" json:"enabled"`
	CreatedAt    int64           `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt    int64           `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt    gorm.DeletedAt  `gorm:"index" json:"-"`
}

// TableName 固定表名。
func (SkillDO) TableName() string { return "skills" }

// SkillRESP 出参（设置页 / 技能列表展示）。
type SkillRESP struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	WhenToUse    string          `json:"when_to_use"`
	Body         string          `json:"body,omitempty"`
	AllowedTools []string        `json:"allowed_tools,omitempty"`
	Scripts      []SkillScript   `json:"scripts,omitempty"`
	SourceKind   SkillSourceKind `json:"source_kind"`
	SourceRef    string          `json:"source_ref,omitempty"`
	Version      string          `json:"version,omitempty"`
	Enabled      bool            `json:"enabled"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
}

// SkillREQ 创建/更新入参（custom 来源）。
type SkillREQ struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	WhenToUse    string        `json:"when_to_use"`
	Body         string        `json:"body"`
	AllowedTools []string      `json:"allowed_tools"`
	Scripts      []SkillScript `json:"scripts,omitempty"`
	Enabled      *bool         `json:"enabled"`
}

// 包级错误变量；错误码段位 8000。
var (
	ErrSkillNotFound = pkg.New(8001, "skill not found", "")
	ErrSkillParse    = pkg.New(8002, "SKILL.md parse failed", "")
)
