// 一轮对话的秒表：只在跑的时候跳，一秒一格。
// 抽出来的理由：轮内脚标与输入区活动条要显示同一个数字，
// 各自起一个 interval 会看到两份对不上的时间。
import { computed, onBeforeUnmount, ref, watch } from 'vue'

/**
 * 一轮的秒表。
 *
 * active 为真才走：收尾之后停住，读数不再涨——涨着的时间会被当成「还在跑」。
 * startedAt 是毫秒时间戳（0 表示还没有起点，此时不显示）。
 */
export function useTurnClock(active: () => boolean, startedAt: () => number) {
  const now = ref(Date.now())
  let timer: ReturnType<typeof setInterval> | undefined

  watch(
    active,
    (on) => {
      if (on) {
        now.value = Date.now()
        if (timer === undefined) timer = setInterval(() => (now.value = Date.now()), 1000)
      } else if (timer !== undefined) {
        clearInterval(timer)
        timer = undefined
      }
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    if (timer !== undefined) clearInterval(timer)
  })

  /** 已经过去多少毫秒。 */
  const elapsed = computed(() => (startedAt() ? Math.max(0, now.value - startedAt()) : 0))

  /** 秒表文字：60 秒内纯秒，过了带分。 */
  const label = computed(() => fmtElapsed(elapsed.value))

  return { now, elapsed, label }
}

/** 秒表文字。0 返回空串：没有起点时不该显示一个「0s」。 */
export function fmtElapsed(ms: number): string {
  if (!ms) return ''
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`
}
