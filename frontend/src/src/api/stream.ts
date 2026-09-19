import type { ChatStreamReq, ChatStreamEvent } from '@/types/api'
import { apiPost, apiGet, getApiBase } from './http'
import { onUnmounted } from 'vue'

/**
 * 流式聊天订阅 + 发送：POST /chat/stream 拿 runID，再经 SSE 消费事件。
 * 可靠性三件套：Last-Event-ID 重放、watchdog 假死检测（>2× 心跳周期）、指数退避重连。
 */

export interface StreamHandle {
  promise: Promise<void>
  cancel: () => void
}

/** streamChat 可选项。 */
export interface StreamChatOpts {
  /** 手动重连成功后调用（store 据此重拉会话快照，弥补重放覆盖不到的窗口）。 */
  onReconnect?: () => void
}

/**
 * 归一化后端停止原因 → 前端领域原因（blocks.ts 的 stopReasonText 只认这组值）。
 *
 * <p>为什么必须归一化：历史版本后端 chat:done 同时下发领域 reason（completed/cancelled…）
 * 与 LLM 原始 finish_reason（stop/length/tool_calls…）。原始值一旦直通前端，
 * stopReasonText 不命中 → 兜底文案「已停止生成（部分内容已保留）」在正常结束也误弹。
 * 归一化后未知/平台自定义终因一律按 completed 处理（不制造惊扰横幅）。
 */
function normalizeStopReason(raw: string | null | undefined): string {
  const v = (raw ?? '').trim().toLowerCase()
  switch (v) {
    case 'completed':
    case 'end_turn':
    case 'stop':
    case 'tool_calls':
    case 'function_call':
    case '':
      return 'completed'
    case 'cancelled':
      return 'cancelled'
    case 'max_turns':
      return 'max_turns'
    case 'stagnation':
      return 'stagnation'
    case 'length':
    case 'max_tokens':
    case 'budget_exceeded':
    case 'token_budget':
      return 'token_budget'
    case 'error':
    case 'content_filter':
      return 'error'
    default:
      return 'completed'
  }
}

