// 前端与后端的契约类型总出口。一个域一个文件，字段名与 Go 侧的 json tag 一一对应。
// 改字段形态必须同时把 backend/service.ContractVersion +1。
// 命名纪律：字段一律带业务前缀（session_title 而不是 title）——模板里一眼看出读的是哪个对象。

export * from './api-core'
export * from './api-chat'
export * from './api-data'
export * from './api-workspace'
