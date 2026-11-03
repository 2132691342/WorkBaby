// 失败现场：最近几十次请求失败的原话与码。
//
// 为什么要有它：错误码的「下一步」写在文档里，而用户面对的是一条已经消失的提示。
// 这里把每一条失败留在内存里（不写盘、不发送），设置页能逐条看、能一键复制——
// 「刚才那一下到底哪一层出的错」才有地方可查。
import { ref } from 'vue'

/** 一条失败现场。 */
export interface DiagEntry {
  /** 发生时刻（毫秒时间戳） */
  at: number
  /** 错误码；网络失败是 8003 */
  code: string
  /** 给用户看的那句话 */
  message: string
  /** 底层原文；界面上默认折叠，复制时才带上 */
  detail?: string
  /** 请求路径（`POST /chat/send`），定位是哪一次调用 */
  where: string
}

/** 只留最近 50 条：诊断看的是刚才那几十秒，更早的没有查的价值。 */
const LIMIT = 50

const entries = ref<DiagEntry[]>([])
let seq = 0

/** 记一条失败。 */
export function recordFailure(code: string, message: string, detail: string, where: string): void {
  entries.value = [
    { at: Date.now(), code, message, detail: detail || undefined, where },
    ...entries.value,
  ].slice(0, LIMIT)
  seq++
}

/** 清空。 */
export function clearFailures(): void {
  entries.value = []
}

/** 失败现场列表（模块级单例：它不属于任何一屏）。 */
export function useDiagnostics() {
  return { entries, clear: clearFailures, seq: () => seq }
}

/** 把一条失败拼成可粘贴的文本。 */
export function formatFailure(e: DiagEntry): string {
  const time = new Date(e.at).toLocaleString()
  const lines = [`[${time}] ${e.where} → ${e.code} ${e.message}`]
  if (e.detail) lines.push(`  detail: ${e.detail}`)
  return lines.join('\n')
}
