// 对话流状态：一次 run 的增量正文、工具卡、引擎提问卡与截断态都在这里落地。
import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type {
  AskClosedData,
  AskData,
  AttachmentREQ,
  CompressedData,
  ContextData,
  DeltaData,
  DoneData,
  ErrorData,
  MessageVO,
  NotifyData,
  RetryData,
  StartData,
  StatusData,
  ToolEndData,
  ToolStartData,
  WidgetData,
} from '../types/api'
import { useSettingsStore } from './settings'
import { useToastStore } from './toast'
import { i18n } from '../i18n'
import { stripAnsi } from '../utils/text'

/** 状态条的文案在 store 里也走 i18n：语言一换，提示条不能还是中文 */
function tt(key: string, args?: Record<string, string | number>): string {
  return i18n.global.t(key, args || {})
}

// 当前正在跑的工具调用：流式期间只做「展示」，chat:done 后用权威快照覆盖。
export interface ToolRun {
  tool_call_id: string
  tool: string
  label: string
  args: Record<string, unknown>
  running: boolean
  ok: boolean
  title: string
  output: string
  duration_ms: number
}

// 引擎抛回来的一条提问：来源是 PI 里的扩展（含内置的提问工具）。
export interface PendingAsk {
  id: string
  method: string
  title: string
  message: string
  options: string[]
  /** 输入类提问的预置提示（多选题的「1,3; 自定义答案」就靠它）*/
  placeholder: string
  /** editor 的预填内容 */
  prefill: string
  /** 等待上限（毫秒）：0 表示不限时 */
  timeoutMs: number
}

