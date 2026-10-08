// 消息流分组：一次「提问 → 回答」是一个回合（turn）。
// 这里是纯函数，可独立测试——之前的实现里 for + i=end 的写法
// 会让外层 i++ 恰好跳过下一条 user 消息，表现为「第二条消息永远不显示」。
import type { MessageVO } from '../types/api'

interface TurnPart<A> {
  msg: MessageVO
  tools: MessageVO[]
  /** approvals 由调用方挂载：审批属于触发它的回合 */
  approvals?: A
}

export type TurnBlock<A> =
  | { key: string; kind: 'user'; msg: MessageVO }
  | { key: string; kind: 'assistant'; parts: TurnPart<A>[] }

export function groupTurns<A>(msgs: MessageVO[], approvalsFor?: (msg: MessageVO, tools: MessageVO[]) => A): TurnBlock<A>[] {
  const out: TurnBlock<A>[] = []
  let i = 0
  while (i < msgs.length) {
    const m = msgs[i]
    if (m.role !== 'user') {
      i++ // 游离的助手/工具消息不单独成块
      continue
    }
    out.push({ key: `t-${m.id}`, kind: 'user', msg: m })
    i++
    // 收下这条提问之后的全部助手内容，直到下一条 user 为止
    const parts: TurnPart<A>[] = []
    while (i < msgs.length && msgs[i].role !== 'user') {
      const cur = msgs[i]
      i++
      if (cur.role !== 'assistant') continue // 工具结果由声明它的 assistant 收纳
      const tools: MessageVO[] = []
      while (i < msgs.length && msgs[i].role === 'tool') {
        tools.push(msgs[i])
        i++
      }
      const part: TurnPart<A> = { msg: cur, tools }
      if (approvalsFor) part.approvals = approvalsFor(cur, tools)
      parts.push(part)
    }
    if (parts.length) {
      out.push({ key: `t-${m.id}-a`, kind: 'assistant', parts })
    }
  }
  return out
}
