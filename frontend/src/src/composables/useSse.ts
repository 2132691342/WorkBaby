import { onUnmounted, ref, watch } from 'vue'
import { getBaseURL, getBaseURLRef } from '../api/http'
import type { ServerEvent } from '../types/api'

// 连接状态全站唯一：标题栏小灯与消息流读同一份，断线只换颜色不弹提示。
export const sseConnected = ref(false)

// 订阅维度是会话：一次 run 的全部事件都走这个通道。两件事必须自己扛——
// 端口握手是异步的（握手前 baseURL 为空串，订阅会静默失败），EventSource 断开不自恢复。
export function useSse(
  sessionId: () => string | null,
  onEvent: (env: ServerEvent) => void,
  /** 断过又接上时回调：断开期间可能丢了 done/error，要拉权威快照对账 */
  onReconnect?: () => void,
) {
  let source: EventSource | null = null
  let currentSid = ''
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  // 断过一次才需要对账：首次连上不算，否则每次打开会话都白刷一遍。
  let dropped = false
  // 最后收到的 seq：重建连接时带回服务端，断开期间的事件（含 done/error）会重放补齐。
  // 慢消费者被断开时就是靠它对账，不然界面会永远停在「运行中」。
  let lastSeq = 0

  const close = () => {
    if (retryTimer) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
    if (source) {
      source.close()
      source = null
    }
    sseConnected.value = false
  }

  const scheduleRetry = () => {
    if (retryTimer || !currentSid) return
    // 退避到 3 秒封顶：断线期间用户往往正在发消息，重连要快但不能打转。
    retryTimer = setTimeout(() => {
      retryTimer = null
      open(currentSid)
    }, 800)
  }

  const open = (sid: string) => {
    close()
    if (sid !== currentSid) lastSeq = 0
    currentSid = sid
    const base = getBaseURL()
    if (!base) {
      // 端口还没握手好：等一小会儿再试，而不是放弃这条流。
      scheduleRetry()
      return
    }
    const resume = lastSeq > 0 ? `&last_event_id=${lastSeq}` : ''
    const es = new EventSource(`${base}/events?session_id=${encodeURIComponent(sid)}${resume}`)
    es.onopen = () => {
      sseConnected.value = true
      if (dropped) {
        dropped = false
        onReconnect?.()
      }
    }
    es.onerror = () => {
      sseConnected.value = false
      dropped = true
      // EventSource 自带重连，但它不会带上我们要的 session 维度语义，
      // 且服务端可能已判定为慢客户端而移除订阅——统一自己重连。
      es.close()
      if (source === es) source = null
      scheduleRetry()
    }
    const names = [
      'chat:start',
      'chat:delta',
      'chat:tool_start',
      'chat:tool_end',
      'chat:approval',
      'chat:compressed',
      'chat:user',
      'chat:context',
      'chat:done',
      'chat:stopped',
      'chat:error',
      'chat:gap',
    ]
    for (const name of names) {
      es.addEventListener(name, (ev) => {
        try {
          const env = JSON.parse((ev as MessageEvent).data) as ServerEvent
          if (env.seq > lastSeq) lastSeq = env.seq
          onEvent(env)
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
      else {
        currentSid = ''
        close()
      }
    },
    { immediate: true },
  )

  // 端口握手晚于组件挂载时，补一次订阅。
  watch(getBaseURLRef, (base) => {
    if (base && currentSid && !source) open(currentSid)
  })

  onUnmounted(close)
  return { connected: sseConnected, close }
}
