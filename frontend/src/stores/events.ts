// SSE 事件路由：把一帧事件分派到对应的 store。
// 「后端发了什么 → 界面怎么反应」的完整对照，加一个新事件只改这张表。
// 依赖由调用方以接口传入，本文件不 import 任何 store，避免循环依赖。
import {
  asNumber,
  asString,
  readAsk,
  readBlocks,
  readPatch,
  readStrings,
  readTodos,
} from '@/utils/sse'
import type { TurnUsagePayload } from '@/utils/sse'
import i18n from '@/i18n'
import { useChatStore } from './chat'

/** 对话 store 的实例类型。 */
type ChatStore = ReturnType<typeof useChatStore>

/** 路由需要的应用侧能力。 */
export interface RouteDeps {
  /** 对话状态 */
  chat: ChatStore
  /** 重拉运行时与 pi 的状态 */
  refreshEngines: () => Promise<unknown>
  /** 重拉会话列表 */
  refreshSessions: () => Promise<unknown>
  /** 技能清单有后台变化 */
  bumpSkills: () => void
  /** MCP 目录有后台变化 */
  bumpMcp: () => void
  /** 引擎改了工作区文件（一轮结束），工作区页该重看一眼 */
  bumpWorkspace: () => void
  /** 工作区被换了（事实通知）：app 侧按当前工作区判断走全量刷新还是回声对账 */
  onWorkspaceChanged: (workspace: string) => Promise<void>
  /** 用户按了窗口关闭按钮 */
  setClosePrompt: (open: boolean) => void
  /** 弹一条全局提示 */
  toast: (level: 'info' | 'ok' | 'warn' | 'error', message: string) => void
}

// 哪些事件属于「某一次对话」：chat:* 是会话内的流；engine:* / app:* / session:*
// 是全局事实，不能按运行过滤，否则会出现「装完了界面还在转圈」。
export function isChatScoped(event: string): boolean {
  return event.startsWith('chat:')
}

/** 生成速度的取样：按 run 记上次读数，两帧差出 tokens/s；间隔太近沿用上次结果防抖动。 */
let speedSample: { runId: string; tokens: number; at: number; speed: number } | null = null
function sampleSpeed(runId: string, tokens: number): number {
  const prev = speedSample && speedSample.runId === runId ? speedSample : null
  if (!prev || tokens <= prev.tokens) {
    speedSample = { runId, tokens, at: Date.now(), speed: 0 }
    return 0
  }
  const dt = (Date.now() - prev.at) / 1000
  if (dt < 0.5) return prev.speed
  const speed = Math.round((tokens - prev.tokens) / dt)
  speedSample = { runId, tokens, at: Date.now(), speed }
  return speed
}

/** 未知事件只告警一次：后端加了新事件而这张表没接时，要能在控制台看出来。 */
const warnedEvents = new Set<string>()

