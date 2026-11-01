// HTTP 客户端：基址注入、统一响应解包与错误抛出。
// 浏览器层错误翻成人话时走 i18n：错误文案也是界面文案。
import axios from 'axios'
import { ref } from 'vue'
import { i18n } from '../i18n'
import type { Resp } from '../types/api'

// 端口由 app:ready 注入；开发模式走本地代理。
let baseURL = ''

// 响应式副本：SSE 的订阅时机依赖「端口何时可用」，
// 拿普通变量去 watch 等不到变化，握手晚于挂载时事件流就再也建不起来。
const baseURLRef = ref('')

export function setBaseURL(port: number) {
  baseURL = `http://127.0.0.1:${port}/api/v1`
  baseURLRef.value = baseURL
}

export function getBaseURL() {
  return baseURL
}

export function getBaseURLRef() {
  return baseURLRef
}

// 120 秒：大 PDF 建索引、拉取上游模型列表、导入技能都可能跑过 30 秒；
// 超时只砍断等待，不打断后端任务，用户一重试就是重复索引/重复导入。
const http = axios.create({ timeout: 120000 })

http.interceptors.request.use((cfg) => {
  if (baseURL && !cfg.url?.startsWith('http')) {
    cfg.baseURL = baseURL
  }
  return cfg
})

export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  try {
    const res = await http.get<Resp<T>>(url, { params })
    return unwrap(res.data)
  } catch (e) {
    throw humanize(e)
  }
}

export async function post<T>(url: string, body?: unknown): Promise<T> {
  try {
    const res = await http.post<Resp<T>>(url, body)
    return unwrap(res.data)
  } catch (e) {
    throw humanize(e)
  }
}

// 浏览器层面的失败（断连/被拦）axios 只会说 Network Error，
// 不翻成人话，用户连该找谁都不知道。
function humanize(e: unknown): Error {
  const t = i18n.global.t
  if (e && typeof e === 'object' && 'isAxiosError' in e) {
    const err = e as unknown as { response?: { status: number }; code?: string; message: string }
    if (err.response) {
      return new Error(t('net.badStatus', { status: err.response.status }))
    }
    if (err.code === 'ECONNABORTED') {
      return new Error(t('net.timeout'))
    }
    return new Error(t('net.unreachable', { base: getBaseURL() || t('net.noHandshake') }))
  }
  return e instanceof Error ? e : new Error(String(e))
}

// 业务错误带上 code：调用方要按码分流（审批 4102「已失效」要把卡片摘掉，
// 其他失败要保留卡片供重试，两者不能只看 message 猜）。
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly code: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function unwrap<T>(payload: Resp<T>): T {
  const t = i18n.global.t
  if (!payload) throw new Error(t('net.emptyBody'))
  if (payload.code !== '0') throw new ApiError(payload.message || t('net.requestFailed'), payload.code)
  return payload.data
}

export default http
