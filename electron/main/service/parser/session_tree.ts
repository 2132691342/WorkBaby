//! pi 会话 JSONL 解析：条目树、活动分支与内容块的公共读取。
import fs from 'node:fs'

/** 会话条目（字段名与 pi 的落盘格式一致）。 */
export interface PiEntry {
  type: string
  id: string
  parentId: string | null
  timestamp: string
  message?: PiMessage
  provider?: string
  modelId?: string
  thinkingLevel?: string
  summary?: string
  firstKeptEntryId?: string
  tokensBefore?: number
  usage?: PiUsage
  kind?: string
  name?: string
  customType?: string
  content?: string
  display?: boolean
  [key: string]: unknown
}

/** pi 消息形状（只取我们消费的字段）。 */
export interface PiMessage {
  role: string
  content?: unknown
  timestamp?: number
  model?: string
  provider?: string
  usage?: PiUsage
  stopReason?: string
  errorMessage?: string
  toolCallId?: string
  toolName?: string
  isError?: boolean
  [key: string]: unknown
}

/** pi 的用量对象。 */
export interface PiUsage {
  input?: number
  output?: number
  cacheRead?: number
  cacheWrite?: number
  totalTokens?: number
  cost?: { input?: number; output?: number; cacheRead?: number; cacheWrite?: number; total?: number }
}

/** 读全部条目（首行 session 头不算树节点）。 */
export function readEntries(file: string): PiEntry[] {
  const text = fs.readFileSync(file, 'utf8')
  const out: PiEntry[] = []
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    try {
      const parsed = JSON.parse(trimmed) as PiEntry
      if (parsed.type !== 'session') out.push(parsed)
    } catch {
      // 坏行跳过：会话文件允许中途断电产生的残行。
    }
  }
  return out
}

/** 读会话头（首行）。 */
export function readHeader(file: string): { id: string; cwd: string; timestamp: string } | null {
  try {
    const text = fs.readFileSync(file, 'utf8')
    const first = text.split('\n').find((l) => l.trim().startsWith('{'))
    if (!first) return null
    const parsed = JSON.parse(first) as { type?: string; id?: string; cwd?: string; timestamp?: string }
    if (parsed.type !== 'session') return null
    return { id: parsed.id ?? '', cwd: parsed.cwd ?? '', timestamp: parsed.timestamp ?? '' }
  } catch {
    return null
  }
}

/** 活动分支：从最后一个条目沿 parentId 走到根，再反转成时间序。 */
export function activePath(entries: PiEntry[]): PiEntry[] {
  if (entries.length === 0) return []
  const by_id = new Map<string, PiEntry>()
  for (const e of entries) by_id.set(e.id, e)
  const chain: PiEntry[] = []
  let cur: PiEntry | undefined = entries[entries.length - 1]
  const guard = new Set<string>()
  while (cur && !guard.has(cur.id)) {
    guard.add(cur.id)
    chain.push(cur)
    cur = cur.parentId ? by_id.get(cur.parentId) : undefined
  }
  return chain.reverse()
}

/** 从消息 content 里抠纯文本：字符串直接用，块数组拼 text 块。 */
export function textOf(content: unknown): string {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return ''
  const parts: string[] = []
  for (const block of content as Record<string, unknown>[]) {
    if (block?.type === 'text' && typeof block.text === 'string') parts.push(block.text)
  }
  return parts.join('\n')
}