/** SSE event 载荷 → decoder 事件；返回 null 表示该事件无需下传。 */
export function mapSSEEvent(name: string, data: unknown): ChatStreamEvent | null {
  const p = (data ?? {}) as Record<string, unknown>
  switch (name) {
    case 'chat:stream':
      return { type: 'content', data: p.delta ?? '' }
    case 'chat:thinking':
      return { type: 'thinking', data: p.delta ?? '' }
    case 'chat:turn-start':
      // 第 N 轮开始（N 从 1 计）：长任务里轮次推进是重要的「还在干活」信号
      return { type: 'turn_start', data: { turn: Number(p.turn ?? 0) } }
    case 'chat:checkpoint':
      // 检查点已写入：这是崩溃/中断后续跑的位点，前端记录下来用于续跑说明
      return { type: 'checkpoint', data: { turn: Number(p.turn ?? 0) } }
    case 'chat:stats':
      // 每轮结束累计用量 → decoder 的 stats 分支（setStats）
      return {
        type: 'stats',
        data: {
          input_tokens: p.input_tokens,
          output_tokens: p.output_tokens,
          cache_read_tokens: p.cache_read_tokens,
          cache_creation_tokens: p.cache_creation_tokens,
          total_tokens: p.total_tokens,
          latency_ms: p.latency_ms
        }
      }
    case 'chat:skill':
      // 技能命中：早于首帧正文，前端渲染为执行过程时间线首行
      return {
        type: 'skill',
        data: {
          name: p.name ?? '',
          source: p.source ?? '',
          version: p.version ?? '',
          description: p.description ?? '',
          tools: Array.isArray(p.tools) ? (p.tools as string[]) : [],
          injected_chars: typeof p.injected_chars === 'number' ? p.injected_chars : 0
        }
      }
    case 'chat:tool': {
      // activity 由工具自述（"正在编辑 app.ts"）：前端直接展示动词，不维护名称映射。
      // 空值不写进载荷，避免下游为「空字符串」再做一次判断。
      const activity = typeof p.activity === 'string' ? p.activity : ''
      return {
        type: 'tool_call',
        data: {
          id: p.id,
          name: p.name,
          args: p.arguments,
          agent: p.agent ?? '',
          ...(activity ? { activity } : {})
        }
      }
    }
    case 'chat:tool-result':
      return {
        type: 'tool_result',
        data: {
          id: p.id,
          name: p.name,
          output: p.content ?? '',
          state: p.error ? 'error' : 'success',
          agent: p.agent ?? '',
          // 后端计量的真实执行耗时；缺失时 decoder 退回本地 started_at 差值
          duration_ms: typeof p.duration_ms === 'number' ? p.duration_ms : undefined,
          // 工具结构化结果（knowledge_search 命中列表等）：来源卡直接消费
          data: (p.data ?? undefined) as Record<string, unknown> | undefined
        }
      }
    case 'chat:subagent-start':
      return { type: 'subagent_start', data: { sub_run_id: p.sub_run_id, agent: p.agent ?? '' } }
    case 'chat:subagent-done':
      return { type: 'subagent_done', data: { sub_run_id: p.sub_run_id, agent: p.agent ?? '', reason: p.reason ?? '' } }
    case 'chat:subagent-error':
      return { type: 'subagent_error', data: { sub_run_id: p.sub_run_id, agent: p.agent ?? '', message: p.message ?? '' } }
    case 'chat:approval':
      return {
        type: 'tool_approval_request',
        data: {
          id: p.id,
          command: p.command ?? '',
          reason: p.reason ?? '',
          risk: p.risk === 'irreversible' ? 'irreversible' : p.risk === 'input_required' ? 'input_required' : 'needs_approval',
          // 后端声明是否允许「本会话允许」：不可逆操作恒为 false，前端据此隐藏该选项
          can_remember: p.can_remember === true
        }
      }
    case 'chat:approval-decided':
      return {
        type: 'approval_decided',
        data: { id: p.id, decision: p.decision ?? 'unknown' }
      }
    case 'chat:done':
      // 领域 reason 优先；原始 stop_reason 归一化兜底，避免误弹「已停止生成」
      return {
        type: 'stopped',
        data: { reason: normalizeStopReason(String(p.reason ?? p.stop_reason ?? p.status ?? 'completed')) }
      }
    case 'chat:error':
      return { type: 'error', data: { code: p.code, message: p.message ?? 'unknown error' } }
    case 'chat:retry':
      return { type: 'retry', data: { attempt: p.attempt, delay_ms: p.delay_ms } }
    case 'chat:gap':
      // 断线重放窗口已被环形缓冲覆盖：decoder 转 requestSnapshot，
      // 由 store 立即拉权威快照兜底。
      return { type: 'gap', data: { run_id: p.run_id, last_seq: p.last_seq } }
    case 'ping':
      // 心跳：仅用于前端 watchdog 续期（resetWatchdog 在 handle 内），不下传 decoder
      return null
    // P2：模型产出/任务生命周期事件，旁路消费
    case 'chat:todo':
      return { type: 'todo', data: { state: p.state } }
    case 'chat:file-change':
      return { type: 'file_change', data: { change: p.change } }
    case 'chat:warn':
      // 越界告警：process 副作用落到了 .workbaby/ 之外（污染用户原有目录）
      return {
        type: 'warn',
        data: {
          kind: String(p.kind ?? ''),
          message: String(p.message ?? '工作区越界写入'),
          rel_path: p.rel_path ? String(p.rel_path) : '',
          path: p.path ? String(p.path) : ''
        }
      }
    case 'chat:artifact':
      return { type: 'session_artifact', data: { artifact: p.artifact } }
    case 'chat:compressed':
      return {
        type: 'compressed',
        data: {
          removed_messages: p.removed_messages ?? 0,
          filter_key: p.filter_key ? String(p.filter_key) : '',
          recovery_refs: Array.isArray(p.recovery_refs) ? p.recovery_refs.map(String) : [],
          summary: p.summary ? String(p.summary) : ''
        }
      }
    case 'chat:context-trimmed':
      // 上下文按预算裁剪（system 段超限被丢）：回答质量可能受影响，必须让用户看到原因
      return {
        type: 'context_trimmed',
        data: {
          dropped_segments: Array.isArray(p.dropped_segments) ? p.dropped_segments.map(String) : [],
          budget_runes: Number(p.budget_runes ?? 0)
        }
      }
    default:
      return null
  }
}

/**
 * SSE 事件名白名单：只有「前端需要渲染」的事件才登记。
 *
 * <p>后端还会发一些前端无需感知的事件（`chat:stream.start` 已由建流响应覆盖、
 * `chat:tool-start` 由 `chat:tool` 一并落地为 running 状态、`chat:steer` 由本地 toast 回执、
 * harness 的 `agent.*` 属内核内部事件不跨层），这些**有意不登记**——
 * 白名单是「关心的子集」，不等于「后端事件全集」，不要因为对不上就误判为漏收。
 */
