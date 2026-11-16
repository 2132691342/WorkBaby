// 流式正文的平滑揭示：delta 不直灌 DOM，而是按受控节奏放字——基速约 60 字/s，
// 积压超过约 500ms 的量就在 500ms 内追平，rAF 合帧到 60Hz。
// 不流式（历史消息、收尾后）立即全量返回，不跑任何动画环。
import { computed, onScopeDispose, ref, watch, type ComputedRef, type Ref } from 'vue'

/** 基速（字/秒）：跟得上普通阅读速度，又不至于让人等字。 */
const BASE_CHARS_PER_SEC = 60
/** 积压上限（基速×0.5s）：超过就按 500ms 追平加速，显示最多落后半秒。 */
const MAX_LAG_CHARS = 30
/** 超长源不做逐字揭示：大段旧文重放走一遍动画只是负担。 */
const MAX_SMOOTH_CHARS = 32 * 1024
/** 合帧下限：高刷屏上也只让渲染管线每秒动 60 次。 */
const FRAME_MS = 1000 / 60

/**
 * 平滑揭示 source 的前缀。streaming 为假或超长源时等价于 source 本身。
 * 卸载与收尾都立即补全剩余文本——读者不该被留在半句上。
 */
export function useSmoothText(source: Ref<string>, streaming: Ref<boolean>): ComputedRef<string> {
  // 揭示到哪：初值即全文（挂载时已有的内容一次性可见，只有后续增长走节奏）。
  const revealed = ref(source.value.length)
  let rafId = 0
  let lastFrame = 0
  let fractional = 0

  function stopLoop(): void {
    if (rafId) cancelAnimationFrame(rafId)
    rafId = 0
    lastFrame = 0
    fractional = 0
  }

  function flush(): void {
    revealed.value = source.value.length
    stopLoop()
  }

  function tick(now: number): void {
    rafId = 0
    const backlog = source.value.length - revealed.value
    if (!streaming.value || backlog <= 0) {
      lastFrame = 0
      fractional = 0
      return
    }
    const elapsed = now - lastFrame
    if (elapsed < FRAME_MS) {
      rafId = requestAnimationFrame(tick)
      return
    }
    lastFrame = now
    const speed = backlog > MAX_LAG_CHARS ? backlog / 0.5 : BASE_CHARS_PER_SEC
    // dt 封顶 100ms：切后台再回来不能一帧吐几千字。
    const exact = fractional + speed * (Math.min(elapsed, 100) / 1000)
    const advance = Math.floor(exact)
    fractional = exact - advance
    if (advance > 0) revealed.value = Math.min(revealed.value + advance, source.value.length)
    rafId = requestAnimationFrame(tick)
  }

  function wake(): void {
    if (!streaming.value) return
    if (source.value.length <= revealed.value) return
    if (source.value.length > MAX_SMOOTH_CHARS) {
      revealed.value = source.value.length
      return
    }
    if (rafId === 0) {
      lastFrame = performance.now()
      rafId = requestAnimationFrame(tick)
    }
  }

  watch(source, wake)
  watch(streaming, (live) => {
    if (live) wake()
    else flush()
  })
  onScopeDispose(stopLoop)

  return computed(() => {
    const text = source.value
    if (!streaming.value || text.length > MAX_SMOOTH_CHARS) return text
    // 不在 UTF-16 代理对中间切开。
    let end = Math.min(revealed.value, text.length)
    if (end > 0 && end < text.length) {
      const code = text.charCodeAt(end - 1)
      if (code >= 0xd800 && code <= 0xdbff) end = Math.min(end + 1, text.length)
    }
    return text.slice(0, end)
  })
}
