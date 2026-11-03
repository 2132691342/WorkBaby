// SSE 事件路由：把一帧事件分派到对应的 store。
// 「后端发了什么 → 界面怎么反应」的完整对照，加一个新事件只改这张表。
// 依赖由调用方以接口传入，本文件不 import 任何 store，避免循环依赖。
import { asNumber, asString, readAppProgress, readAsk, readBlocks, readCommands, readPatch, readStrings } from '@/utils/sse'
import type { AppProgressLog, TurnUsagePayload } from '@/utils/sse'
import i18n from '@/i18n'
import { useChatStore } from './chat'
import { useAgentsStore } from './agents'

/** 对话状态与子代理观测台的实例类型。 */
type ChatStore = ReturnType<typeof useChatStore>
type AgentsStore = ReturnType<typeof useAgentsStore>

/** 路由需要的应用侧能力。 */
export interface RouteDeps {
  /** 对话状态 */
  chat: ChatStore
  /** 子代理观测台 */
  agents: AgentsStore
  /** 重拉引擎状态（探测完成、进程起落时） */
  refreshEngines: () => Promise<unknown>
  /** 重拉会话列表 */
  refreshSessions: () => Promise<unknown>
  /** 重拉应用清单（安装跑完之后） */
  loadApps: (refresh?: boolean) => Promise<unknown>
  /** 断流后拉一次快照对账 */
  reconcile: () => void | Promise<void>
  /** 引擎安装进度（按引擎分桶） */
  appProgress: Record<string, AppProgressLog>
  /** 写回引擎安装进度 */
  setAppProgress: (kind: string, next: AppProgressLog) => void
  /** 用户按了窗口关闭按钮 */
  setClosePrompt: (open: boolean) => void
  /** 引擎要求浏览器完成一次授权 */
  setPendingLink: (link: { url: string; label: string; instructions: string } | null) => void
  /** 弹一条全局提示 */
  toast: (level: 'info' | 'ok' | 'warn' | 'error', message: string) => void
}

/**
 * 哪些事件属于「某一个引擎的某一次对话」。
 *
 * 判据是命名空间：chat:* 是一个会话内的流，subagent:* 与 agents:* 同属于它。
 * engine:* / app:* / session:* 是全局事实（引擎起没起、装到哪一步、最新标题），
 * 两个引擎都会改，过滤掉就会「切过去看到旧状态」。
 */
export function isChatScoped(event: string): boolean {
  return event.startsWith('chat:') || event.startsWith('subagent:') || event.startsWith('agents:')
}

/**
 * 判断一帧是否属于当前正在看的引擎。
 *
 * 两个引擎、以及同一引擎的多个工作目录，共用同一条 SSE。帧上带着
 * engine_kind，不属于当前引擎的聊天帧必须丢掉——不丢的话，切走之后
 * 旧实例仍在推的正文会跳进新会话。
 *
 * 帧上没带 engine_kind 时一律放过：全局事件（安装进度、引擎状态）没有
 * 这个字段，而且老后端也不会发它。宁可多处理，不能把正常帧拦掉。
 */
export function belongsToActive(data: Record<string, unknown>, activeKind: string): boolean {
  const kind = data.engine_kind
  if (typeof kind !== 'string' || !kind) return true
  return kind === activeKind
}

/**
 * 分派一帧事件。
 *
 * activeKind 是当前正在看的引擎：不属于它的聊天帧在这里丢掉。
 */