export const useChatStore = defineStore('chat', () => {
  const running = ref(false)
  const thinking = ref('')
  const streaming = ref('')
  const runs = ref<ToolRun[]>([])
  const asks = ref<PendingAsk[]>([])
  const lastError = ref('')
  const lastErrorCode = ref('')
  const notice = ref('')
  const truncated = ref(false)
  const truncBudget = ref(0)
  const contextUsed = ref(0)
  const contextWindow = ref(0)
  const contextRatio = ref(0)
  const contextKnown = ref(false)
  const contextReserve = ref(0)
  const queued = ref<string[]>([])
  const retrying = ref(false)
  // 引擎扩展的状态条与小部件：key → 内容，多扩展共存互不覆盖
  const engineStatus = ref<Record<string, string>>({})
  const engineWidgets = ref<Record<string, string[]>>({})
  /** 引擎给窗口起的标题（扩展的 setTitle）：标题栏用它，不写会话名 */
  const suggestedTitle = ref('')

  function reset() {
    running.value = false
    stopTicker()
    startedAt.value = 0
    elapsedSec.value = 0
    thinking.value = ''
    streaming.value = ''
    runs.value = []
    asks.value = []
    lastError.value = ''
    lastErrorCode.value = ''
    notice.value = ''
    truncated.value = false
    truncBudget.value = 0
    contextUsed.value = 0
    contextWindow.value = 0
    contextRatio.value = 0
    contextKnown.value = false
    contextReserve.value = 0
    queued.value = []
    retrying.value = false
    engineStatus.value = {}
    engineWidgets.value = {}
    suggestedTitle.value = ''
  }

  // 思考内容可能被用户关掉：关掉后不累积，也不出现在历史渲染里。
  let showThinking = true

  function setShowThinking(on: boolean) {
    showThinking = on
    if (!on) thinking.value = ''
  }

  /** 发送；sessionId 为空表示新对话，返回体里带着后端真正建出来的会话编号 */
  async function send(sessionId: string, content: string, attachments?: AttachmentREQ[]) {
    lastError.value = ''
    const resp = await api.chat.send({
      session_id: sessionId || undefined,
      workspace: useSettingsStore().values.workspace || undefined,
      content,
      attachments,
    })
    running.value = true
    return resp
  }

  // 停止：后端广播 chat:stopped 才是权威信号，这里只负责把请求发出去。
  async function stop(sessionId: string) {
    if (!sessionId) return
    await api.chat.stop(sessionId)
  }

  async function steer(sessionId: string, content: string) {
    await api.chat.steer(sessionId, content)
  }

  async function followUp(sessionId: string, content: string) {
    await api.chat.followUp(sessionId, content)
    queued.value.push(content)
  }

  function onDequeued(content: string) {
    const i = queued.value.indexOf(content)
    if (i >= 0) queued.value.splice(i, 1)
    else if (queued.value.length) queued.value.shift()
  }

  function clearQueued() {
    queued.value = []
  }

  /** 回答引擎的提问：confirm 走 confirmed，其余走 value / cancelled */
  async function answer(ask: PendingAsk, value: string, confirmed: boolean, cancelled = false) {
    try {
      await api.chat.answer({
        id: ask.id,
        value,
        confirmed,
        cancelled,
      })
      asks.value = asks.value.filter((a) => a.id !== ask.id)
    } catch (e) {
      // "5010" = 这条提问已经不需要回答了（等待超时 / 会话中止）：摘掉卡片并说清原因。
      const code = (e as { code?: string })?.code
      if (code === '5010') {
        asks.value = asks.value.filter((a) => a.id !== ask.id)
        useToastStore().info(tt('chat.askClosed'))
        return
      }
      useToastStore().bad(tt('chat.askAnswerFailed', { msg: (e as Error)?.message || '' }))
    }
  }

  // ---- 事件入口（SSE → store） ----
  // run 开始时刻与本轮 token 读数：耗时读数由界面计时器消费
  const startedAt = ref(0)
  const elapsedSec = ref(0)
  let tickTimer = 0
  function startTicker() {
    stopTicker()
    tickTimer = window.setInterval(() => {
      if (startedAt.value) elapsedSec.value = Math.round((Date.now() - startedAt.value) / 1000)
    }, 1000)
  }
  function stopTicker() {
    if (tickTimer) clearInterval(tickTimer)
    tickTimer = 0
  }

  function onStart(_data: StartData) {
    running.value = true
    startedAt.value = Date.now()
    elapsedSec.value = 0
    startTicker()
    retrying.value = false
    thinking.value = ''
    streaming.value = ''
    runs.value = []
    lastError.value = ''
    lastErrorCode.value = ''
    truncated.value = false
    truncBudget.value = 0
    notice.value = ''
  }

  function onDelta(data: DeltaData) {
    running.value = true
    // 后端已剥离 ANSI；这里再兜一层——缓存里的旧响应与假后端都可能带着转义序列。
    const delta = stripAnsi(data.delta || '')
    if (data.kind === 'thinking') {
      if (showThinking) thinking.value += delta
    } else streaming.value += delta
  }

  function onToolStart(data: ToolStartData) {
    // 断线重连重放可能把同一条 tool_start 再送一次：按 tool_call_id 去重。
    if (runs.value.some((r) => r.tool_call_id === data.tool_call_id)) return
    runs.value.push({
      tool_call_id: data.tool_call_id,
      tool: data.tool,
      label: data.label || data.tool,
      args: data.args,
      running: true,
      ok: true,
      title: '',
      output: '',
      duration_ms: 0,
    })
  }

  function onToolEnd(data: ToolEndData) {
    const run = runs.value.find((r) => r.tool_call_id === data.tool_call_id)
    if (!run) return
    run.running = false
    run.ok = data.ok
    run.title = data.title
    run.output = data.output
    run.duration_ms = data.duration_ms
  }

  function onAsk(data: AskData) {
    if (asks.value.some((a) => a.id === data.id)) return
    asks.value.push({
      id: data.id,
      method: data.method,
      title: data.title,
      message: data.message,
      options: data.options ?? [],
      placeholder: data.placeholder ?? '',
      prefill: data.prefill ?? '',
      timeoutMs: data.timeout_ms ?? 0,
    })
  }

  function onAskClosed(data: AskClosedData) {
    asks.value = asks.value.filter((a) => a.id !== data.id)
  }

  function onDone(data: DoneData) {
    running.value = false
    stopTicker()
    retrying.value = false
    switch (data.stop_reason) {
      case 'length':
        truncated.value = true
        truncBudget.value = data.max_tokens || 0
        break
      case 'max_turns':
        notice.value = tt('chat.noticeMaxTurns')
        break
    }
    if (data.usage) {
      seed(data.usage.context, contextWindow.value, contextKnown.value)
    }
    runs.value = []
    thinking.value = ''
    streaming.value = ''
    asks.value = []
    queued.value = []
  }

  function onStopped() {
    running.value = false
    stopTicker()
    retrying.value = false
    notice.value = tt('chat.noticeStopped')
    queued.value = []
    asks.value = []
  }

  /** 切回一条还在跑的会话：SSE 不会补发 chat:start，把运行态从快照种回来。
   *  计时从接入点起算（真实起点只有内核知道），文本流从现在开始累积，
   *  chat:done 之后的权威快照会把整段补齐。 */
  function resumeRunning() {
    if (running.value) return
    running.value = true
    startedAt.value = Date.now()
    elapsedSec.value = 0
    startTicker()
    notice.value = ''
  }

  function onError(data: ErrorData) {
    running.value = false
    stopTicker()
    retrying.value = false
    lastError.value = data.message
    lastErrorCode.value = data.code || ''
    queued.value = []
  }

  function onCompressed(data: CompressedData) {
    notice.value = tt('chat.noticeCompressed', {
      before: data.tokens_before,
      after: data.tokens_after,
    })
  }

  function onRetry(data: RetryData) {
    retrying.value = true
    notice.value = data.message
      ? tt('chat.noticeRetryReason', { attempt: data.attempt, max: data.max, msg: data.message })
      : tt('chat.noticeRetry', { attempt: data.attempt, max: data.max })
  }

  async function cancelRetry(sessionId: string) {
    if (!sessionId) return
    await api.chat.cancelRetry(sessionId)
    retrying.value = false
    notice.value = tt('chat.noticeCancelRetry')
  }

  function onContext(data: ContextData) {
    contextUsed.value = data.used
    contextWindow.value = data.window
    contextKnown.value = data.known !== false
    contextReserve.value = data.reserve || 0
    contextRatio.value = Math.max(0, Math.min(100, Math.round(data.ratio || 0)))
  }

  // ---- 快照回填：进入 / 切换会话立刻有水位读数 ----

  function seed(used: number, win: number, known: boolean, reserve = 0) {
    contextUsed.value = Math.max(0, used || 0)
    if (win > 0) contextWindow.value = win
    if (reserve > 0) contextReserve.value = reserve
    contextKnown.value = win > 0 ? known : false
    const usable = win - reserve > 0 ? win - reserve : Math.floor(win / 2)
    contextRatio.value =
      usable > 0
        ? Math.max(0, Math.min(100, Math.round((contextUsed.value / usable) * 100)))
        : 0
  }

  function lastContextOf(messages: MessageVO[]): number {
    for (let i = messages.length - 1; i >= 0; i--) {
      const m = messages[i]
      if (m.role === 'assistant' && (m.usage?.context || 0) > 0) return m.usage?.context || 0
    }
    return 0
  }

  /** 模型窗口从 models.json 读：SessionVO.model 找到同 id 的条目 */
  function windowOf(provider: string, model: string): { window: number; max: number } {
    const list = useSettingsStore().providers?.providers ?? []
    const target = list.find((p) => p.id === provider) ?? list[0]
    if (!target) return { window: 0, max: 0 }
    const hit = target.models.find((m) => m.id === model)
    if (hit) return { window: hit.context_window, max: hit.max_tokens }
    for (const p of list) {
      const m = p.models.find((x) => x.id === model)
      if (m) return { window: m.context_window, max: m.max_tokens }
    }
    return { window: 0, max: 0 }
  }

  async function seedContext(provider: string, model: string, messages: MessageVO[]) {
    if (running.value) return
    const { window, max } = windowOf(provider, model)
    seed(lastContextOf(messages), window, window > 0, max)
  }

  function notify(text: string) {
    notice.value = text
  }

  function onGap() {
    notice.value = tt('chat.noticeGap')
    running.value = false
    runs.value = []
    streaming.value = ''
    thinking.value = ''
  }

  // ---- 引擎扩展的 fire-and-forget 反馈 ----

  function onNotify(data: NotifyData) {
    const t = useToastStore()
    const msg = stripAnsi(data.message || '')
    if (data.level === 'error') t.bad(msg)
    else if (data.level === 'warning') t.info(`⚠ ${msg}`)
    else t.info(msg)
  }

  function onStatus(data: StatusData) {
    const next = { ...engineStatus.value }
    const text = stripAnsi(data.text || '')
    if (text) next[data.key] = text
    else delete next[data.key]
    engineStatus.value = next
  }

  function onWidget(data: WidgetData) {
    const next = { ...engineWidgets.value }
    const lines = (data.lines || []).map(stripAnsi)
    if (lines.length) next[data.key] = lines
    else delete next[data.key]
    engineWidgets.value = next
  }

  function onTitle(data: { title: string }) {
    suggestedTitle.value = (data.title || '').trim()
  }

  return {
    running,
    thinking,
    streaming,
    runs,
    asks,
    lastError,
    lastErrorCode,
    notice,
    truncated,
    truncBudget,
    contextUsed,
    contextWindow,
    contextRatio,
    contextKnown,
    contextReserve,
    queued,
    retrying,
    engineStatus,
    engineWidgets,
    suggestedTitle,
    elapsedSec,
    reset,
    send,
    stop,
    steer,
    followUp,
    onDequeued,
    clearQueued,
    answer,
    notify,
    setShowThinking,
    onStart,
    onDelta,
    onToolStart,
    onToolEnd,
    onAsk,
    onAskClosed,
    onDone,
    resumeRunning,
    onStopped,
    onError,
    onCompressed,
    onRetry,
    cancelRetry,
    onContext,
    onGap,
    onNotify,
    onStatus,
    onWidget,
    onTitle,
    seedContext,
  }
})
