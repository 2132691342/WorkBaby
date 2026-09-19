/**
 * 错误消费层：Wails 把 Go AppError 序列化成 JS Error 时只保留 message 字符串，
 * 本层按 "[code] human: details" 解析出错误码与详情，解析失败回退统一文案。
 */

import { t } from '@/i18n'

interface AppErrorShape {
  code: number
  message?: string
  details?: string
}

/** 解析不出结构化错误时的兜底文案（随语言切换）。 */
function fallback(): string {
  return t('common.failRetry')
}

export function errorMessage(e: unknown): string {
  if (typeof e === 'string') return e
  if (e instanceof Error) {
    const m = e.message
    // Wails 返回的 message 形如 "[3001] provider not ready: <name>"，直接透传
    return m || fallback()
  }
  return fallback()
}

export function errorCode(e: unknown): number | undefined {
  const s = e as Partial<AppErrorShape> | undefined
  if (typeof s?.code === 'number') return s.code
  // 从 message 头部解析 "[3001]"
  if (e instanceof Error) {
    const m = e.message.match(/^\[(\d+)\]/)
    if (m) return Number(m[1])
  }
  return undefined
}

export interface ToastLike {
  error(msg: string, detail?: string): void
  warning(msg: string, detail?: string): void
  success(msg: string): void
}

export function handleError(e: unknown, _opts: Record<number, () => void> = {}): void {
  // 全局兜底：调用方按需 import element-plus ElMessage
  // 为避免在 util 里引 UI 组件，调用方应在 catch 里显式 toast
  console.error('[WorkBaby error]', e)
}
