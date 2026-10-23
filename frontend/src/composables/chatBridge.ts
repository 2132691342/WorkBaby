// 对话事件桥：SSE → chat store 的全站唯一入口。
// 挂在应用壳上而不是对话页：离开对话页时 run 还在跑，事件必须继续落进 store；
// 否则切回来只剩一份「运行中」的僵尸快照，正文与工具卡全部接不上。
import { i18n } from '../i18n'
import type { ServerEvent } from '../types/api'
import { useChatStore } from '../stores/chat'
import { useSessionStore } from '../stores/session'
import { useSse } from './useSse'

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

  async function handle(env: ServerEvent) {
    switch (env.event) {
      case 'chat:start':
        chat.onStart(env.data)
        break
      case 'chat:delta':
        chat.onDelta(env.data)
        break
      case 'chat:tool_start':
        chat.onToolStart(env.data)
        break
      case 'chat:tool_end':
        chat.onToolEnd(env.data)
        break
      case 'chat:ask':
        chat.onAsk(env.data)
        break
      case 'chat:ask_closed':
        chat.onAskClosed(env.data)
        break
      case 'chat:notify':
        chat.onNotify(env.data)
        break
      case 'chat:status':
        chat.onStatus(env.data)
        break
      case 'chat:widget':
        chat.onWidget(env.data)
        break
      case 'chat:title':
        chat.onTitle(env.data)
        break
      case 'chat:compressed':
        chat.onCompressed(env.data)
        break
      case 'chat:user':
        session.echoUserMessage(env.data.content)
        chat.onDequeued(env.data.content)
        break
      case 'chat:context':
        chat.onContext(env.data)
        break
      case 'chat:retry':
        chat.onRetry(env.data)
        break
      case 'chat:done':
        chat.onDone(env.data)
        await reloadChatSnapshot()
        void session.loadList()
        break
      case 'chat:stopped':
        chat.onStopped()
        await reloadChatSnapshot()
        break
      case 'chat:error':
        chat.onError(env.data)
        await reloadChatSnapshot()
        break
      case 'chat:gap':
        chat.onGap()
        await reloadChatSnapshot()
        break
      case 'chat:switched':
        chat.onStopped()
        if (env.data?.session_id && env.data.session_id !== session.currentId) {
          await session.open(env.data.session_id)
        } else {
          await reloadChatSnapshot()
        }
        break
    }
  }

  async function reconcile() {
    chat.onGap()
    await reloadChatSnapshot()
  }

  useSse(
    () => session.currentId,
    async (env) => {
      // 一条事件处理失败不能拖垮整条流：吞掉异常并留痕，后续事件照常处理。
      try {
        await handle(env)
      } catch (e) {
        console.error('[WorkBaby] 处理事件失败', env.event, e)
      }
    },
    reconcile,
  )
}
