// 消息流分组：一次「提问 → 回答」是一个回合（turn）。纯函数，可独立测试。
// 下标必须由函数自己推进：外层再 i++ 会跳过下一条 user，表现为「第二条消息不显示」。
import type { MessageVO } from '../types/api'

interface TurnPart<A> {
  msg: MessageVO
  tools: MessageVO[]
  /** approvals 由调用方挂载：审批属于触发它的回合 */
  approvals?: A
}

export type TurnBlock<A> =
  | { key: string; kind: 'notice'; msg: MessageVO }
  | { key: string; kind: 'user'; msg: MessageVO }
  | { key: string; kind: 'assistant'; parts: TurnPart<A>[] }

export function groupTurns<A>(
  msgs: MessageVO[],
  approvalsFor?: (msg: MessageVO, tools: MessageVO[]) => A,
): TurnBlock<A>[] {
  const out: TurnBlock<A>[] = []
  // parts 为 null 表示「还不在某个回合里」：首个 user 之前的零星助手消息不单独成块
  let parts: TurnPart<A>[] | null = null
  let open = false

  const flush = () => {
    if (parts && parts.length) {
      out.push({ key: `a-${parts[0].msg.message_id}`, kind: 'assistant', parts })
    }
    parts = null
  }

  let i = 0
  while (i < msgs.length) {
    const m = msgs[i]
    switch (m.message_role) {
      case 'user':
        flush()
        out.push({ key: `t-${m.message_id}`, kind: 'user', msg: m })
        open = true
        parts = []
        i++
        break
      case 'notice':
        // 压缩 / 换模型这类分隔条插在流里：它属于两个回合之间，不吞掉后续段
        flush()
        out.push({ key: `n-${m.message_id}`, kind: 'notice', msg: m })
        parts = open ? [] : null
        i++
        break
      case 'assistant': {
        i++
        const tools: MessageVO[] = []
        while (i < msgs.length && msgs[i].message_role === 'tool') {
          tools.push(msgs[i])
          i++
        }
        if (parts) {
          const part: TurnPart<A> = { msg: m, tools }
          if (approvalsFor) part.approvals = approvalsFor(m, tools)
          parts.push(part)
        }
        break
      }
      default:
        // 工具结果由声明它的 assistant 收纳；其它游离消息不单独成块
        i++
        break
    }
  }
  flush()
  return out
}
