package domain

import "WorkBaby/internal/pkg"

// FolderDO 文件夹（folders 表）。
// path 为完整路径字符串（a/b/c）；parentId 指向父目录；workspaceId 为空 = 全局（任何会话可见）。
type FolderDO struct {
	ID          string  `gorm:"primaryKey;size:64"  json:"id"`
	Name        string  `gorm:"size:256;not null"   json:"name"`
	ParentID    *string `gorm:"size:64;index"      json:"parent_id,omitempty"`
	Path        string  `gorm:"size:1024;not null"  json:"path"`
	Description string  `gorm:"size:512"            json:"description,omitempty"`
	WorkspaceID *string `gorm:"size:64;index"      json:"workspace_id,omitempty"`
	CreatedAt   int64   `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt   int64   `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (FolderDO) TableName() string { return "folders" }

// FolderREQ 创建/更新请求（校验在 api 层）。
type FolderREQ struct {
	Name        string  `json:"name"`
	ParentID    *string `json:"parent_id,omitempty"`
	Description string  `json:"description,omitempty"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
}

// FolderRESP 出参（对齐前端 Folder；childCount 为直接子目录数）。
type FolderRESP struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ParentID    *string `json:"parent_id,omitempty"`
	Path        string  `json:"path"`
	Description string  `json:"description,omitempty"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
	ChildCount  int     `json:"child_count"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

// FolderTreeNode 树节点（前端 el-tree 展示）。
type FolderTreeNode struct {
	Folder   FolderRESP       `json:"folder"`
	Children []FolderTreeNode `json:"children"`
}

// 错误变量；段位 1100-1199。
var (
	ErrFolderNotFound    = pkg.New(1101, "folder not found", "")
	ErrFolderNameEmpty   = pkg.New(1102, "folder name required", "")
	ErrFolderParentCycle = pkg.New(1103, "cannot move folder into itself or its descendant", "")
	ErrFolderNotEmpty    = pkg.New(1104, "folder not empty, move or delete children first", "")
	ErrFolderParentMiss  = pkg.New(1105, "parent folder not found", "")
)
