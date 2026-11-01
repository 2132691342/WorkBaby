import { errorText } from '../utils/errors'
// 对话流状态：一次 run 的增量正文、工具卡、引擎提问卡与上下文水位都在这里落地。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import type { AskClosedData, AskData, MessageVO, ModelVO } from '../types/api'
import type {
  CompressedData,
  ContextData,
  DeltaData,
  DoneData,
  ErrorData,
  LivePart,
  NotifyData,
  PendingAsk,
  RetryData,
  StartData,
  StatusData,
  ToolEndData,
  ToolRun,
  ToolStartData,
  WidgetData,
} from '../types/chatEvents'
import { useSettingsStore } from './settings'
import { useToastStore } from './toast'
import { i18n } from '../i18n'
import { stripAnsi } from '../utils/text'
import { useRunClock } from './runClock'

/** 状态条的文案在 store 里也走 i18n：语言一换，提示条不能还是中文 */
function tt(key: string, args?: Record<string, string | number>): string {
  return i18n.global.t(key, args || {})
}

// SSE 事件载荷的类型定义在 types/chatEvents.ts（形状与 store 无关）。


export const useChatStore = defineStore('chat', () => {
  const running = ref(false)
  // 收尾中：内核已 settled，但权威快照还没拉到。这段时间继续显示本轮已写好的内容，
  // 否则用户会看到「答案凭空消失一下又回来」。
  const finalizing = ref(false)
  const parts = ref<LivePart[]>([])
  /** 工具记录按 tool_call_id 索引；parts 只记顺序 */
  const runMap = ref<Record<string, ToolRun>>({})
  const runs = computed(() => Object.values(runMap.value))
  /** 思考 / 正文的全文：都从 parts 拼出来，单一真相 */
  const thinking = computed(() =>
    parts.value.map((p) => (p.kind === 'thinking' ? p.text : '')).join(''),
  )
  const streaming = computed(() =>
    parts.value.map((p) => (p.kind === 'text' ? p.text : '')).join(''),
  )
  const asks = ref<PendingAsk[]>([])
  const lastError = ref('')
  const lastErrorCode = ref('')
  const notice = ref('')
  const contextUsed = ref(0)
  const contextWindow = ref(0)
  const contextRatio = ref(0)
  const contextKnown = ref(false)
  const contextReserve = ref(0)
  const queued = ref<string[]>([])
  const retrying = ref(false)
  // 停止请求已发出、内核还在收尾：这段时间不准把运行态重新种回来，
  // 否则界面会在「已停止」与「运行中」之间反复跳。
  const stopping = ref(false)
  // 引擎的状态条与小部件：key → 内容，多来源共存互不覆盖
  const engineStatus = ref<Record<string, string>>({})
  const engineWidgets = ref<Record<string, string[]>>({})

  function reset() {
    running.value = false
    finalizing.value = false
    stopTicker()
    startedAt.value = 0
    elapsedSec.value = 0
    parts.value = []
    runMap.value = {}
    asks.value = []
    lastError.value = ''
    lastErrorCode.value = ''
    notice.value = ''
    contextUsed.value = 0
    contextWindow.value = 0
    contextRatio.value = 0
    contextKnown.value = false
    contextReserve.value = 0
    queued.value = []
    retrying.value = false
    stopping.value = false
    engineStatus.value = {}
    engineWidgets.value = {}
  }

  // 思考内容可能被用户关掉：关掉后不累积，也不出现在历史渲染里。
  let showThinking = true

  function setShowThinking(on: boolean) {
    showThinking = on
    if (!on) parts.value = parts.value.filter((p) => p.kind !== 'thinking')
  }

  /** 发送；sessionId 为空表示新对话，返回体里带着后端真正建出来的会话编号 */
  async function send(sessionId: string, content: string) {
    lastError.value = ''
    const resp = await api.chat.send({
      session_id: sessionId || undefined,
      content,
    })
    running.value = true
    // chat:start 可能已经先到（也可能被会话切换挤丢）：计时从那一边起算，
    // 两边都不设就永远停在 0s，用户会以为界面卡了。
    if (!startedAt.value) {
      restartClock()
    }
    return resp
  }

  // 停止：后端广播 chat:stopped 才是权威信号，这里只负责把请求发出去。
  async function stop(sessionId: string) {
    if (!sessionId) return
    stopping.value = true
    try {
      await api.chat.stop(sessionId)
    } catch (e) {
      stopping.value = false
      throw e
    }
  }

  async function steer(sessionId: string, content: string) {
    await api.chat.steer(sessionId, content)
  }

  async function followUp(sessionId: string, content: string) {
    await api.chat.followUp(sessionId, content)
    queued.value.push(content)
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
      useToastStore().bad(tt('chat.askAnswerFailed', { msg: errorText(e) }))
    }
  }

  // ---- 事件入口（SSE → store） ----
  // ---- 事件入口（SSE → store） ----
  // 本轮秒表：起跳 / 停表 / 读数都在 runClock 里，这里只用它。
  const { startedAt, elapsedSec, stopTicker, restart: restartClock } = useRunClock()
  function onStart(_data: StartData) {
    running.value = true
    finalizing.value = false
    restartClock()
    retrying.value = false
    parts.value = []
    runMap.value = {}
    lastError.value = ''
    lastErrorCode.value = ''
    notice.value = ''
  }

  // 段序号只在一次 run 内递增：key 用于 v-for 与增量渲染缓存
  let partSeq = 0

  /** 往最后一段追加文字；段型变了就开新段。 */
  function appendText(kind: 'text' | 'thinking', delta: string) {
    const last = parts.value[parts.value.length - 1]
    if (last && last.kind === kind) {
      last.text += delta
      return
    }
    partSeq++
    parts.value.push({ key: `${kind}-${partSeq}`, kind, text: delta })
  }

  function onDelta(data: DeltaData) {
    // 丢了 chat:start（例如第一条消息建会话时 SSE 刚切到新编号）也要回到运行态：
    // 增量已经在路上，这时界面还显示「可以发送」就是自相矛盾。
    if (!running.value) {
      running.value = true
      if (!startedAt.value) {
        restartClock()
      }
    }
    // 后端已剥离 ANSI；这里再兜一层——缓存里的旧响应与假后端都可能带着转义序列。
    const delta = stripAnsi(data.delta || '')
    if (!delta) return
    if (data.kind === 'thinking') {
      if (showThinking) appendText('thinking', delta)
    } else {
      appendText('text', delta)
    }
  }

  function onToolStart(data: ToolStartData) {
    // 断线重连重放可能把同一条 tool_start 再送一次：按 tool_call_id 去重。
    if (runMap.value[data.tool_call_id]) return
    runMap.value[data.tool_call_id] = {
      tool_call_id: data.tool_call_id,
      tool: data.name,
      label: data.label || data.name,
      args: data.args ?? {},
      running: true,
      ok: true,
      title: '',
      output: '',
      duration_ms: 0,
      // 改动文件与差异行由 tool_end 回填：工具参数里猜路径会猜错（同一文件两次改动）
      file_path: '',
      diff: [],
    }
    partSeq++
    parts.value.push({ key: `tool-${partSeq}`, kind: 'tool', toolCallId: data.tool_call_id })
  }

  function onToolEnd(data: ToolEndData) {
    const run = runMap.value[data.tool_call_id]
    if (!run) return
    run.running = false
    run.ok = !data.is_error
    run.output = data.result
    run.duration_ms = data.duration_ms
    run.file_path = data.file_path || ''
    run.diff = Array.isArray(data.diff) ? data.diff : []
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

  function onDone(_data: DoneData) {
    running.value = false
    stopping.value = false
    stopTicker()
    retrying.value = false
    // 不在这时丢 parts：快照到手后由 clearLive() 换掉，中间不留空白。
    finalizing.value = true
    asks.value = []
    queued.value = []
  }

  /** 权威快照已经接上：把流式那一轮的段清掉，后续渲染交给历史投影。 */
  function clearLive() {
    if (running.value) return
    finalizing.value = false
    parts.value = []
    runMap.value = {}
  }

  function onStopped() {
    running.value = false
    finalizing.value = true
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
    if (running.value || stopping.value) return
    running.value = true
    restartClock()
    notice.value = ''
  }

  function onError(data: ErrorData) {
    running.value = false
    stopping.value = false
    stopTicker()
    retrying.value = false
    lastError.value = data.message
    lastErrorCode.value = data.code || ''
    queued.value = []
  }

  function onCompressed(data: CompressedData) {
    notice.value = tt('chat.noticeCompressed', {
      before: data.tokens_before,
      // 压缩后读数事件里没有，留空占位
      after: '',
    })
  }

  function onRetry(data: RetryData) {
    retrying.value = true
    notice.value = tt('chat.noticeRetry', { attempt: data.attempt, max: data.max })
  }

  async function cancelRetry(sessionId: string) {
    if (!sessionId) return
    await api.chat.cancelRetry(sessionId)
    retrying.value = false
    notice.value = tt('chat.noticeCancelRetry')
  }

  function onContext(data: ContextData) {
    contextUsed.value = Math.max(0, data.usage?.context || 0)
    if (data.context_window > 0) contextWindow.value = data.context_window
    contextKnown.value = data.context_window > 0
    // 后端给的是 0-1 占用比；store 统一存百分比，与 seed() 口径一致
    contextRatio.value = Math.max(0, Math.min(100, Math.round((data.ratio || 0) * 100)))
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
      if (m.message_role === 'assistant' && (m.message_usage?.usage_context || 0) > 0)
        return m.message_usage?.usage_context || 0
    }
    return 0
  }

  /** 模型窗口从模型配置读：按 provider/model id 找同 id 的条目。
   *  配置里留空（0）时按兜底值算：0 会让水位环永远显示 0%。 */
  function windowOf(provider: string, model: string): { window: number; max: number } {
    const list = useSettingsStore().providers ?? []
    const pick = (m: ModelVO) => ({
      window: m.model_context_window > 0 ? m.model_context_window : 300000,
      max: m.model_max_tokens > 0 ? m.model_max_tokens : 64000,
    })
    const target = list.find((p) => p.provider_id === provider) ?? list[0]
    if (!target) return { window: 300000, max: 64000 }
    const hit = target.provider_models.find((m) => m.model_id === model)
    if (hit) return pick(hit)
    for (const p of list) {
      const m = p.provider_models.find((x) => x.model_id === model)
      if (m) return pick(m)
    }
    return { window: 300000, max: 64000 }
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
    stopping.value = false
    finalizing.value = false
    parts.value = []
    runMap.value = {}
  }

  // ---- 引擎的 fire-and-forget 反馈 ----

  function onNotify(data: NotifyData) {
    const t = useToastStore()
    const msg = stripAnsi(data.text || '')
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

  return {
    running,
    finalizing,
    parts,
    thinking,
    streaming,
    runs,
    /** 按 tool_call_id 取本轮工具记录：模板里逐段渲染时要用 */
    runOf: (id: string) => runMap.value[id],
    asks,
    lastError,
    lastErrorCode,
    notice,
    contextUsed,
    contextWindow,
    contextRatio,
    contextKnown,
    contextReserve,
    queued,
    retrying,
    stopping,
    engineStatus,
    engineWidgets,
    elapsedSec,
    reset,
    send,
    stop,
    steer,
    followUp,
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
    clearLive,
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
    seedContext,
  }
})
