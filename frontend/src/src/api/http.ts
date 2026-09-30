import axios from 'axios'
import { ref } from 'vue'
import type { Resp } from '../types/api'

// 端口由 app:ready 注入；开发模式下由 vite 代理补齐。
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

const http = axios.create({ timeout: 30000 })

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
  if (e && typeof e === 'object' && 'isAxiosError' in e) {
    const err = e as unknown as { response?: { status: number }; code?: string; message: string }
    if (err.response) {
      return new Error(`本地服务返回了异常状态 ${err.response.status}`)
    }
    if (err.code === 'ECONNABORTED') {
      return new Error('本地服务长时间没有响应，请重试一次')
    }
    return new Error(`连不上本地服务（${getBaseURL() || '尚未握手'}）。请检查是否残留了旧版进程，或代理软件拦截了本机回环地址`)
  }
  return e instanceof Error ? e : new Error(String(e))
}

function unwrap<T>(payload: Resp<T>): T {
  if (!payload) throw new Error('后端没有返回数据')
  if (payload.code !== 0) throw new Error(payload.message || '请求失败')
  return payload.data
}

export default http
