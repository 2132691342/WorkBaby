// Package domain 是业务聚合根：每个聚合根一个文件，含 DO/DTO/REQ/VO/RESP/枚举/常量/错误变量。
// 约束：不 import 任何上层（仅可 import pkg）；跨边界时间字段一律 int64 毫秒；
// 枚举常量集中声明；错误变量以 Err* 命名。
package domain

// ID 前缀常量（ULID 前缀 SCREAMING_SNAKE）；上限 16 个字符，列存 64。
const (
	IDProvider      = "PROVIDER"
	IDSession       = "SESSION"
	IDMessage       = "MESSAGE"
	IDMessageBlock  = "BLOCK"
	IDTodo          = "TODO"
	IDSystemSetting = "SYS"

	IDSkill          = "SKILL"
	IDMcpServer      = "MCP"
	IDKnowledgeDoc   = "DOC"
	IDKnowledgeChunk = "CHUNK"
	IDSprite         = "SPRITE"
	IDFolder         = "FOLDER"
	IDFile           = "FILE"

	IDAgentProfile = "AGENT"
	IDUserCommand  = "UCMD"
)

// LocalUserID 本机单用户固定 ID；与 doc 17 auth §1 一致。
const LocalUserID = "local"

// DefaultTenant 默认租户；MVP 单租户。
const DefaultTenant = "default"

// ContractVersion 前端 ↔ 后端 API 契约版本。
//
// 前端 `frontend/src/src/api/contract.ts` 的同名常量必须与此一致，
// 由 `scripts/check-boundaries.ps1` 双写校验；不一致时后端经 `GET /api/v1/meta/contract`
// 暴露本值，前端启动比对后显式提示，而非静默 404。
const ContractVersion = 2
