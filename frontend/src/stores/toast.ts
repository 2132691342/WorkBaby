// 全局操作反馈：成功一句、失败一句，三秒半消失。
// 指针停在提示上时不计时：提示里的路径与报错原文要能选中复制。
import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface Toast {
  id: number
  text: string
  kind: 'ok' | 'bad' | 'info'
  /** 剩余寿命（毫秒）；暂停时由它记住，恢复时续上 */
  remain: number
  /** 到期绝对时刻；0 = 已被指针按住，不计时 */
  deadline: number
}

const LIFE = 3500
/** 松开指针后至少再给的阅读时间：刚滑过就消失同样是「看不清」 */
const MIN_RESUME = 1500

export const useToastStore = defineStore('toast', () => {
  const items = ref<Toast[]>([])
  let seq = 0
  let ticker: ReturnType<typeof setInterval> | undefined

  function push(text: string, kind: Toast['kind'] = 'info') {
    const t: Toast = { id: ++seq, text, kind, remain: LIFE, deadline: Date.now() + LIFE }
    items.value.push(t)
    // 最多同屏 4 条，旧的先走
    if (items.value.length > 4) items.value.shift()
    startTicker()
  }

  function startTicker() {
    if (ticker) return
    ticker = setInterval(() => {
      const now = Date.now()
      items.value = items.value.filter((t) => t.deadline === 0 || now < t.deadline)
      if (!items.value.length) stopTicker()
    }, 200)
  }
  function stopTicker() {
    if (!ticker) return
    clearInterval(ticker)
    ticker = undefined
  }

  /** 指针进出提示：进 = 停表，出 = 续表（续表至少 MIN_RESUME） */
  function hold(id: number, on: boolean) {
    const t = items.value.find((x) => x.id === id)
    if (!t) return
    if (on) {
      if (t.deadline) t.remain = Math.max(t.deadline - Date.now(), 0)
      t.deadline = 0
      return
    }
    if (!t.deadline) t.deadline = Date.now() + Math.max(t.remain, MIN_RESUME)
  }

  function dismiss(id: number) {
    items.value = items.value.filter((t) => t.id !== id)
    if (!items.value.length) stopTicker()
  }

  const ok = (text: string) => push(text, 'ok')
  const bad = (text: string) => push(text, 'bad')
  const info = (text: string) => push(text, 'info')

  return { items, push, dismiss, hold, ok, bad, info }
})
