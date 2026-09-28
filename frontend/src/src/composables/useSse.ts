import { onUnmounted, ref, watch } from 'vue'
import { getBaseURL } from '../api/http'
import type { ServerEvent } from '../types/api'

// 连接状态全站唯一：标题栏小灯与消息流读同一份，断线只换颜色不弹提示。
export const sseConnected = ref(false)

// 订阅维度是会话：一次 run 内的全部事件都走这个通道。
export function useSse(sessionId: () => string | null, onEvent: (env: ServerEvent) => void) {
  let source: EventSource | null = null

  const close = () => {
    if (source) {
      source.close()
      source = null
    }
    sseConnected.value = false
  }

  const open = (sid: string) => {
    close()
    const base = getBaseURL()
    if (!base) return
    const es = new EventSource(`${base}/events?session_id=${encodeURIComponent(sid)}`)
    es.onopen = () => (sseConnected.value = true)
    es.onerror = () => (sseConnected.value = false)
    const names = [
      'chat:start',
      'chat:delta',
      'chat:tool_start',
      'chat:tool_end',
      'chat:approval',
      'chat:compressed',
      'chat:context',
      'chat:done',
      'chat:error',
      'chat:gap',
    ]
    for (const name of names) {
      es.addEventListener(name, (ev) => {
        try {
          onEvent(JSON.parse((ev as MessageEvent).data) as ServerEvent)
        } catch {
          // 坏帧直接丢弃，不打断整条流
        }
      })
    }
    source = es
  }

  watch(
    sessionId,
    (sid) => {
      if (sid) open(sid)
      else close()
    },
    { immediate: true },
  )

  onUnmounted(close)
  return { connected: sseConnected, close }
}
