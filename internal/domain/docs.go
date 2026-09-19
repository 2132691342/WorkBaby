package domain

import "WorkBaby/internal/pkg"

// ErrDocNotFound 内置文档不存在。
var ErrDocNotFound = pkg.New(2016, "doc not found", "")

// DocItemRESP 内置文档列表项（GET /api/v1/docs）。
type DocItemRESP struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// DocDetailRESP 内置文档详情（GET /api/v1/docs/:name）。
type DocDetailRESP struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// AdminOverviewRESP 管理后台概览（GET /api/v1/admin/overview）。
// 聚合版本 / 环境 / 数据目录 + 各领域计数。
type AdminOverviewRESP struct {
	Version        string `json:"version"`
	Phase          string `json:"phase"`
	Home           string `json:"home"`
	DBEnabled      bool   `json:"db_enabled"`
	ToolCount      int    `json:"tool_count"`
	Providers      int    `json:"providers"`
	Sessions       int64  `json:"sessions"`
	Messages       int64  `json:"messages"`
	MemoryEpisodes int64  `json:"memory_episodes"`
	KnowledgeDocs  int64  `json:"knowledge_docs"`
}
