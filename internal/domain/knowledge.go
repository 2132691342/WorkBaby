// 知识库聚合根：文档与其切片的持久化形态。
package domain

import "WorkBaby/internal/pkg"

var (
	ErrDocNotFound = pkg.New(7101, "文档不存在或已不可读", "")
	ErrDocType     = pkg.New(7102, "暂不支持这种文件", "支持 PDF / Word / Excel / Markdown / 文本")
	ErrQueryEmpty  = pkg.New(7104, "请输入要查的内容", "")
)

// KnowledgeDocDO 知识库文档。
type KnowledgeDocDO struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	Path      string `gorm:"size:1024" json:"path"`
	Title     string `gorm:"size:256" json:"title"`
	Ext       string `gorm:"size:16" json:"ext"`
	Size      int64  `json:"size"`
	Chunks    int    `json:"chunks"`
	Status    string `gorm:"size:16" json:"status"`
	Error     string `gorm:"size:512" json:"error"`
	CreatedAt int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (KnowledgeDocDO) TableName() string { return "knowledge_docs" }

// KnowledgeChunkDO 文档分块；content 同时进 FTS 虚表，靠触发器同步。
type KnowledgeChunkDO struct {
	ID      string `gorm:"primaryKey;size:64" json:"id"`
	DocID   string `gorm:"size:64;index" json:"doc_id"`
	Seq     int    `json:"seq"`
	Content string `gorm:"type:text" json:"content"`
	Hash    string `gorm:"size:64" json:"hash"`
}

// TableName 显式指定表名：GORM 会把 DO 后缀复数化成 _dos。
func (KnowledgeChunkDO) TableName() string { return "knowledge_chunks" }

// KnowledgeDocVO 文档列表项。
type KnowledgeDocVO struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	Chunks    int    `json:"chunks"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// AddDocsREQ 添加文档入参。
type AddDocsREQ struct {
	Paths []string `json:"paths"`
}

// SearchREQ 检索入参。
type SearchREQ struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// SearchHitVO 检索命中。
type SearchHitVO struct {
	DocID   string  `json:"doc_id"`
	Title   string  `json:"title"`
	Path    string  `json:"path"`
	Seq     int     `json:"seq"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// SearchRESP 检索出参。
type SearchRESP struct {
	Hits []SearchHitVO `json:"hits"`
}

// AddDocsRESP 添加文档出参。
type AddDocsRESP struct {
	Added int `json:"added"`
}

// ReindexRESP 重建索引出参。
type ReindexRESP struct {
	Reindexed int `json:"reindexed"`
}