const EVENT_NAMES = [
  'chat:stream',
  'chat:thinking',
  'chat:stats',
  'chat:turn-start',
  'chat:checkpoint',
  'chat:skill',
  'chat:tool',
  'chat:tool-result',
  'chat:approval',
  'chat:approval-decided',
  'chat:done',
  'chat:error',
  'chat:gap',
  // M2 可靠性：具名心跳（服务端 sse.go 30s 一次）
  'ping',
  // P2 扩展事件
  'chat:todo',
  'chat:file-change',
  'chat:warn',
  'chat:artifact',
  // 子 Agent 委派生命周期（后端 chat.go 对子 run 事件独立成通道，避免假 chat:done）
  'chat:subagent-start',
  'chat:subagent-done',
  'chat:subagent-error',
  // 建流瞬时错误自动重试提示
  'chat:retry',
  // 自动上下文压缩（达到预算阈值时后端自动触发，需要让用户看见）
  'chat:compressed',
  // 上下文按预算裁剪（system 段超限被丢，回答质量受影响需要可解释）
  'chat:context-trimmed'
] as const

/** 重连退避：500ms 起步指数递增，8s 封顶。 */
const RECONNECT_BASE_MS = 500
const RECONNECT_MAX_MS = 8000
/**
 * 双看门狗阈值：
 *   - IDLE_MS：完全无任何事件（含 ping）的最大容忍时间——TCP 未断但数据不通；
 *   - STALL_MS：仅 ping 而无业务事件的最大容忍时间——服务端心跳还在发，
 *     但 chat:stream / chat:thinking 都卡住，常见于模型侧抽风或 SSE 缓冲满。
 * 两个独立计时器：任一超时都判定假死，主动断开走退避重连。
 */
const IDLE_MS = 75_000
const STALL_MS = 5 * 60_000

/**
 * 整轮 run 的总超时兜底：后端 runLLM 墙钟上限 10 分钟（default Agent 未配 MaxWallTime），
 * 这里留足余量。chat:done 一旦丢失（且重放窗口覆盖不到）前端会永远停在「正在输入」，
 * 总超时是最后一道防线——超时后由调用方拉取权威快照收尾。
 */
const MAX_RUN_MS = 15 * 60_000

