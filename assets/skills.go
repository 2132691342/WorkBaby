// Package assets 是编译期嵌入的静态资源：内置 Skill、图标与默认 sprite。
package assets

import "embed"

// Skills 内置 Skill 目录（assets/skills/{name}/SKILL.md）。
//
//go:embed skills
var Skills embed.FS
