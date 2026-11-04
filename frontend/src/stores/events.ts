// SSE 事件路由：把一帧事件分派到对应的 store。
// 「后端发了什么 → 界面怎么反应」的完整对照，加一个新事件只改这张表。
// 依赖由调用方以接口传入，本文件不 import 任何 store，避免循环依赖。
import {
  asNumber,
  asString,
  readAppProgress,
  readAsk,
  readBlocks,
  readCommands,
  readPatch,
  readStrings,
} from '@/utils/sse'
import type { AppProgressLog, TurnUsagePayload } from '@/utils/sse'
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
  /** 重拉应用清单（安装跑完之后） */
  loadApps: () => Promise<unknown>
  /** 断流后拉一次快照对账 */
  reconcile: () => void | Promise<void>
  /** 安装进度（同一个时刻只有一条长任务，所以只留一份） */
  appProgress: AppProgressLog | null
  /** 写回安装进度 */
  setAppProgress: (next: AppProgressLog | null) => void
  /** 用户按了窗口关闭按钮 */
  setClosePrompt: (open: boolean) => void
  /** 弹一条全局提示 */
  toast: (level: 'info' | 'ok' | 'warn' | 'error', message: string) => void
}

/**
 * 哪些事件属于「某一次对话」。
 *
 * 判据是命名空间：chat:* 是一个会话内的流。engine:* / app:* / session:*
 * 是全局事实（引擎起没起、装到哪一步、最新标题），不能按运行过滤，
 * 否则会出现「装完了界面还在转圈」。
 */
export function isChatScoped(event: string): boolean {
  return event.startsWith('chat:')
}

/** 分派一帧事件。 */
export function routeFrame(event: string, data: Record<string, unknown>, deps: RouteDeps): void {
  const { chat } = deps
  switch (event) {
    case 'stream:open':
      return
    case 'engine:status':
      // SSE 连接建立时后端补的一张当前状态，也用于安装完成后的刷新。
      void deps.refreshEngines()
      return
    case 'engine:ready':
    case 'engine:exit':
    case 'engine:detected':
      void deps.refreshEngines()
      return
    case 'engine:commands': {
      const list = readCommands(data.commands)
      if (list.length) chat.commands = list
      return
    }
    case 'app:progress': {
      const frame = readAppProgress(data)
      if (!frame) return
      const prev = deps.appProgress
      // start 开一份新日志，line 追加，done / fail 收尾。
      if (frame.progress_phase === 'start') {
        deps.setAppProgress({ action: frame.progress_action, phase: 'start', lines: [], message: '' })
        return
      }
      const next: AppProgressLog = {
        action: frame.progress_action,
        phase: frame.progress_phase,
        lines: [...(prev?.lines ?? [])],
        message: frame.progress_message ?? '',
      }
      if (frame.progress_phase === 'line' && frame.progress_line) {
        next.lines.push(frame.progress_line)
        // 面板只留最后 400 行：装 pi 与插件的日志能到几百行，
        // 全留会让长任务把内存吃掉，而用户看的是尾部。
        if (next.lines.length > 400) next.lines = next.lines.slice(-400)
      }
      deps.setAppProgress(next)
      if (frame.progress_phase === 'done' || frame.progress_phase === 'fail') {
        // 结束后重拉清单与状态：装在后台跑完时界面也要跟上。
        void deps.loadApps().catch(() => undefined)
        void deps.refreshEngines().catch(() => undefined)
      }
      return
    }
    case 'app:close_requested': {
      // 首次按关闭：不静默躲进托盘，而是问一次。
      // 静默隐藏是「关不掉」这种抱怨的唯一来源。
      deps.setClosePrompt(true)
      return
    }
    case 'plugin:status':
    case 'skill:status':
      // 这两类变化由各自的页面在动作后自己重拉，这里只提示一下。
      return
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
    case 'chat:blocks': {
      if (chat.turn && asString(data.run_id) === chat.turn.run_id) {
        chat.turn.blocks = readBlocks(data.turn_blocks)
        chat.lastActivityAt = Date.now()
      }
      return
    }
    case 'chat:queue':
      chat.queue = { steering: readStrings(data.steering), follow_up: readStrings(data.follow_up) }
      return
    case 'chat:ask': {
      const ask = readAsk(data)
      if (ask) chat.asks = [...chat.asks.filter((a) => a.ask_id !== ask.ask_id), ask]
      return
    }
    case 'chat:notify': {
      const level = asString(data.level, 'info')
      chat.pushNotice(level, asString(data.message))
      return
    }
    case 'chat:status':
      chat.setStatus(asString(data.key), asString(data.block_text))
      return
    case 'chat:widget':
      chat.setWidget(asString(data.key), readStrings(data.lines))
      return
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
    case 'chat:compaction':
      chat.compacting = data.state === 'start'
      return
    case 'chat:turn_end':
      // 用量与耗时跟着这一帧来：它比 chat:done 早，
      // 而界面要在收尾时就把读数摆出来。
      chat.setUsage(asString(data.run_id), data.turn_usage as TurnUsagePayload | undefined)
      return
    case 'chat:done':
      chat.finishTurn()
      void chat.refresh(chat.sessionFile)
      void deps.refreshSessions()
      return
    case 'chat:settled':
      chat.streaming = false
      return
    case 'chat:error': {
      // 这一轮没完成，但它已经结束了——必须同时解除 streaming，
      // 否则活动条会停在「运行中」而没有任何东西在跑（引擎进程死了就是这条路）。
      const message = asString(data.message, '出错了')
      chat.runError = message
      chat.promptError = message
      chat.streaming = false
      deps.toast('error', message)
      return
    }
    case 'chat:editor_text': {
      const text = asString(data.block_text)
      if (text) chat.draftOverride = text
      return
    }
    case 'chat:notice': {
      const level = asString(data.level, 'info')
      deps.toast(level === 'error' ? 'error' : level === 'warning' ? 'warn' : 'info', asString(data.message))
      return
    }
    case 'chat:command_output':
      chat.pushNotice('info', asString(data.block_text))
      return
    case 'chat:gap':
      void deps.reconcile()
      return
    case 'session:info':
    case 'session:title':
      void deps.refreshSessions()
      return
    default:
      return
  }
}