/** 分派一帧事件。 */
export function routeFrame(event: string, data: Record<string, unknown>, deps: RouteDeps): void {
  const { chat } = deps
  switch (event) {
    case 'stream:open':
      return
    case 'engine:ready':
      void deps.refreshEngines()
      // 引擎是后台预热的：就绪那一刻对话页要从回放态落到实况，
      // 模型、上下文与命令清单都在引擎手里，不刷就一直停在「启动中」。
      void chat.refreshIfIdle()
      return
    case 'engine:detected':
      void deps.refreshEngines()
      return
    case 'app:close_requested': {
      // 首次按关闭：不静默躲进托盘，而是问一次。
      // 静默隐藏是「关不掉」这种抱怨的唯一来源。
      deps.setClosePrompt(true)
      return
    }
    case 'skill:status':
      deps.bumpSkills()
      return
    case 'mcp:changed':
      deps.bumpMcp()
      return
    case 'workspace:changed': {
      // 事实通知不是动作：不再调写接口。是否本机切换的回声由 app 侧
      // 对照当前工作区判断（回声轻量对账，异处变更走全量刷新链路）。
      const ws = asString(data.workspace)
      if (ws) void deps.onWorkspaceChanged(ws)
      return
    }
    case 'models:changed':
      // 服务商或模型增删改：对话页的模型下拉要重拉，否则用户在模型页加完
      // 服务商，输入区里还是加之前那一份——选中的模型可能刚被删掉。
      void chat.refreshModels()
      return
    case 'chat:model': {
      // 模型被切换（可能由别的入口发起）：只换身份字段，其余读数保留；
      // 清单强制重拉一次，选中项若已被删掉 refreshModels 会纠正。
      const provider = asString(data.provider_name)
      const modelId = asString(data.model_id)
      if (!provider || !modelId) return
      const prev = chat.model
      chat.model = prev
        ? { ...prev, provider_name: provider, model_id: modelId }
        : {
            provider_name: provider,
            model_id: modelId,
            model_name: modelId,
            api_protocol: '',
            supports_reasoning: false,
            context_window: 0,
            max_tokens: 0,
          }
      void chat.refreshModels()
      return
    }
    case 'chat:start':
      chat.streaming = true
      chat.promptError = ''
      chat.runError = ''
      // 引擎真正受理的那一刻：慢机冷启动时「点了发送 → 首字到达」能差十几秒，
      // 秒表从这一刻起跑才是「引擎在干活」而非「我在等引擎启动」。
      if (asNumber(data.started_at)) chat.setRunStart(asNumber(data.started_at))
      chat.resetLive()
      return
    case 'chat:live': {
      const runID = asString(data.run_id)
      // 属于另一轮的读数丢掉：切走再切回时旧实例的读数跳进来会让
      // 活动条说一个不属于当前会话的工具名。
      if (runID && chat.turn && runID !== chat.turn.run_id) return
      const phase = asString(data.phase)
      chat.live = {
        run_id: runID,
        phase: phase === 'thinking' || phase === 'text' || phase === 'tool' ? phase : '',
        tool_name: asString(data.tool_name),
        tool_call_id: asString(data.tool_call_id),
        output_tokens: asNumber(data.output_tokens),
        tokens_per_s: sampleSpeed(runID, asNumber(data.output_tokens)),
      }
      // 每一帧读数就是一次「引擎还在动」的证据：活动条靠它判断安静了多久。
      chat.lastActivityAt = Date.now()
      return
    }
    case 'chat:turn': {
      const runID = asString(data.run_id)
      const blocks = readBlocks(data.turn_blocks)
      // 已经收过尾的那一轮，之后的权威快照丢掉：chat:done 后引擎还会补一帧
      // 整表快照，再接受一次界面尾部就多一条重复回复。
      if (!chat.streaming && runID && runID === chat.lastFinishedRunId) return
      if (runID && chat.turn && runID !== chat.turn.run_id) return
      if (!chat.turn || chat.turn.run_id !== runID) {
        chat.turn = { run_id: runID, user_message: asString(data.user_message), blocks }
      } else if (!chat.turn.user_message) {
        chat.turn.user_message = asString(data.user_message)
      }
      return
    }
    case 'chat:patch': {
      const patch = readPatch(data)
      if (patch) chat.applyPatch(patch)
      chat.lastActivityAt = Date.now()
      return
    }
    case 'chat:queue':
      chat.queue = { steering: readStrings(data.steering), follow_up: readStrings(data.follow_up) }
      return
    case 'chat:todos': {
      // 待办清单随工具结果实时推：面板要跟着跑动更新，
      // 等下一轮刷新才看到，用户会以为清单没生效。
      chat.todos = readTodos(data.todos)
      return
    }
    case 'chat:plan':
      // 计划模式开关（可能由 /plan 命令切换）：退出时被拦清单随之清空。
      chat.planMode = data.plan_enabled === true
      if (!chat.planMode) chat.blockedTools = []
      return
    case 'chat:ask': {
      const ask = readAsk(data)
      if (ask) chat.asks = [...chat.asks.filter((a) => a.ask_id !== ask.ask_id), ask]
      return
    }
    case 'chat:retry':
      chat.retry = {
        attempt: asNumber(data.attempt),
        max: asNumber(data.max),
        delay_ms: asNumber(data.delay_ms),
      }
      return
    case 'chat:retry_end':
      chat.retry = null
      return
    case 'chat:compaction': {
      const start = data.state === 'start'
      // 压缩结束要吆一声：活动条消失是静默的，
      // 用户不知道窗口腾没腾出来——这一句就是「压缩完成了」的唯一回执。
      if (chat.compacting && !start) chat.pushNotice('info', i18n.global.t('chat.compactDone'))
      chat.compacting = start
      return
    }
    case 'chat:turn_end':
      // 用量与耗时跟着这一帧来：它比 chat:done 早，
      // 而界面要在收尾时就把读数摆出来。
      chat.setUsage(asString(data.run_id), data.turn_usage as TurnUsagePayload | undefined)
      return
    case 'chat:done':
      chat.finishTurn()
      void chat.refresh(chat.sessionFile)
      void deps.refreshSessions()
      // 引擎这一轮多半动过文件：工作区页开着就该重看一眼。
      deps.bumpWorkspace()
      return
    case 'chat:settled':
      chat.streaming = false
      return
    case 'chat:error': {
      // 这一轮没完成，但它已经结束了——必须同时解除 streaming，
      // 否则活动条会停在「运行中」而没有任何东西在跑（引擎进程死了就是这条路）。
      const message = asString(data.message, i18n.global.t('chat.errorFallback'))
      chat.runError = message
      chat.promptError = message
      chat.streaming = false
      deps.toast('error', message)
      return
    }
    case 'session:info':
    case 'session:title':
      void deps.refreshSessions()
      return
    default:
      if (!warnedEvents.has(event)) {
        warnedEvents.add(event)
        console.warn(`[wb] 未接管的 SSE 事件：${event}`)
      }
      return
  }
}
