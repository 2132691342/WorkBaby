// Package assets 是编译期嵌入的静态资源：应用内帮助文档。
package assets

import "embed"

// Docs 内置用户文档目录（assets/docs/*.md；首行 # 标题作为 title）。
//
//go:embed docs
var Docs embed.FS
