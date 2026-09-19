package domain

import "WorkBaby/internal/pkg"

// ArtifactKind 工件类别（前端面板按类别渲染图标/预览器）。
type ArtifactKind string

const (
	ArtifactImage ArtifactKind = "image"
	ArtifactDoc   ArtifactKind = "doc"
	ArtifactCode  ArtifactKind = "code"
	ArtifactData  ArtifactKind = "data"
	ArtifactFile  ArtifactKind = "file"
)

// ArtifactDO 会话产出物登记（artifacts 表；）。
//
// 只登记引用（路径 + 大小），不存内容——内容留在工作区，前端经 /files 服务预览。
// (session_id, rel_path) 唯一：同一文件反复修改只更新元信息，不堆历史版本
// （历史版本由 file_changes 的快照承担）。
type ArtifactDO struct {
	ID        string       `gorm:"primaryKey;size:64"           json:"id"`
	SessionID string       `gorm:"size:64;index:idx_art_session;uniqueIndex:uk_art_session_path,priority:1" json:"session_id"`
	RunID     string       `gorm:"size:64"                      json:"run_id"`
	Kind      ArtifactKind `gorm:"size:16"                      json:"kind"`
	Name      string       `gorm:"size:255"                     json:"name"`
	RelPath   string       `gorm:"size:512;uniqueIndex:uk_art_session_path,priority:2" json:"rel_path"`
	Path      string       `gorm:"size:512"                     json:"-"` // 磁盘绝对路径，不回传前端
	MimeType  string       `gorm:"size:64"                      json:"mime_type"`
	Size      int64        `json:"size"`
	CreatedAt int64        `gorm:"autoCreateTime:milli"         json:"created_at"`
	UpdatedAt int64        `gorm:"autoUpdateTime:milli;index:idx_art_session" json:"updated_at"`
}

// TableName 固定表名。
func (ArtifactDO) TableName() string { return "artifacts" }

// ArtifactRESP 出参（前端工件面板条目）。
type ArtifactRESP struct {
	ID        string       `json:"id"`
	SessionID string       `json:"session_id"`
	RunID     string       `json:"run_id"`
	Kind      ArtifactKind `json:"kind"`
	Name      string       `json:"name"`
	RelPath   string       `json:"rel_path"`
	MimeType  string       `json:"mime_type"`
	Size      int64        `json:"size"`
	CreatedAt int64        `json:"created_at"`
	UpdatedAt int64        `json:"updated_at"`
}

// ArtifactListRESP 工件列表出参。
type ArtifactListRESP struct {
	Items []ArtifactRESP `json:"items"`
	Total int            `json:"total"`
}

// 错误变量；段位 4011。
var ErrArtifactNotFound = pkg.New(4011, "工件不存在", "")
