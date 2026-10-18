// SSE 订阅：一条连接跟一个会话目标，断线重连带 Last-Event-ID，丢帧时回调对账。
import { onUnmounted, ref, watch } from 'vue'
import { getBaseURL, getBaseURLRef } from '../api/http'
import type { ServerEvent } from '../types/api'

// 连接状态全站唯一：标题栏小灯与消息流读同一份，断线只换颜色不弹提示。
export const sseConnected = ref(false)
// 是否存在订阅目标：连接跟着「当前会话」走，且不随视图卸载而断开。
// 没有目标时（首启还没有任何会话）标题栏不亮灯——无流可说，不谎报断线。
export const sseSubscribed = ref(false)

type EventSink = (env: ServerEvent) => void
type ReconnectSink = () => void

// 连接是模块级单例：SSE 承载运行态事件，离开对话页时 run 还在跑，
// 连接断掉会让标题栏误报「正在重连」；事件消费随组件注册 / 注销。
const sinks = new Set<EventSink>()
const reconnectSinks = new Set<ReconnectSink>()

// 关注的事件名清单：订阅粒度按名字注册监听，后端加事件必须同步这里。
const names = [
  'chat:start',
  'chat:delta',
  'chat:tool_start',
  'chat:tool_end',
  'chat:compressed',
  'chat:user',
  'chat:context',
  'chat:done',
  'chat:stopped',
  'chat:error',
  'chat:gap',
  // PI 正在自动重试：没有这个监听，重试期间界面看上去就是卡死。
  'chat:retry',
  // 会话被切走（回退分叉 / 工作区重建进程）：必须拉新快照。
  'chat:switched',
  // 环境安装进度。
  'chat:setup',
  // 引擎里的扩展在问用户（confirm / select / input）。
  'chat:ask',
  'chat:ask_closed',
]

let source: EventSource | null = null
let currentSid = ''
let retryTimer: ReturnType<typeof setTimeout> | null = null
// 断过一次才需要对账：首次连上不算，否则每次打开会话都白刷一遍。
let dropped = false
// 最后收到的 seq：重建连接时带回服务端，断开期间的事件（含 done/error）会重放补齐。
// 慢消费者被断开时就是靠它对账，不然界面会永远停在「运行中」。
let lastSeq = 0

// channel 是「连一条会话的流」的唯一工厂：seq 对账、断线自管、坏帧丢弃都收在这里。
// 主连接（当前会话）与附加通道（团队执行观测等）共用，不各写一套重连逻辑。
function openChannel(
  sid: string,
  resume: number,
  onOpen: () => void,
  onError: () => void,
  onEvent: EventSink,
): { es: EventSource; close: () => void } | null {
  const base = getBaseURL()
  if (!base) return null
  const q = resume > 0 ? `&last_event_id=${resume}` : ''
  const es = new EventSource(`${base}/events?session_id=${encodeURIComponent(sid)}${q}`)
  es.onopen = onOpen
  es.onerror = onError
  for (const name of names) {
    es.addEventListener(name, (ev) => {
      try {
        const env = JSON.parse((ev as MessageEvent).data) as ServerEvent
        onEvent(env)
      } catch {
        // 坏帧直接丢弃，不打断整条流
      }
    })
  }
  return { es, close: () => es.close() }
}

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

// 退避到 3 秒封顶：断线期间用户往往正在发消息，重连要快但不能打转。
// 固定 800ms 在服务端长时间没起来时会一秒多敲一次，白白刷日志与 CPU。
let retryDelay = 800
const scheduleRetry = () => {
  if (retryTimer || !currentSid) return
  retryTimer = setTimeout(() => {
    retryTimer = null
    open(currentSid)
  }, retryDelay)
  retryDelay = Math.min(retryDelay * 2, 3000)
}

const open = (sid: string) => {
  close()
  sseSubscribed.value = true
  if (sid !== currentSid) lastSeq = 0
  currentSid = sid
  const ch = openChannel(
    sid,
    lastSeq,
    () => {
      sseConnected.value = true
      retryDelay = 800
      if (dropped) {
        dropped = false
        for (const cb of reconnectSinks) cb()
      }
    },
    () => {
      sseConnected.value = false
      dropped = true
      // EventSource 自带重连，但它不会带上我们要的 session 维度语义，
      // 且服务端可能已判定为慢客户端而移除订阅——统一自己重连。
      if (source) {
        source.close()
        source = null
      }
      scheduleRetry()
    },
    (env) => {
      if (env.seq > lastSeq) lastSeq = env.seq
      for (const sink of sinks) sink(env)
    },
  )
  if (!ch) {
    // 端口还没握手好：等一小会儿再试，而不是放弃这条流。
    scheduleRetry()
    return
  }
  source = ch.es
}

/** 供壳层或测试显式接管订阅目标；业务侧走 useSse。 */
export function setSseSession(sid: string | null) {
  if (sid) open(sid)
  else {
    currentSid = ''
    close()
    sseSubscribed.value = false
  }
}

/** 订阅当前会话的事件流。连接全站单例：重复订阅不重开，组件卸载只摘处理器——
 *  run 不在对话页时也要继续跑。 */
export function useSse(
  sessionId: () => string | null,
  onEvent: (env: ServerEvent) => void,
  /** 断过又接上时回调：断开期间可能丢了 done/error，要拉权威快照对账 */
  onReconnect?: () => void,
) {
  const sink: EventSink = onEvent
  const reSink = onReconnect
  sinks.add(sink)
  if (reSink) reconnectSinks.add(reSink)

  watch(
    sessionId,
    (sid) => {
      if (sid && sid === currentSid && source) return
      setSseSession(sid)
    },
    { immediate: true },
  )

  // 端口握手晚于组件挂载时，补一次订阅。
  watch(getBaseURLRef, (base) => {
    if (base && currentSid && !source) open(currentSid)
  })

  onUnmounted(() => {
    sinks.delete(sink)
    if (reSink) reconnectSinks.delete(reSink)
  })
  return { connected: sseConnected }
}

/** 订阅一条附加会话流（子代理观测用）：独立连接、随组件卸载关闭。
 *  与主连接并存——主连接跟「当前会话」走，这条跟「正在观测的子代理」走。 */
export function useSseChannel(
  sessionId: () => string | null,
  onEvent: (env: ServerEvent) => void,
  onReconnect?: () => void,
) {
  const connected = ref(false)
  let es: EventSource | null = null
  let sid = ''
  let seq = 0
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let dropped = false

  const cleanup = () => {
    if (retryTimer) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
    if (es) {
      es.close()
      es = null
    }
    connected.value = false
  }
  const retry = () => {
    if (retryTimer || !sid) return
    retryTimer = setTimeout(() => {
      retryTimer = null
      start(sid)
    }, 1200)
  }
  const start = (target: string) => {
    cleanup()
    sid = target
    if (!target) return
    const ch = openChannel(
      target,
      seq,
      () => {
        connected.value = true
        if (dropped) {
          dropped = false
          onReconnect?.()
        }
      },
      () => {
        connected.value = false
        dropped = true
        cleanup()
        retry()
      },
      (env) => {
        if (env.seq > seq) seq = env.seq
        onEvent(env)
      },
    )
    if (!ch) {
      retry()
      return
    }
    es = ch.es
  }

  watch(
    sessionId,
    (target) => {
      if (target === sid) return
      seq = 0
      dropped = false
      start(target || '')
    },
    { immediate: true },
  )
  watch(getBaseURLRef, (base) => {
    if (base && sid && !es) start(sid)
  })

  onUnmounted(cleanup)
  return { connected }
}
