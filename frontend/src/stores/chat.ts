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
        useToastStore().info('这个问题已经不需要回答了')
        return
      }
      useToastStore().bad(`回答失败：${(e as Error)?.message || '请重试'}`)
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
    if (data.kind === 'thinking') {
      if (showThinking) thinking.value += data.delta
    } else streaming.value += data.delta
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
        notice.value = '这一轮步骤太多，已经停下。可以点「继续」接着做'
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
    notice.value = '已停止'
    queued.value = []
    asks.value = []
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
    notice.value = `对话太长了，已整理较早的内容（${data.tokens_before} → ${data.tokens_after}）`
  }

  function onRetry(data: RetryData) {
    retrying.value = true
    notice.value = data.message
      ? `上游拒绝，正在重试（${data.attempt}/${data.max}）：${data.message}`
      : `正在重试（${data.attempt}/${data.max}）`
  }

  async function cancelRetry(sessionId: string) {
    if (!sessionId) return
    await api.chat.cancelRetry(sessionId)
    retrying.value = false
    notice.value = '已停止重试'
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
    notice.value = '连接出现过中断，正在对账刷新'
    running.value = false
    runs.value = []
    streaming.value = ''
    thinking.value = ''
  }

  // ---- 引擎扩展的 fire-and-forget 反馈 ----

  function onNotify(data: NotifyData) {
    const t = useToastStore()
    if (data.level === 'error') t.bad(data.message)
    else if (data.level === 'warning') t.info(`⚠ ${data.message}`)
    else t.info(data.message)
  }

  function onStatus(data: StatusData) {
    const next = { ...engineStatus.value }
    if (data.text) next[data.key] = data.text
    else delete next[data.key]
    engineStatus.value = next
  }

  function onWidget(data: WidgetData) {
    const next = { ...engineWidgets.value }
    if (data.lines?.length) next[data.key] = data.lines
    else delete next[data.key]
    engineWidgets.value = next
  }

  function onTitle(data: { title: string }) {
    if (!data.title) return
    notice.value = `助手建议标题：${data.title}`
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
