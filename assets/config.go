// Package assets 是编译期嵌入的静态资源：内置 Skill、用户文档与文档目录。
package assets

import "embed"

// Skills 内置 Skill 目录（assets/skills/{name}/SKILL.md）。
//
//go:embed skills
var Skills embed.FS

// Docs 内置用户文档目录（assets/docs/*.md；首行 # 标题作为 title）。
//
//go:embed docs
var Docs embed.FS