export function routeFrame(
  event: string,
  data: Record<string, unknown>,
  activeKind: string,
  deps: RouteDeps,
): void {
  const { chat, agents } = deps

  // 两个引擎、多个工作目录共用一条 SSE。聊天与子代理帧只属于某一个引擎，
  // 不属于当前引擎的要丢掉——否则旧实例仍在推的正文会跳进新会话。
  // 引擎状态、安装进度、会话标题是全局的，不能过滤——过滤只做在聊天类命名空间。
  if (isChatScoped(event) && !belongsToActive(data, activeKind)) return

  switch (event) {
    case 'stream:open':
      return
    case 'engine:status': {
      // SSE 连接建立时后端补的一张当前引擎状态。
      // 它盖住「探测完成早于本连接」那条时序：那一帧 engine:detected
      // 发在没有订阅者的时刻会被丢掉，前端就停在「未安装」不动。
      void deps.refreshEngines()
      return
    }
    case 'engine:ready':
    case 'engine:exit':
      void deps.refreshEngines()
      return
    case 'engine:detected':
      // 探测完成了：引擎清单要跟着换，概览页的「未安装」才能换成事实。
      // 这一帧可能永远不来（Bootstrap 触发的那轮探测可能早于 SSE 连接），
      // 所以 detecting 之外还有 detectedAt 那条兜底。
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
      const prev = deps.appProgress[frame.kind]
      const next: AppProgressLog = {
        action: frame.action,
        phase: frame.phase,
        command: frame.phase === 'start' ? asString(frame.command) : (prev?.command ?? ''),
        lines: frame.phase === 'start' ? [] : [...(prev?.lines ?? [])],
        message: asString(frame.message),
      }
      if (frame.phase === 'line') {
        next.lines.push(asString(frame.line))
        // 面板只留最后 400 行：装一个引擎的日志能到几百行，
        // 全留会让长任务把内存吃掉，而用户看的是尾部。
        if (next.lines.length > 400) next.lines = next.lines.slice(-400)
      }
      deps.setAppProgress(frame.kind, next)
      if (frame.phase === 'done' || frame.phase === 'fail') {
        // 结束后重拉清单与引擎状态：装在后台跑完时界面也要跟上。
        void deps.loadApps(true).catch(() => undefined)
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
    case 'engine:extension_error':
      return // 扩展自己的报错不该弹给用户
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
      // 属于另一轮的读数丢掉：两个工作目录共一条 SSE，切过去之后
      // 旧实例的读数跳进来会让活动条说一个不属于当前会话的工具名。
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
      const blocks = readBlocks(data.blocks)
      // 已经收过尾的那一轮，之后的权威快照丢掉：chat:done 后引擎还会补一帧
      // 整表快照，再接受一次界面尾部就多一条重复回复。
      // 判据是「结束过」不是「没在流式」：断流重连时 streaming=false 也可能是真在跑。
      if (!chat.streaming && runID && runID === chat.lastFinishedRunId) return
      // 已经在服务器历史里的那一轮也不再画一次：
      // 引擎在 chat:turn_end 之后会补这一帧，而那时历史可能已经有它了。
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
        chat.turn.blocks = readBlocks(data.blocks)
        chat.lastActivityAt = Date.now()
      }
      return
    }
    case 'chat:queue':
      chat.queue = { steering: readStrings(data.steering), followUp: readStrings(data.followUp) }
      return
    case 'chat:ask': {
      const ask = readAsk(data)
      if (ask) chat.asks = [...chat.asks.filter((a) => a.id !== ask.id), ask]
      return
    }
    case 'chat:notify': {
      const level = asString(data.level, 'info')
      chat.pushNotice(level, asString(data.message))
      return
    }
    case 'chat:status':
      chat.setStatus(asString(data.key), asString(data.text))
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
      chat.setUsage(asString(data.run_id), data.usage as TurnUsagePayload | undefined)
      // 拉一次快照对账：引擎落盘可能比这一帧晚，
      // 而不拉的话这一轮的内容要等 chat:done 才进历史。
      void chat.refresh(chat.sessionFile)
      return
    case 'chat:done':
      chat.finishTurn()
      void chat.refresh(chat.sessionFile)
      void deps.refreshSessions()
      return
    case 'chat:settled':
      chat.streaming = false
      return
    case 'chat:prompt_result': {
      const status = asString(data.status)
      if (status && status !== 'completed') {
        chat.runError = asString(data.message, '这一轮没有完成')
        chat.promptError = chat.runError
      }
      return
    }
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
    case 'chat:open_url': {
      // 优先用 launchUrl（短回环重定向），它比原始长地址更可能在默认浏览器里打开。
      const url = asString(data.launch_url) || asString(data.url)
      if (!url) return
      deps.setPendingLink({
        url,
        label: asString(data.url),
        instructions: asString(data.instructions),
      })
      return
    }
    case 'chat:editor_text': {
      const text = asString(data.text)
      if (text) chat.draftOverride = text
      // 停止时撤回的排队消息：原文回输入框之外，再报一句数，
      // 不然用户不知道「排队的那些话去哪了」。
      const restored = asNumber(data.restored)
      if (restored > 0) {
        deps.toast('info', i18n.global.t('chat.stopRestored', { n: restored }))
      }
      return
    }
    case 'chat:notice': {
      const level = asString(data.level, 'info')
      deps.toast(level === 'error' ? 'error' : level === 'warning' ? 'warn' : 'info', asString(data.message))
      return
    }
    case 'chat:command_output':
      chat.pushNotice('info', asString(data.text))
      return
    case 'chat:gap':
      void deps.reconcile()
      return
    case 'subagent:lifecycle':
      agents.applyLifecycle(data)
      return
    case 'subagent:progress':
      agents.applyProgress(data)
      return
    case 'subagent:event':
      return // 只有 events 档才有；面板不订阅这一档
    case 'agents:cleared':
      // 会话切换后引擎已丢弃旧 id 的帧，本地也必须清干净，
      // 否则界面会一直显示已经不存在的子代理。
      agents.reset()
      return
    case 'session:info':
    case 'session:title':
      void deps.refreshSessions()
      return
    default:
      return
  }
}
