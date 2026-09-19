import type { ApiResponse, SessionToken } from '@/types/api'

/**
 * 本机会话令牌（Phase 1 stub：Wails 桌面端无需令牌机制）。
 *
 * <p>Java 版：Spring 启动时生成随机 token，前端首次请求先换；同源内存持有。
 * <p>Go 版：Wails 绑定方法本身即绑定到具体的方法调用，没有跨域 token 需要；
 * 鉴权需求为零。本文件作为兼容 shim 保留导出，函数全部 no-op，
 * `isSessionPath` 永远返回 true（"全免鉴权"），旧 store 代码无需改动。
 */

const inflightToken: SessionToken = {
  access_token: '',
  token_type: 'Bearer',
  expires_in: 0,
  user_id: 'local',
  username: 'local'
}

let token: string | null = ''
let _sessionPath = '/api/v1/auth/session'

export function getSessionToken(): string | null { return token }

export function ensureSessionToken(): Promise<string> {
  return Promise.resolve(token ?? '')
}

export function isSessionPath(_path: string): boolean { return true }

export async function fetchSessionTokenInfo(): Promise<ApiResponse<SessionToken>> {
  return { code: 0, message: 'ok', data: inflightToken }
}

// 兼容旧 store 偶尔直接 fetch `/api/v1/auth/session`
export async function primeSessionToken(): Promise<string> {
  return ''
}

export const SESSION_PATH = _sessionPath
