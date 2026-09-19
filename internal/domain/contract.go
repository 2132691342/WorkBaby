package domain

// ContractVersion 前端 ↔ 后端 API 契约版本。
//
// 前端 `frontend/src/src/api/contract.ts` 的同名常量必须与此一致，
// 由 `scripts/check-boundaries.ps1` 双写校验；不一致时后端经 `GET /api/v1/meta/contract`
// 暴露本值，前端启动比对后显式提示，而非静默 404。
const ContractVersion = 2
