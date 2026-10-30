// 对话事件桥：SSE → chat store 的全站唯一入口。挂在应用壳上而不是对话页：
// 离开时 run 还在跑，事件必须继续落进 store，否则切回来只剩一份「运行中」的僵尸快照。
import { i18n } from '../i18n'
import type { AskClosedData, AskData } from '../types/api'
import { useChatStore } from '../stores/chat'
import type {
  CompressedData,
  ContextData,
  DeltaData,
  DoneData,
  ErrorData,
  NotifyData,
  RetryData,
  StartData,
  StatusData,
  SwitchedData,
  TitleData,
  ToolEndData,
  ToolStartData,
  UserData,
  WidgetData,
} from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useSse, type SseFrame } from './useSse'

function tt(key: string, args?: Record<string, string | number>): string {
  return i18n.global.t(key, args || {})
}

/** 拉一次权威快照，并把流式那一轮换成历史投影。两个入口共用：事件桥与对话页重挂载。 */
export async function reloadChatSnapshot() {
  const session = useSessionStore()
  const chat = useChatStore()
  try {
    await session.refresh()
  } catch (e) {
    chat.notify(tt('chat.refreshFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    chat.clearLive()
  }
}

/** 在应用壳的 setup 里调用一次：注册事件消费，随应用生命周期存在。 */
export function installChatBridge() {
  const session = useSessionStore()
  const chat = useChatStore()

  async function handle(frame: SseFrame) {
    switch (frame.event) {
      case 'chat:start':
        chat.onStart(frame.data as StartData)
        break
      case 'chat:delta':
        chat.onDelta(frame.data as DeltaData)
        break
      case 'chat:tool_start':
        chat.onToolStart(frame.data as ToolStartData)
        break
      case 'chat:tool_end':
        chat.onToolEnd(frame.data as ToolEndData)
        break
      case 'chat:ask':
        chat.onAsk(frame.data as AskData)
        break
      case 'chat:ask_closed':
        chat.onAskClosed(frame.data as AskClosedData)
        break
      case 'chat:notify':
        chat.onNotify(frame.data as NotifyData)
        break
      case 'chat:status':
        chat.onStatus(frame.data as StatusData)
        break
      case 'chat:widget':
        chat.onWidget(frame.data as WidgetData)
        break
      case 'chat:title':
        chat.onTitle(frame.data as TitleData)
        break
      case 'chat:compressed':
        chat.onCompressed(frame.data as CompressedData)
        break
      case 'chat:user': {
        const d = frame.data as UserData
        session.echoUserMessage(d.content)
        chat.onDequeued(d.content)
        break
      }
      case 'chat:context':
        chat.onContext(frame.data as ContextData)
        break
      case 'chat:retry':
        chat.onRetry(frame.data as RetryData)
        break
      case 'chat:done':
        chat.onDone(frame.data as DoneData)
        await reloadChatSnapshot()
        void session.loadList()
        break
      case 'chat:stopped':
        chat.onStopped()
        await reloadChatSnapshot()
        break
      case 'chat:error':
        chat.onError(frame.data as ErrorData)
        await reloadChatSnapshot()
        break
      case 'chat:gap':
        chat.onGap(frame.data as { missing: number })
        await reloadChatSnapshot()
        break
      case 'chat:switched': {
        chat.onStopped()
        const d = frame.data as SwitchedData
        if (d?.session_id && d.session_id !== session.currentId) {
          await session.open(d.session_id)
        } else {
          await reloadChatSnapshot()
        }
        break
      }
    }
  }

  async function reconcile() {
    chat.onGap()
    await reloadChatSnapshot()
  }

  useSse(
    () => session.currentId,
    async (frame) => {
      // 一条事件处理失败不能拖垮整条流：吞掉异常并留痕，后续事件照常处理。
      try {
        await handle(frame)
      } catch (e) {
        console.error('[WorkBaby] 处理事件失败', frame.event, e)
      }
    },
    reconcile,
  )
}
