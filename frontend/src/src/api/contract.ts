/**
 * 前后端 API 契约版本。
 *
 * 必须与后端 `internal/domain/contract.go` 的 `ContractVersion` 保持一致，
 * 由 `scripts/check-boundaries.ps1` 双写校验；不一致时显式提示而非静默 404。
 */
export const UI_API_CONTRACT_VERSION = 2
