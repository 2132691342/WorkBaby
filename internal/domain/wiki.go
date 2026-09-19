// Package domain 本文件：Wiki 仓库导读聚合（无独立表；导读随扫随生成）。
package domain

import "WorkBaby/internal/pkg"

// WikiFileSummaryRESP 文件行：路径 + 行数 + 一句话摘要（首部注释 / 首个声明）。
type WikiFileSummaryRESP struct {
	Path    string `json:"path"`
	Lang    string `json:"lang"`
	Lines   int    `json:"lines"`
	Summary string `json:"summary"`
}

// WikiNodeRESP 目录树节点。
type WikiNodeRESP struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Type     string          `json:"type"` // dir / file
	Lines    int             `json:"lines"`
	Lang     string          `json:"lang,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Children []*WikiNodeRESP `json:"children,omitempty"`
}

// WikiOverviewRESP 导读首屏：仓库画像 + 语言统计 + 入口 + 目录树。
type WikiOverviewRESP struct {
	Root       string                `json:"root"`
	Name       string                `json:"name"`
	TotalFiles int                   `json:"total_files"`
	TotalLines int                   `json:"total_lines"`
	Langs      []WikiLangRESP        `json:"langs"`
	Entries    []WikiFileSummaryRESP `json:"entries"` // README / main / go.mod 等入口
	Tree       []*WikiNodeRESP       `json:"tree"`
}

// WikiLangRESP 语言统计行。
type WikiLangRESP struct {
	Lang  string `json:"lang"`
	Files int    `json:"files"`
	Lines int    `json:"lines"`
}

// WikiPageRESP 详情页：dir 页带子项清单，file 页带正文（截断到 512KB）。
type WikiPageRESP struct {
	Path      string                `json:"path"`
	Type      string                `json:"type"`
	Lang      string                `json:"lang,omitempty"`
	Lines     int                   `json:"lines"`
	Summary   string                `json:"summary"`
	Children  []WikiFileSummaryRESP `json:"children,omitempty"`
	Content   string                `json:"content,omitempty"`
	Truncated bool                  `json:"truncated,omitempty"`
}

// 包级错误变量；错误码段位 8800（Wiki 导读）。
var (
	ErrWikiNotWorkspace = pkg.New(8801, "wiki scan failed: workspace unavailable", "")
	ErrWikiPageNotFound = pkg.New(8802, "wiki page not found", "")
)
