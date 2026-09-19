import { errorMessage } from '@/utils/error'

/**
 * HTTP API 层（双主机，doc/15 §3）。
 *
 * <p>业务 API 走 gin HTTP（127.0.0.1 随机端口）；端口由 `app:ready` 注入并缓存到 localStorage。
 * 统一响应 `{code, message, data}`：code=0 返回 data，非 0 抛 Error(message)。
 */
const PORT_KEY = 'wb.serverPort'

/** 后端 gin 监听的 origin（不含 /api/v1 前缀）。 */
let origin = ''

/** 端口注入入口（App.vue 收到 app:ready 后调用）。 */
export function initApi(port: string | number): void {
  origin = `http://127.0.0.1:${port}`
  try {
    localStorage.setItem(PORT_KEY, String(port))
  } catch {
    // ignore: 非安全上下文可能不可写
  }
}

/** 惰性取 origin；未初始化时尝试从 localStorage 恢复。 */
function getOrigin(): string {
  if (origin) return origin
  try {
    const p = localStorage.getItem(PORT_KEY)
    if (p) origin = `http://127.0.0.1:${p}`
  } catch {
    // ignore
  }
  return origin
}

/** 当前端口（调试/展示用）；未初始化返回 0。 */
export function getServerPort(): number {
  const m = /^http:\/\/127\.0\.0\.1:(\d+)$/.exec(getOrigin())
  return m ? Number(m[1]) : 0
}

/**
 * SSE 等非 fetch 通道的基础地址（含 /api/v1）。
 * 与 fetch 通道同源同前缀，避免两侧拼接规则漂移。
 */
export function getApiBase(): string {
  const o = getOrigin()
  return o ? `${o}/api/v1` : ''
}

async function request<T>(path: string, method: 'GET' | 'POST', body?: unknown): Promise<T> {
  const b = getOrigin()
  if (!b) throw new Error('[4000] server not initialized (missing serverPort)')
  const res = await fetch(b + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body)
  })
  return unwrap<T>(res, path, method)
}

async function unwrap<T>(res: Response, path: string, method: string): Promise<T> {
  let payload: { code: number | string; message: string; data: T; details?: string }
  try {
    payload = (await res.json()) as typeof payload
  } catch {
    // 非 JSON 响应（404 / 路由未命中 / 网关错误）：带上 method+path 便于定位契约漂移
    throw new Error(`[${res.status}] ${method} ${path} ${res.statusText || 'server error'}`.trim())
  }
  // code 兼容数字 0 与字符串 "0"（旧契约）
  const ok = payload.code === 0 || payload.code === '0'
  if (!ok) {
    const e = new Error(payload.message || 'operation failed')
    ;(e as { code?: number }).code = Number(payload.code)
    throw e
  }
  return payload.data
}

/** GET 请求（query 直接拼在 path 上）。 */
export async function apiGet<T>(path: string): Promise<T> {
  return request<T>(path, 'GET')
}

/** POST 请求；body 可省略（无参提交）。 */
export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, 'POST', body)
}

/** 与旧 utils/error 兼容的导出。 */
export { errorMessage }
