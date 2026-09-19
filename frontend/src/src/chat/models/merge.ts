import type { Message as ApiMessage } from '@/types/api'

/** 乐观消息与权威消息的配对键（同角色 + 前 120 字相同视为同一条）。 */
export function contentKey(m: ApiMessage): string {
  return `${m.role}|${(m.content ?? '').slice(0, 120)}`
}

/**
 * 尚未产出正文的 assistant 占位：流式期间由流式气泡单独呈现。
 *
 * <p>merge 时跳过——否则列表里会同时出现「空 assistant 气泡」与流式气泡，
 * 视觉上就是同一轮回复被展示两遍。
 */
export function isPendingAssistant(m: ApiMessage): boolean {
  return (
    m.role === 'assistant' &&
    !(m.content ?? '').trim() &&
    m.status !== 'completed' &&
    m.status !== 'failed'
  )
}

/**
 * 权威快照 → UI 消息列表。
 *
 * <p>role=tool 行是给 LLM 上下文重建用的（多轮 ReAct 回灌），工具过程已随
 * assistant 消息的 message_blocks 持久化并由 TaskTimeline 回放；不过滤的话
 * 它会被当成 assistant 气泡，把工具结果原文整段再渲染一遍。
 */
export function toUiMessages(loaded: ApiMessage[]): ApiMessage[] {
  return loaded.filter((m) => m.role !== 'tool')
}

/**
 * 流式收尾决策：判断本次 run 的回复在权威快照中的落位。
 * 返回 dedupe（快照已含，不重复 push）/ fill（空占位，用流式内容补齐）/ missing（快照缺失，允许本地兜底）。
 * 本 run 的 assistant 取「最后一条 user 之后的消息」，工具场景下它不在末条。
 */
export function resolveRunAssistant(
  messages: ApiMessage[],
  content: string
): 'dedupe' | { kind: 'fill'; message: ApiMessage } | 'missing' {
  if (content && messages.some((m) => m.role === 'assistant' && (m.content ?? '').trim() === content)) {
    return 'dedupe'
  }
  let lastUserIdx = -1
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'user') {
      lastUserIdx = i
      break
    }
  }
  for (let i = messages.length - 1; i > lastUserIdx; i--) {
    const m = messages[i]
    if (m.role !== 'assistant') continue
    if (!(m.content ?? '').trim()) return { kind: 'fill', message: m }
    return 'dedupe'
  }
  return 'missing'
}

/**
 * 本地乐观消息与权威快照合并。
 *
 * <p>本地消息优先按 id、其次按内容配对到权威行（用权威行替换，保证拿到落库后的
 * 用量与过程块）；权威新增项按原序补在末尾；权威已删除的本地行直接丢弃。
 */
export function mergeLoadedMessages(local: ApiMessage[], loaded: ApiMessage[]): ApiMessage[] {
  if (local.length === 0) return loaded
  const byContent = new Map<string, ApiMessage>()
  const byID = new Map<string, ApiMessage>()
  for (const m of loaded) {
    if (isPendingAssistant(m)) continue
    const k = contentKey(m)
    if (!byContent.has(k)) byContent.set(k, m)
    byID.set(m.id, m)
  }
  const result: ApiMessage[] = []
  const consumed = new Set<string>()
  for (const m of local) {
    if (m.id.startsWith('local-')) {
      const hit = byContent.get(contentKey(m))
      if (hit && !consumed.has(hit.id)) {
        result.push(hit)
        consumed.add(hit.id)
        continue
      }
      result.push(m)
      continue
    }
    const hit = byID.get(m.id)
    if (hit) {
      result.push(hit)
      consumed.add(hit.id)
    }
    // 权威已无此 id → 消息已被删除/截断，丢弃
  }
  for (const m of loaded) {
    if (isPendingAssistant(m)) continue
    if (!consumed.has(m.id)) result.push(m)
  }
  return result
}
