// 全局 ID 生成：ULID 大写 + 领域前缀（单机单用户，不需要用户维度 id）。
package domain

// 主键前缀：ULID 带前缀便于日志与排障时一眼看出实体类别。
// 会话与消息的编号由 PI 生成，这里只留 WorkBaby 自己落库的两类。
const (
	PrefixRun  = "RUN"
	PrefixTrip = "TRACE"
)

// ContractVersion 前后端契约版本。改动 RESP 字段或接口路径必须同步改前端 types/api.ts 并 +1。
const ContractVersion = 7
