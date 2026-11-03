// 提问卡状态：内核在等用户回答时弹出的卡片，以及回答的回填。
// 拆出来的理由只有一条——提问卡有独立的生命周期（可以跨好几轮才被回答），
// 与「本轮运行」的流式状态混在一起会让两边互相干扰。
import { ref } from 'vue'
import * as api from '../api'
import { useToastStore } from '../stores/toast'
import { i18n } from '../i18n'
import { errorText } from '../utils/errors'
import type { AskData, AskClosedData } from '../types/api'
import type { PendingAsk } from '../types/chatEvents'

/** 状态条的文案在 store 里也走 i18n：语言一换，提示条不能还是旧语。 */
function tt(key: string, args?: Record<string, string | number>): string {
  return i18n.global.t(key, args || {})
}

/** 提问卡：挂到内核提问（ask_user）与安全闸门（审批）两处。 */
export function useAskBoard() {
  const asks = ref<PendingAsk[]>([])

  /** 收到一张提问卡。重复 id 直接跳过：断线重连会把同一张卡再送一次。 */
  function onAsk(data: AskData) {
    if (asks.value.some((a) => a.id === data.id)) return
    asks.value.push({
      id: data.id,
      method: data.method,
      title: data.title,
      message: data.message,
      options: data.options ?? [],
      questions: data.questions ?? [],
      placeholder: data.placeholder ?? '',
      prefill: data.prefill ?? '',
      timeoutMs: data.timeout_ms ?? 0,
      approval_tool: data.approval_tool,
      approval_summary: data.approval_summary,
    })
  }

  /** 提问已关闭（回答完 / 超时 / 会话中止）：卡片收起来。 */
  function onAskClosed(data: AskClosedData) {
    asks.value = asks.value.filter((a) => a.id !== data.id)
  }

  /** 回答引擎的提问：confirm 走 confirmed，其余走 value / cancelled。 */
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

  /** 清空（新一轮开始时用）。 */
  function clear() {
    asks.value = []
  }

  return { asks, onAsk, onAskClosed, answer, clear }
}
