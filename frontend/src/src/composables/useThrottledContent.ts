/**
 * 流式内容节流 composable。
 *
 * <p>把高频写入的 source 节流到固定频率推送给 throttled ref，避免流式期间 markdown / DOMPurify 重复解析与滚动抖动。
 * {@code streaming=true} 时按 {@code intervalMs} 节流；{@code streaming=false} 时立即 flush 终态内容。
 */
import { onUnmounted, ref, watch } from 'vue'
import type { Ref } from 'vue'

/** 流式节流的默认间隔（ms）。 */
export const DEFAULT_STREAMING_INTERVAL_MS = 100

/**
 * 把高频写入的 content 节流到固定频率输出。
 *
 * @param source     原始内容 ref（流式时高频变化）
 * @param streaming  当前是否处于流式状态
 * @param intervalMs 节流间隔，默认 100ms；可传 ref 随内容长度自适应
 * @returns          节流后的内容 ref
 */
export function useThrottledContent(
  source: Ref<string>,
  streaming: Ref<boolean>,
  intervalMs: number | Ref<number> = DEFAULT_STREAMING_INTERVAL_MS
): Ref<string> {
  const intervalOf = (): number =>
    typeof intervalMs === 'number' ? intervalMs : intervalMs.value
  const throttled = ref(source.value)
  const latestRef = ref(source.value)
  const lastFlushRef = ref(0)
  const timerRef = ref<ReturnType<typeof setTimeout> | null>(null) as Ref<ReturnType<typeof setTimeout> | null>

  // 每次 source 变化都更新最新 ref；非流式必须立即推进（终态 content 就地修改，
  // streaming 不会再翻转 → 节流会让用户看到「流式可见、结束后整条消息空白」）。
  watch(source, (v) => {
    latestRef.value = v
    if (!streaming.value) {
      flushNow()
    }
  })

  function flushNow(): void {
    throttled.value = latestRef.value
    lastFlushRef.value = Date.now()
  }

  function clearTimer(): void {
    if (timerRef.value != null) {
      clearTimeout(timerRef.value)
      timerRef.value = null
    }
  }

  watch(streaming, (isStreaming) => {
    if (!isStreaming) {
      clearTimer()
      flushNow()
    } else {
      // streaming 关闭→打开瞬间立即 flush 首帧，避免用户看到空白
      flushNow()
    }
  }, { immediate: true })

  // 流式中监听 source 变化：节流 flush
  watch(source, () => {
    if (!streaming.value) return
    const now = Date.now()
    const elapsed = now - lastFlushRef.value
    const iv = intervalOf()
    if (elapsed >= iv) {
      lastFlushRef.value = now
      throttled.value = latestRef.value
    } else if (timerRef.value == null) {
      timerRef.value = setTimeout(() => {
        timerRef.value = null
        lastFlushRef.value = Date.now()
        throttled.value = latestRef.value
      }, iv - elapsed)
    }
  })

  onUnmounted(() => clearTimer())

  return throttled
}
