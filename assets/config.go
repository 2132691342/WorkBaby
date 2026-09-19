// Package assets 是编译期嵌入的静态资源：内置 Skill、文档与用户配置模板。
package assets

import "embed"

// _ 仅用于满足 go:embed 编译期导入约束；模板资源使用下方的字节变量。
var _ embed.FS

// ModelConfig 是首启生成到用户数据目录的 model.json 模板。
//
//go:embed config/model.json
var ModelConfig []byte

// MCPConfig 是首启生成到用户数据目录的 mcp.json 模板。
//
//go:embed config/mcp.json
var MCPConfig []byte
