// Package domain 是业务聚合根：每个聚合根一个文件，含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量。
//
// 本文件：Knowledge 聚合根：知识库文档 + 分块 + FTS5 检索。
package domain

import (
	"gorm.io/gorm"

	"WorkBaby/internal/pkg"
)

// KnowledgeDocStatus 文档索引状态机：pending → parsing → indexed | failed。
type KnowledgeDocStatus string

const (
	KnowledgeStatusPending KnowledgeDocStatus = "pending"
	KnowledgeStatusParsing KnowledgeDocStatus = "parsing"
	KnowledgeStatusIndexed KnowledgeDocStatus = "indexed"
	KnowledgeStatusFailed  KnowledgeDocStatus = "failed"
)

// KnowledgeSourceType 文档来源。
type KnowledgeSourceType string

const (
	KnowledgeSourceFile KnowledgeSourceType = "file"
	KnowledgeSourceURL  KnowledgeSourceType = "url"
	KnowledgeSourceText KnowledgeSourceType = "text"
)

// KnowledgeDocDO 知识库文档（knowledge_docs 表）。
type KnowledgeDocDO struct {
	ID         string              `gorm:"primaryKey;size:64" json:"id"`
	FolderID   *string             `gorm:"size:64;index" json:"folder_id,omitempty"`
	Name       string              `gorm:"size:512" json:"name"`
	Source     string              `gorm:"size:1024" json:"source"` // 文件路径 / URL / 原文
	SourceType KnowledgeSourceType `gorm:"size:32" json:"source_type"`
	MIME       string              `gorm:"size:128" json:"mime"`
	SizeBytes  int64               `gorm:"default:0" json:"size_bytes"`
	ChunkCount int                 `gorm:"default:0" json:"chunk_count"`
	Status     KnowledgeDocStatus  `gorm:"size:16;index" json:"status"`
	ErrorMsg   string              `gorm:"size:1024" json:"error_msg"`
	CreatedAt  int64               `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt  int64               `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt  gorm.DeletedAt      `gorm:"index" json:"-"`
}

// TableName 固定表名。
func (KnowledgeDocDO) TableName() string { return "knowledge_docs" }

// KnowledgeChunkDO 文档分块（knowledge_chunks 表）。
type KnowledgeChunkDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	DocID     string `gorm:"size:64;index" json:"doc_id"`
	Sequence  int    `json:"sequence"`
	Content   string `gorm:"type:text" json:"content"`
	Tokens    int    `gorm:"default:0" json:"tokens"`
	MetaJSON  string `gorm:"type:text" json:"-"` // 标题/页码等定位信息
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 固定表名。
func (KnowledgeChunkDO) TableName() string { return "knowledge_chunks" }

// KnowledgeDocREQ 上传入参。
type KnowledgeDocREQ struct {
	FolderID   *string `json:"folder_id,omitempty"`
	Name       string  `json:"name"`
	Source     string  `json:"source"`      // 文件路径 / URL / 原文
	SourceType string  `json:"source_type"` // file / url / text
}

// KnowledgeDocRESP 出参。
type KnowledgeDocRESP struct {
	ID         string              `json:"id"`
	FolderID   *string             `json:"folder_id,omitempty"`
	Name       string              `json:"name"`
	Source     string              `json:"source"`
	SourceType KnowledgeSourceType `json:"source_type"`
	MIME       string              `json:"mime"`
	SizeBytes  int64               `json:"size_bytes"`
	ChunkCount int                 `json:"chunk_count"`
	Status     KnowledgeDocStatus  `json:"status"`
	ErrorMsg   string              `json:"error_msg,omitempty"`
	CreatedAt  int64               `json:"created_at"`
	UpdatedAt  int64               `json:"updated_at"`
}

// KnowledgeHitRESP 检索命中项。
type KnowledgeHitRESP struct {
	DocID   string            `json:"doc_id"`
	DocName string            `json:"doc_name"`
	ChunkID string            `json:"chunk_id"`
	Content string            `json:"content"`
	Score   float64           `json:"score"`
	Source  string            `json:"source"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// 包级错误变量；错误码段位 7000。
var (
	ErrKnowledgeDocNotFound = pkg.New(7005, "knowledge doc not found", "")
	ErrKnowledgeUnsupported = pkg.New(7006, "unsupported file format", "")
)
