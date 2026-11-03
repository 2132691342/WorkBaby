// 轻提示：一句原因 + 可选下一步 + 可选动作（如「撤销」）。悬停暂停计时。
// 不做成一闪而过的 toast——失败信息消失了就等于没提示。
import { ref } from 'vue'

export interface ToastItem {
  id: number
  level: 'info' | 'ok' | 'warn' | 'error'
  message: string
  detail?: string
  /** 提示上的一个动作（如撤销）：跑完回调就收起提示 */
  action?: { label: string; run: () => void }
}

const items = ref<ToastItem[]>([])
const timers = new Map<number, number>()
let seq = 0

function clear(id: number) {
  const timer = timers.get(id)
  if (timer) window.clearTimeout(timer)
  timers.delete(id)
  items.value = items.value.filter((t) => t.id !== id)
}

function arm(item: ToastItem, ttl: number) {
  timers.set(
    item.id,
    window.setTimeout(() => clear(item.id), ttl),
  )
}

export function useToasts() {
  function push(level: ToastItem['level'], message: string, detail?: string, action?: ToastItem['action']) {
    if (!message) return
    const item: ToastItem = { id: ++seq, level, message, detail, action }
    items.value = [...items.value, item].slice(-4)
    arm(item, level === 'error' ? 8000 : 4200)
    return item.id
  }

  /** 跑一条提示的动作并收起它。 */
  function runAction(item: ToastItem) {
    item.action?.run()
    clear(item.id)
  }

  function dismiss(id: number) {
    clear(id)
  }

  /** 鼠标悬停时暂停倒计时，移开再续 4 秒。 */
  function hold(id: number, paused: boolean) {
    const timer = timers.get(id)
    if (!timer) return
    window.clearTimeout(timer)
    timers.delete(id)
    if (!paused) {
      const item = items.value.find((t) => t.id === id)
      if (item) arm(item, 4000)
    }
  }

  return { items, push, dismiss, hold, runAction }
}
