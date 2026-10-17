// 项目架构图聚合根：本地静态分析的图数据（specs/21）。不落库，每次请求重算。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrArchDirMissing   = pkg.New("9401", "这个目录不存在或读不了", "")
	ErrArchNoCode       = pkg.New("9402", "这个目录里没有可解析的代码文件", "")
	ErrArchTimeout      = pkg.New("9403", "分析超时，结果可能不全", "")
	ErrArchTooManyFiles = pkg.New("9404", "文件太多（超过 10 万），分析不了", "")
)

// ArchNode 图的一个节点 = 一个目录（模块）。
type ArchNode struct {
	ID       string `json:"id"`       // 相对路径（"" = 根）
	Label    string `json:"label"`    // 目录名
	Files    int    `json:"files"`    // 直接文件数
	Lines    int    `json:"lines"`    // 直接行数合计
	Indegree int    `json:"indegree"` // 被依赖次数（不含自身）
	Depth    int    `json:"depth"`    // 依赖深度：入口 0，被入口依赖的 1 …
}

// ArchEdge 一条依赖边：from 依赖 to。
type ArchEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ArchCoverage 解析覆盖率：把「哪些没解析出来」画在界面上，
// 比画一张貌似完整的图负责。
type ArchCoverage struct {
	Parsed  int `json:"parsed"`  // 成功解析出 import 的文件
	Dynamic int `json:"dynamic"` // 动态 / 无法静态解析的引用
	Skipped int `json:"skipped"` // 不认识的语言（文件级索引）
}

// ArchGraph 一张架构图。
type ArchGraph struct {
	Root     string       `json:"root"`
	Nodes    []ArchNode   `json:"nodes"`
	Edges    []ArchEdge   `json:"edges"`
	Coverage ArchCoverage `json:"coverage"`
}

// ArchEntrypointVO 一个入口文件。
type ArchEntrypointVO struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"` // main / index / app / config
	Label string `json:"label"`
}

// AnalyzeArchREQ 分析请求：path 为空用当前工作目录。
type AnalyzeArchREQ struct {
	Path string `json:"path"`
}
