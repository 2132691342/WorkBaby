// 一轮对话的「过程读数」：状态文字、引擎小组件、提示条、重试进度。
// 它们用完即弃、下一轮重来，与要跨轮保留的会话状态分开，
// 「reset 要清哪些」才说得清。
import { ref } from 'vue'
import type { ChatLive } from '@/types/api'

/** 一条过程提示。 */
export interface Notice {
  id: number
  level: string
  message: string
}

/** 引擎报上来的一组读数（多行）。 */
export interface Widget {
  key: string
  lines: string[]
}

/** 重试进度。 */
export interface RetryState {
  attempt: number
  max: number
  delay_ms: number
}

/**
 * 一轮对话的过程读数。
 */
export function useRunStatus() {
  const statusText = ref('')
  const widgets = ref<Widget[]>([])
  const notices = ref<Notice[]>([])
  const retry = ref<RetryState | null>(null)
  // live 是引擎报的实时阶段：思考中 / 写正文 / 跑某个工具。
  // 它与 statusText 不是一回事：statusText 是扩展自己设的一句话，
  // live 是引擎自身的阶段，不依赖任何扩展。
  const live = ref<ChatLive>({ run_id: '', phase: '', tool_name: '', tool_call_id: '', output_tokens: 0 })
  // lastActivityAt 是「最后一次看到引擎动」的时刻（毫秒）。
  // 它与 startedAt 分开：秒表回答「这一轮跑了多久」，它回答「上一次有新东西是多久之前」。
  // 两者之差就是「安静了多久」，而那正是「卡住了没有」的唯一客观依据。
  const lastActivityAt = ref(0)
  let noticeSeq = 0

  /** 推一条过程提示。最多留四条：更多的话屏幕会被它占满。 */
  function pushNotice(level: string, message: string): void {
    notices.value = [...notices.value, { id: ++noticeSeq, level, message }].slice(-4)
  }

  function dismissNotice(id: number): void {
    notices.value = notices.value.filter((n) => n.id !== id)
  }

  /**
   * 设状态文字。
   *
   * key 与 text 任一为空就清掉：引擎报「这一项没有值」时，
   * 界面上留一条空行比不留更糟。
   */
  function setStatus(key: string, text: string): void {
    if (key === '' || text === '') {
      statusText.value = ''
      return
    }
    statusText.value = text
  }

  /** 设一组读数；给了空行就撤掉这一组。 */
  function setWidget(key: string, lines: string[]): void {
    const rest = widgets.value.filter((w) => w.key !== key)
    widgets.value = (lines ?? []).length ? [...rest, { key, lines }] : rest
  }

  /** 清空这一轮的过程读数（收尾或换会话时）。 */
  function reset(): void {
    statusText.value = ''
    widgets.value = []
    notices.value = []
    retry.value = null
    live.value = { run_id: '', phase: '', tool_name: '', tool_call_id: '', output_tokens: 0 }
    lastActivityAt.value = 0
  }

  return {
    statusText,
    widgets,
    notices,
    retry,
    live,
    lastActivityAt,
    pushNotice,
    dismissNotice,
    setStatus,
    setWidget,
    reset,
  }
}
