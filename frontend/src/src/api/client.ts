import { apiGet as httpGet, apiPost as httpPost } from './http'

/**
 * 轻量 API 客户端（双主机 HTTP 版）。
 *
 * <p>stores 仍以 `/api/v1/...` 形态调用；实际请求走 gin HTTP（见 http.ts）。
 * 本文件保留历史 `apiGet/apiPost` 名字是为了让原 store 不动编译；
 * 路由表退化为「端点白名单校验」——未接线的路径抛 404 提示，store 自行 catch。
 */

const KNOWN_PREFIXES = [
  '/api/v1/chat/',
  '/api/v1/ai-provider',
  '/api/v1/settings',
  '/api/v1/kv/',
  '/api/v1/skills',
  '/api/v1/agent-profiles',
  '/api/v1/mcp/',
  '/api/v1/tools',
  '/api/v1/kdocs',
  '/api/v1/memory/',

  '/api/v1/pet/',
  '/api/v1/folders',
  '/api/v1/files',
  '/api/v1/meta/',
  '/api/v1/dashboard',
  '/api/v1/docs',
  '/api/v1/admin',
  '/api/v1/tasks',
  '/api/v1/trust',
  '/api/v1/wiki/',
  '/api/v1/hooks'
]

function assertKnown(path: string): void {
  const ok = KNOWN_PREFIXES.some((p) => path === p || path.startsWith(p))
  if (!ok) throw new Error(`[4040] ${path} not wired in Go backend`)
}

export async function apiGet<T>(path: string): Promise<T> {
  assertKnown(path)
  return httpGet<T>(path)
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  assertKnown(path)
  return httpPost<T>(path, body)
}

/**
 * multipart 文件上传：双主机下用文件对话框 + HTTP POST（见各域具名 API）。
 */
export async function apiUpload<T>(_path: string, _file: File): Promise<T> {
  throw new Error('[4041] file upload: use Wails dialog + named endpoint instead')
}

export type { ApiResponse } from '@/types/api'
export { errorCode, errorMessage } from '@/utils/error'
