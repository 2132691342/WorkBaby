// 对话状态：消息流、流式增量、提问卡片、队列、用量。
// 消息流本身在 useMessageFlow 里：流式正文只追加当前轮的 blocks，不整表重建。
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '@/api/http'
import { useEngineModels } from '@/composables/useEngineModels'
import { useRunStatus } from '@/composables/useRunStatus'
import { useMessageFlow } from '@/composables/useMessageFlow'
import { setActiveKind } from './app'
import type {
  AskRequest, ChatAttachment, ChatState, ContextUsage, EngineCaps, EngineKind,
  HistoryTurn, ModelInfo, ModelRow, SessionStats, SlashCommand, UIResponse,
} from '@/types/api'

export const useChatStore = defineStore('chat', () => {
  const sessionFile = ref('')
  const workDir = ref('')
  const engineRunning = ref(false)
  const streaming = ref(false)
  const model = ref<ModelInfo | null>(null)
  const thinkingLevel = ref('')
  const levels = ref<string[]>([])
  // 可选模型清单：引擎的 get_available_models。切换走 RPC，不是本地开关。
  const models = ref<ModelRow[]>([])
  // 切换进行中的动作名（'model' / 'thinking' / 'new' / 'compact'），
  // 按钮靠它进 loading 并锁尺寸，避免连点造成两次切换打架。
  const switching = ref('')
  const context = ref<ContextUsage | null>(null)
  const tokensPerSecond = ref<number | null>(null)
  const autoCompaction = ref(true)
  const stats = ref<SessionStats | null>(null)
  const commands = ref<SlashCommand[]>([])
  const profile = ref('')
  // 这一轮/这个会话属于哪个引擎。切换引擎时它跟着换，输入区据此显示。
  const kind = ref<EngineKind>('omp')
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

  // 每个引擎一份的模型清单与思考档（分桶缓存，理由见 composable）。
  const engineModels = useEngineModels(() => kind.value)
  const { modelsLoading } = engineModels

  // 消息流（当前轮 / 历史轮 / 增量补丁），理由见 composable。
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
  const queue = ref<{ steering: string[]; followUp: string[] }>({ steering: [], followUp: [] })
  const compacting = ref(false)
  const promptError = ref('')
  // runError 是「这一轮自己出的错」。它与 promptError 分开：
  // promptError 是输入区上方的横幅，任何动作失败都会往它里写（换模型、压缩、发消息），
  // 而「这一轮出错了」是轮尾的红色状态——拿一个全局错误去染一轮对话的颜色，
  // 会让一次失败的模型切换把一段已经答完的回复标成出错。
  const runError = ref('')
  // 切不到请求会话的原因（引擎正忙 / 扩展拦下）。它不会自动消失：
  // 用户需要知道“历史可以看，但下一句话会写到另一条会话里”。
  const bindErr = ref('')
  // 引擎通过 set_editor_text 要求回填输入框。它是一次性的，消费后置空。
  const draftOverride = ref('')

  // 界面要求打开某个选择面板（`/model`、`/thinking` 这类内置命令用）。
  // 走 store 不走事件：命令在 composable 里执行，面板在底栏子组件里，
  // 中间隔了多层，共享一个意图位最省事——面板自己 watch 它。
  const pickerRequest = ref<'' | 'model' | 'thinking'>('')

  const activeAsk = computed(() => asks.value[0] ?? null)
  const busy = computed(() => streaming.value || compacting.value)
  const contextPercent = computed(() => {
    if (!context.value || !context.value.contextWindow) return 0
    // pi 的 tokens 可能为 null（还没估算过），此时按 percent 兜底。
    if (typeof context.value.tokens === 'number') return context.value.tokens / context.value.contextWindow
    if (typeof context.value.percent === 'number') return context.value.percent / 100
    return 0
  })

  function applyState(state: ChatState) {
    sessionFile.value = state.session_file ?? ''
    workDir.value = state.work_dir ?? ''
    engineRunning.value = state.engine_running
    // 切不到请求的会话时，引擎的当前会话仍是另一条。这时把“实际会写到哪里”
    // 原话交给界面提示——静默把历史当当前会话看，下一句话就会落到错的文件里。
    bindErr.value = state.bind_err ?? ''
    streaming.value = state.streaming
    model.value = state.model ?? null
    thinkingLevel.value = state.thinking_level ?? ''
    // 引擎以状态为准：会话文件自己带着它属于哪个引擎，比用户上一次的选择可靠。
    // 不同步的话，从会话列表点开一条另一个引擎的会话之后，输入框上还写着旧引擎，
    // 而消息实际走的是新引擎。
    if (state.kind && state.kind !== kind.value) {
      kind.value = state.kind
      setActiveKind(state.kind)
      models.value = engineModels.modelCache.value[state.kind] ?? []
      levels.value = engineModels.levelsOf(state.kind)
      caps.value = state.caps ?? caps.value
    }
    // 思考档按引擎记账：切回上一个引擎时，它的档位立刻就位，
    // 不用等一次 RPC 回来。
    engineModels.setLevels(kind.value, state.levels ?? [])
    levels.value = engineModels.levelsOf(kind.value)
    context.value = state.context ?? null
    tokensPerSecond.value = state.tokens_per_second ?? null
    autoCompaction.value = state.auto_compaction
    stats.value = state.stats ?? null
    commands.value = state.commands ?? []
    profile.value = state.profile ?? ''
    if (state.caps) caps.value = state.caps
    if (state.queued) queue.value = { steering: state.queued.steering ?? [], followUp: state.queued.followUp ?? [] }
    // 走 applyHistory 而不是直接赋值：服务器的历史里可能已经有正在直播的那一轮，
    // 直接赋值会让同一段回复在历史位与直播位各画一遍。
    flow.applyHistory(state.history ?? [])
  }

  async function refresh(session = sessionFile.value) {
    const state = await api.chatState(kind.value, session || undefined)
    applyState(state)
    // 模型清单跟着「当前引擎 + 当前模型」走：引擎可能被换过（装完插件重启、
    // 改了 models 配置），而 get_available_models 的返回不会主动推过来。
    // 不在这里跟一次，下拉里就会一直停着上一次的清单。
    if (!models.value.length || !state.model) void loadModels()
    return state
  }

  /**
   * 拉可选模型清单（走 composable 的分桶缓存）。
   *
   * 失败不抛：模型清单拉不到时输入区仍应可用，只是下拉里只剩「当前模型」一项。
   * 请求返回时用户可能已经切走了：这时不能覆盖当前引擎的清单。
   */
  async function loadModels(force = false): Promise<ModelRow[]> {
    const target = kind.value
    const rows = await engineModels.loadModels(force)
    if (kind.value === target) models.value = rows
    return rows
  }

  // 换模型。乐观更新：先动显示值让点击立刻有反馈，失败拨回去——
  // 否则界面上写着一个引擎并不认识的模型。
  // 引擎取「点下去那一刻」的值：半秒里可能切了引擎，回滚不能改另一个引擎的显示值。
  async function setModel(provider: string, id: string) {
    const engine = kind.value
    const prev = model.value
    switching.value = 'model'
    model.value = { ...(prev ?? ({} as ModelInfo)), provider, id }
    try {
      await api.setModel(engine, sessionFile.value, provider, id)
    } catch (err) {
      if (kind.value === engine) model.value = prev
      throw err
    } finally {
      switching.value = ''
    }
  }

  // 换思考档。同上：先动显示值，失败回滚。
  async function setThinking(level: string) {
    const engine = kind.value
    const prev = thinkingLevel.value
    if (level === prev) return
    switching.value = 'thinking'
    thinkingLevel.value = level
    try {
      await api.setThinking(engine, sessionFile.value, level)
    } catch (err) {
      if (kind.value === engine) thinkingLevel.value = prev
      throw err
    } finally {
      switching.value = ''
    }
  }

  // 开关自动压缩。同上。
  async function setAutoCompaction(enabled: boolean) {
    const engine = kind.value
    const prev = autoCompaction.value
    switching.value = 'compaction'
    autoCompaction.value = enabled
    try {
      await api.setAutoCompaction(engine, sessionFile.value, enabled)
    } catch (err) {
      if (kind.value === engine) autoCompaction.value = prev
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
      const res = await api.newChat(kind.value)
      reset()
      sessionFile.value = res.session_file ?? ''
      await refresh(sessionFile.value)
      return res.session_file
    } finally {
      switching.value = ''
    }
  }

  // 复制当前会话当前位置为一个新会话并切过去（仅 Caps.SessionClone 的引擎提供）。
  async function cloneChat() {
    switching.value = 'clone'
    try {
      const res = await api.cloneChat(kind.value)
      reset()
      sessionFile.value = res.session_file ?? ''
      await refresh(sessionFile.value)
      return res.session_file
    } finally {
      switching.value = ''
    }
  }

  // 切引擎：不切目录。保留已看到的消息（可回看），在新引擎上开新会话——
  // 直接清空会让人以为上下文丢了。
  // 顺序：先摆上目标引擎的缓存值（模型 / 思考档）再去问后端，切换对用户是瞬时的。
  async function switchEngine(next: EngineKind) {
    if (next === kind.value) return
    // 先把已看的消息并进历史，否则它们随 turn 一起被丢掉。
    if (turn.value) finishTurn()
    // kept 是用户在旧引擎上已经读到的那些轮次。它必须一直留在屏幕上：
    // 新引擎的会话是空的，把 kept 换成 state.history 等于把整段对话抹掉。
    const kept = history.value
    const mark: HistoryTurn = { role: 'engine-mark', blocks: [{ kind: 'text', text: next }] }
    kind.value = next
    // 立刻换到目标引擎的缓存：下拉里该出现哪个引擎的模型，一眼就该是它。
    models.value = engineModels.modelCache.value[next] ?? []
    levels.value = engineModels.levelsOf(next)
    // 上下文用量与流式状态属于旧引擎：它们不属于新引擎，留着就是隔别处的读数。
    context.value = null
    tokensPerSecond.value = null
    stats.value = null
    bindErr.value = ''
    promptError.value = ''
    runError.value = ''
    queue.value = { steering: [], followUp: [] }
    // 流里插一条分隔：告诉用户「以下由 xxx 接续，前面的内容不在新会话里」。
    history.value = [...kept, mark]

    // 不等后端往返：await chatState 会在引擎没跑时顺手拉起进程
    // （pi 起一次 Node 实测 3 秒），「切引擎」这个高频动作就得每次盯三秒转圈。
    // 分桶缓存此刻已是目标引擎的样子，真正的往返放后台补。
    void (async () => {
      try {
        const state = await api.chatState(next, undefined)
        // 请求回来时用户可能已经切走了：那就别覆盖当前引擎的状态。
        if (kind.value !== next) return
        applyState(state)
        // applyState 用新引擎的历史覆盖了整列：把 kept 与分隔补回去。
        // 补在**前面**，因为新引擎的历史属于「那之后的对话」。
        const rest = (state.history ?? []).filter(
          (h) => !(h.role === 'engine-mark' && h.blocks[0]?.text === next),
        )
        history.value = kept.length || rest.length ? [...kept, mark, ...rest] : []
      } catch {
        /* 切引擎失败不弹提示：界面已经切过去了，用户能正常用，
           只是这一轮没拿到引擎的会话状态。 */
      }
    })()

    // 后台补这个引擎的模型清单：切过去时可能是第一次用这个引擎。
    void loadModels()
  }

  async function clearQueue() {
    await api.clearQueue(kind.value, sessionFile.value)
    queue.value = { steering: [], followUp: [] }
  }

  // 移除单条排队消息（仅 omp）：清空是 pi 的能力，逐条移除是 omp 的能力。
  async function removeQueued(message: string, queueName: 'steering' | 'followUp') {
    const res = await api.removeQueued(kind.value, sessionFile.value, message, queueName)
    if (res.removed) {
      if (queueName === 'steering') queue.value.steering = queue.value.steering.filter((m) => m !== message)
      else queue.value.followUp = queue.value.followUp.filter((m) => m !== message)
    }
    return res.removed
  }

  async function send(message: string, attachments: ChatAttachment[] = []) {
    promptError.value = ''
    runError.value = ''
    // 新的一轮开始了：上一个已收尾的标记到这里就作废。
    // 不能挂在 refresh() 上——chat:done 后紧跟的 refresh 会先清掉标记，
    // 之后才到的那帧迟到的快照就会把重复的正文再放进界面。
    lastFinishedRunId.value = ''
    const runID = await api
      .send(kind.value, sessionFile.value, message, attachments)
      .then((r) => r.run_id)
    flow.beginTurn(runID, message)
    streaming.value = true
    return runID
  }

  async function stop() {
    await api.stop(kind.value, sessionFile.value)
    streaming.value = false
  }

  async function steer(message: string, attachments: ChatAttachment[] = []) {
    await api.steer(kind.value, sessionFile.value, message, attachments)
  }

  async function followUp(message: string, attachments: ChatAttachment[] = []) {
    await api.followUp(kind.value, sessionFile.value, message, attachments)
    queue.value.followUp = [...queue.value.followUp, message]
  }

  async function compact() {
    compacting.value = true
    switching.value = 'compact'
    try {
      await api.compact(kind.value, sessionFile.value)
    } finally {
      compacting.value = false
      switching.value = ''
    }
  }

  async function answer(response: UIResponse) {
    await api.answer(kind.value, sessionFile.value, { ...response, type: 'extension_ui_response' })
    asks.value = asks.value.filter((a) => a.id !== response.id)
  }

  // 界面层删除一轮：后端按会话记账，引擎的账本不动。
  // 本地先把这轮摘掉（下次投影返回时后端也不再带它），失败要抛给调用方。
  async function hideTurn(hid: string) {
    await api.hideTurn(sessionFile.value, hid)
    history.value = history.value.filter((t) => t.hid !== hid)
  }

  // 撤销一次隐藏：后端撤账后重拉快照，被摘掉的轮次就回来了。
  async function unhideTurn(hid: string) {
    await api.unhideTurn(sessionFile.value, hid)
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
    queue.value = { steering: [], followUp: [] }
    runStatus.reset()
    context.value = null
    stats.value = null
    promptError.value = ''
    runError.value = ''
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
    tokensPerSecond,
    autoCompaction,
    commands,
    kind,
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
    switchEngine,
    clearQueue,
    removeQueued,
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
  }
})
