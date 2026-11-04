// 对话状态：消息流、流式增量、提问卡片、队列、用量。
// 消息流本身在 useMessageFlow 里：流式正文只追加当前轮的 blocks，不整表重建。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '@/api/http'
import { useEngineModels } from '@/composables/useEngineModels'
import { useRunStatus } from '@/composables/useRunStatus'
import { useMessageFlow } from '@/composables/useMessageFlow'
import type {
  AskRequest,
  ChatAttachment,
  ChatState,
  ContextUsage,
  EngineCaps,
  HistoryTurn,
  ModelInfo,
  ModelRow,
  SessionStats,
  SlashCommand,
  UIResponse,
} from '@/types/api'

export const useChatStore = defineStore('chat', () => {
  const sessionFile = ref('')
  const workDir = ref('')
  const engineRunning = ref(false)
  const streaming = ref(false)
  const model = ref<ModelInfo | null>(null)
  const thinkingLevel = ref('')
  const levels = ref<string[]>([])
  // 可选模型清单：引擎的 models.json。切换走 RPC，不是本地开关。
  const models = ref<ModelRow[]>([])
  // 切换进行中的动作名（'model' / 'thinking' / 'new' / 'compact'），
  // 按钮靠它进 loading 并锁尺寸，避免连点造成两次切换打架。
  const switching = ref('')
  const context = ref<ContextUsage | null>(null)
  const autoCompaction = ref(true)
  const stats = ref<SessionStats | null>(null)
  const commands = ref<SlashCommand[]>([])
  // 能力表：界面按它决定显示哪些入口（不渲染灰按钮）。
  const caps = ref<EngineCaps | null>(null)

  // 一轮对话的过程读数（状态文字 / 小组件 / 提示条 / 重试进度）。
  const runStatus = useRunStatus()
  const statusText = runStatus.statusText
  const widgets = runStatus.widgets
  const notices = runStatus.notices
  const retry = runStatus.retry
  const live = runStatus.live
  const lastActivityAt = runStatus.lastActivityAt
  const pushNotice = runStatus.pushNotice
  const dismissNotice = runStatus.dismissNotice
  const setStatus = runStatus.setStatus
  const setWidget = runStatus.setWidget

  const engineModels = useEngineModels()
  const modelsLoading = engineModels.loading

  // 消息流（当前轮 / 历史轮 / 增量补丁）。
  const flow = useMessageFlow()
  const turn = flow.turn
  const lastFinishedRunId = flow.lastFinishedRunId
  const history = flow.history
  // 本轮开始的时刻：消息脚标的秒表从它起走。
  const startedAt = flow.startedAt
  const applyPatch = flow.applyPatch
  const setUsage = flow.setUsage
  const usageOf = flow.usageOf

  const asks = ref<AskRequest[]>([])
  const queue = ref<{ steering: string[]; follow_up: string[] }>({ steering: [], follow_up: [] })
  const compacting = ref(false)
  const promptError = ref('')
  // runError 是「这一轮自己出的错」。它与 promptError 分开：
  // promptError 是输入区上方的横幅，任何动作失败都会往它里写（换模型、压缩、发消息），
  // 而「这一轮出错了」是轮尾的红色状态——拿一个全局错误去染一轮对话的颜色，
  // 会让一次失败的模型切换把一段已经答完的回复标成出错。
  const runError = ref('')
  // 切不到请求会话的原因。它不会自动消失：
  // 用户需要知道「历史可以看，但下一句话会写到另一条会话里」。
  const bindErr = ref('')
  // 引擎通过 set_editor_text 要求回填输入框。它是一次性的，消费后置空。
  const draftOverride = ref('')
  // 界面要求打开某个选择面板（`/model`、`/thinking` 这类内置命令用）。
  const pickerRequest = ref<'' | 'model' | 'thinking'>('')

  const activeAsk = computed(() => asks.value[0] ?? null)
  const busy = computed(() => streaming.value || compacting.value)
  const contextPercent = computed(() => {
    if (!context.value || !context.value.context_window) return 0
    // 引擎的 tokens 可能为 null（压缩后还没估算过），此时按 percent 兜底。
    if (typeof context.value.usage_tokens === 'number') return context.value.usage_tokens / context.value.context_window
    if (typeof context.value.usage_percent === 'number') return context.value.usage_percent / 100
    return 0
  })

  function applyState(state: ChatState) {
    sessionFile.value = state.session_file ?? ''
    workDir.value = state.work_dir ?? ''
    engineRunning.value = state.engine_running
    // 切不到请求的会话时，引擎的当前会话仍是另一条。这时把「实际会写到哪里」
    // 原话交给界面提示——静默把历史当当前会话看，下一句话就会落到错的文件里。
    bindErr.value = state.bind_err ?? ''
    streaming.value = state.streaming
    model.value = state.model ?? null
    thinkingLevel.value = state.thinking_level ?? ''
    levels.value = state.thinking_levels ?? []
    context.value = state.context_usage ?? null
    autoCompaction.value = state.auto_compaction
    stats.value = state.stats ?? null
    commands.value = state.commands ?? []
    if (state.caps) caps.value = state.caps
    if (state.queued) queue.value = { steering: state.queued.steering ?? [], follow_up: state.queued.follow_up ?? [] }
    // 走 applyHistory 而不是直接赋值：服务器的历史里可能已经有正在直播的那一轮，
    // 直接赋值会让同一段回复在历史位与直播位各画一遍。
    flow.applyHistory(state.history ?? [])
  }

  async function refresh(session = sessionFile.value) {
    const state = await api.chatState(session || undefined)
    applyState(state)
    // 模型清单跟着「当前模型」走：引擎可能被换过（装完插件重启、改了 models 配置），
    // 而模型清单不会主动推过来。不在这里跟一次，下拉里就会一直停着上一次的清单。
    if (!models.value.length || !state.model) void loadModels()
    return state
  }

  /** 拉可选模型清单。失败不抛：拉不到时输入区仍应可用。 */
  async function loadModels(force = false): Promise<ModelRow[]> {
    const rows = await engineModels.loadModels(force)
    models.value = rows
    return rows
  }

  // 换模型。乐观更新：先动显示值让点击立刻有反馈，失败拨回去——
  // 否则界面上写着一个引擎并不认识的模型。
  async function setModel(provider: string, id: string) {
    const prev = model.value
    switching.value = 'model'
    model.value = { ...(prev ?? ({} as ModelInfo)), provider_name: provider, model_id: id }
    try {
      await api.setModel(sessionFile.value, provider, id)
    } catch (err) {
      model.value = prev
      throw err
    } finally {
      switching.value = ''
    }
  }

  // 换思考档。同上：先动显示值，失败回滚。
  async function setThinking(level: string) {
    const prev = thinkingLevel.value
    if (level === prev) return
    switching.value = 'thinking'
    thinkingLevel.value = level
    try {
      await api.setThinking(sessionFile.value, level)
    } catch (err) {
      thinkingLevel.value = prev
      throw err
    } finally {
      switching.value = ''
    }
  }

  // 开关自动压缩。同上。
  async function setAutoCompaction(enabled: boolean) {
    const prev = autoCompaction.value
    switching.value = 'compaction'
    autoCompaction.value = enabled
    try {
      await api.setAutoCompaction(sessionFile.value, enabled)
    } catch (err) {
      autoCompaction.value = prev
      throw err
    } finally {
      switching.value = ''
    }
  }

  // 开新会话。必须走 /chat/new：只清本地 state 的话，下一句仍会追加进
  // 同一个会话文件，用户会以为「新开了一个」而实际还接着上面写。
  async function newChat() {
    switching.value = 'new'
    try {
      const res = await api.newChat()
      reset()
      sessionFile.value = res.session_file ?? ''
      await refresh(sessionFile.value)
      return res.session_file
    } finally {
      switching.value = ''
    }
  }

  // 复制当前会话当前位置为一个新会话并切过去（仅 Caps.session_clone 的引擎提供）。
  async function cloneChat() {
    switching.value = 'clone'
    try {
      const res = await api.cloneChat()
      reset()
      sessionFile.value = res.session_file ?? ''
      await refresh(sessionFile.value)
      return res.session_file
    } finally {
      switching.value = ''
    }
  }

  async function clearQueue() {
    await api.clearQueue(sessionFile.value)
    queue.value = { steering: [], follow_up: [] }
  }

  async function send(message: string, attachments: ChatAttachment[] = []) {
    promptError.value = ''
    runError.value = ''
    // 新的一轮开始了：上一个已收尾的标记到这里就作废。
    // 不能挂在 refresh() 上——chat:done 后紧跟的 refresh 会先清掉标记，
    // 之后才到的那帧迟到的快照就会把重复的正文再放进界面。
    lastFinishedRunId.value = ''
    const runID = await api
      .send(sessionFile.value, message, attachments)
      .then((r) => r.run_id)
    flow.beginTurn(runID, message)
    streaming.value = true
    return runID
  }

  async function stop() {
    await api.stop(sessionFile.value)
    streaming.value = false
  }

  async function steer(message: string, attachments: ChatAttachment[] = []) {
    await api.steer(sessionFile.value, message, attachments)
  }

  async function followUp(message: string, attachments: ChatAttachment[] = []) {
    await api.followUp(sessionFile.value, message, attachments)
    queue.value.follow_up = [...queue.value.follow_up, message]
  }

  async function compact() {
    compacting.value = true
    switching.value = 'compact'
    try {
      await api.compact(sessionFile.value)
    } finally {
      compacting.value = false
      switching.value = ''
    }
  }

  async function answer(response: UIResponse) {
    await api.answer(sessionFile.value, response)
    asks.value = asks.value.filter((a) => a.ask_id !== response.ask_id)
  }

  // 界面层删除一轮：后端按会话记账，引擎的账本不动。
  // 本地先把这轮摘掉（下次投影返回时后端也不再带它），失败要抛给调用方。
  async function hideTurn(turnId: string) {
    await api.hideTurn(sessionFile.value, turnId)
    history.value = history.value.filter((t) => t.turn_id !== turnId)
  }

  // 撤销一次隐藏：后端撤账后重拉快照，被摘掉的轮次就回来了。
  async function unhideTurn(turnId: string) {
    await api.unhideTurn(sessionFile.value, turnId)
    await refresh()
  }

  // 收尾：这一轮从流里出来，同时解除「正在回答」。
  // 不在本地并进历史——服务器的历史才是那一轮的真身，再并一遍就是画两遍。
  function finishTurn() {
    streaming.value = false
    retry.value = null
    // 实时读数属于「正在跑的那一轮」：收尾了就清掉。
    // 留着的话活动条会拿上一轮的工具名去解释一轮已经不存在的执行。
    resetLive()
    flow.finishTurn()
  }

  /** 清掉实时读数（新开局 / 收尾 / 换会话）。 */
  function resetLive() {
    live.value = { run_id: '', phase: '', tool_name: '', tool_call_id: '', output_tokens: 0 }
  }

  /**
   * 对齐这一轮的开始时刻。
   *
   * 只用引擎报的值往**前**对齐，不往后：用户点发送那一刻起表，
   * 如果引擎受理得晚了（冷启动），把起点改成那个更晚的时刻等于把等待藏起来。
   */
  function setRunStart(at: number) {
    if (!at) return
    if (!startedAt.value || at < startedAt.value) startedAt.value = at
  }

  function reset() {
    flow.reset()
    asks.value = []
    queue.value = { steering: [], follow_up: [] }
    runStatus.reset()
    context.value = null
    stats.value = null
    promptError.value = ''
    runError.value = ''
  }

  /** 历史里哪些轮可以「删除」（直播轮没有 turn_id，不可删）。 */
  function deletableTurns(): HistoryTurn[] {
    return history.value.filter((t) => Boolean(t.turn_id) && t.turn_role !== 'engine-mark')
  }

  return {
    sessionFile,
    workDir,
    streaming,
    model,
    thinkingLevel,
    levels,
    models,
    modelsLoading,
    switching,
    context,
    contextPercent,
    stats,
    autoCompaction,
    commands,
    caps,
    turn,
    lastFinishedRunId,
    history,
    startedAt,
    asks,
    activeAsk,
    queue,
    statusText,
    widgets,
    notices,
    retry,
    live,
    lastActivityAt,
    compacting,
    busy,
    promptError,
    runError,
    bindErr,
    draftOverride,
    pickerRequest,
    applyPatch,
    setUsage,
    usageOf,
    refresh,
    loadModels,
    setModel,
    setThinking,
    setAutoCompaction,
    newChat,
    cloneChat,
    clearQueue,
    send,
    stop,
    steer,
    followUp,
    compact,
    answer,
    hideTurn,
    unhideTurn,
    pushNotice,
    dismissNotice,
    setStatus,
    setWidget,
    finishTurn,
    resetLive,
    setRunStart,
    reset,
    deletableTurns,
  }
})