export function streamChat(
  req: ChatStreamReq,
  onEvent: (event: ChatStreamEvent) => void,
  opts: StreamChatOpts = {}
): StreamHandle {
  let es: EventSource | null = null
  let finished = false
  let cancelled = false
  let lastEventId = ''
  let reconnectAttempt = 0
  let watchdog: ReturnType<typeof setTimeout> | null = null
  let stallWatchdog: ReturnType<typeof setTimeout> | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  // clearWatchdogs 在闭包间共享（resetWatchdogs / cancel / 总超时收尾都用到）。
  // 必须挂在最外层 let 上，TS 才能正确推断各调用点的可访问性。
  const clearWatchdogs = (): void => {
    if (watchdog !== null) clearTimeout(watchdog)
    watchdog = null
    if (stallWatchdog !== null) clearTimeout(stallWatchdog)
    stallWatchdog = null
  }

  const promise = (async (): Promise<void> => {
    // 计时起点必须在 POST 之前：run 在 POST 返回时就已在后端 goroutine 里开跑
    const startedAt = Date.now()
    // 1) 发起 run：POST /api/v1/chat/stream → {run_id, session_id, ...}；续跑走 /chat/runs/{id}/resume（path 上的 id 是 runID，与 cancel 的 sessionID 显式区分）。
    let runID: string
    try {
      if (req.resume_run_id) {
        const res = await apiPost<{ run_id: string }>(`/api/v1/chat/runs/${req.resume_run_id}/resume`)
        runID = res.run_id
      } else {
        const res = await apiPost<{ run_id: string }>('/api/v1/chat/stream', {
          session_id: req.session_id,
          content: req.message,
          // model_id 是前端解析过的 model 名；后端 SendStream / CreateSession 用作兜底回查 provider
          model: req.model_id ?? undefined,
          temperature: req.temperature ?? undefined,
          thinking_effort: req.thinking_effort ?? undefined
        })
        runID = res.run_id
      }
    } catch (e) {
      finished = true
      onEvent({ type: 'error', data: { message: e instanceof Error ? e.message : String(e) } })
      return
    }
    if (cancelled) return

    // 2) SSE 连接 + 手动重连
    const base = getApiBase()
    if (!base) {
      finished = true
      onEvent({ type: 'error', data: { message: '[4000] server not initialized' } })
      return
    }

    /**
     * 重置两个看门狗：
     *   - 全静默（idle）从「收到任意事件」算起，含 ping；
     *   - 仅 ping（stall）从「收到任意业务事件」算起，ping 续期 idle 但不续期 stall。
     * 这样 ping 既能防止 idle 误触发，又不会掩盖「心跳在但模型不动」的真假死。
     */
    const resetWatchdogs = (eventName: string): void => {
      clearWatchdogs()
      watchdog = setTimeout(() => scheduleReconnect(), IDLE_MS)
      if (eventName !== 'ping') {
        stallWatchdog = setTimeout(() => scheduleReconnect(), STALL_MS)
      }
    }

    const scheduleReconnect = (): void => {
      if (finished || cancelled) return
      es?.close()
      es = null
      clearWatchdogs()
      if (reconnectTimer !== null) return
      const delay = Math.min(RECONNECT_BASE_MS * 2 ** reconnectAttempt, RECONNECT_MAX_MS)
      reconnectAttempt++
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null
        if (!finished && !cancelled) openStream()
      }, delay)
    }

    const openStream = (): void => {
      const url =
        `${base}/events?scope=chat&runId=${encodeURIComponent(runID)}` +
        (lastEventId ? `&last_event_id=${encodeURIComponent(lastEventId)}` : '')
      es = new EventSource(url)

      es.addEventListener('sse-ready', () => {
        // 重连成功：先补快照（重放覆盖不到的窗口由 store 权威拉取兜底），再续命看门狗
        if (reconnectAttempt > 0) {
          opts.onReconnect?.()
          reconnectAttempt = 0
        }
        // sse-ready 不算业务事件——只续 idle，不续 stall（让 stall 能识别「连上了但模型卡」）
        resetWatchdogs('sse-ready')
      })

      const handle = (name: string) => (e: MessageEvent<string>) => {
        if (cancelled) return
        resetWatchdogs(name)
        if (e.lastEventId) lastEventId = e.lastEventId
        let data: unknown
        try {
          data = e.data ? JSON.parse(e.data) : {}
        } catch {
          data = {}
        }
        const ev = mapSSEEvent(name, data)
        if (ev) onEvent(ev)
        // 只有 chat:done 结束流：chat:error 可能是非致命错误（如检查点保存失败），
        // run 会继续并最终补发 chat:done——此时断流会丢掉后续全部增量与最终结果。
        // 终态缺失的风险由 MAX_RUN_MS 总超时兜底。
        if (name === 'chat:done') {
          finished = true
          clearWatchdogs()
          es?.close()
        }
      }

      for (const name of EVENT_NAMES) {
        es.addEventListener(name, handle(name) as EventListener)
      }
      es.onerror = () => {
        if (finished || cancelled) {
          clearWatchdogs()
          es?.close()
          return
        }
        scheduleReconnect()
      }
      resetWatchdogs('sse-ready')
    }

    openStream()

    // 3) 等待终态（chat:done 置 finished；总超时兜底防永久挂起）
    let timedOut = false
    while (!finished && !cancelled) {
      if (Date.now() - startedAt > MAX_RUN_MS) {
        timedOut = true
        finished = true
        break
      }
      await new Promise((r) => setTimeout(r, 100))
    }
    clearWatchdogs()
    if (reconnectTimer !== null) clearTimeout(reconnectTimer)
    // es 经 openStream() 闭包赋值；TS 跨函数不追踪外部 let，静态推断仍为 null → 显式断言恢复类型
    if (es !== null) (es as EventSource).close()
    if (timedOut) {
      onEvent({ type: 'error', data: { message: '响应超时，已按当前进度收尾' } })
    }
  })()

  const cancel = (): void => {
    cancelled = true
    finished = true
    clearWatchdogs()
    if (reconnectTimer !== null) clearTimeout(reconnectTimer)
    es?.close()
  }

  // 让调用方在 onUnmounted 时机自动调取消。
  try { onUnmounted(cancel) } catch { /* 非组件调用上下文忽略 */ }

  return { promise, cancel }
}

// 保留：让调用方在挂载后主动拉取权威消息（store 已用 loadMessages）。
export function fetchMessages(sessionID: string): Promise<unknown> {
  return apiGet(`/api/v1/chat/sessions/${sessionID}/messages?limit=200`)
}
