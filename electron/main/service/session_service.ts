//! 会话服务：清单、详情、改名、删除、隐藏台账、历史轮与 HTML 导出。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { getSetting, saveSettings } from '../repo/settings_repo'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_NOT_FOUND, CODE_SESSION_IO } from '../domain/errors'
import { parseDiffLines } from '../domain/types_data'
import {
  activePath,
  readEntries,
  readHeader,
  textOf,
  type PiEntry,
  type PiMessage,
  type PiUsage,
} from './parser/session_tree'
import type {
  SessionDetailVO,
  SessionExportVO,
  SessionItemVO,
  SessionRowVO,
} from '../domain/types_data'
import type { ChatBlockVO, HistoryTurnVO, TurnUsageVO } from '../domain/types_chat'

/** 会话根目录。 */
export function sessionsRoot(): string {
  return path.join(paths().wb_pi_agent_dir, 'sessions')
}

/** 工作区编码：剥一个头部分隔符，/ \ : 换 -，两端包 --（与 pi 落盘规则一致）。 */
export function encodedWorkspace(ws: string): string {
  const stripped = ws.replace(/^[\\/]/, '').replace(/[\\/: ]/g, '-')
  return `--${stripped}--`
}

function guardSession(file: string): string {
  const resolved = path.resolve(file)
  const root = path.resolve(sessionsRoot())
  if (!resolved.startsWith(root + path.sep)) {
    throw appError(CODE_SESSION_IO, '会话路径越界', resolved)
  }
  if (!fs.existsSync(resolved)) throw appError(CODE_NOT_FOUND, '会话文件不存在', resolved)
  return resolved
}

/** 单文件摘要：标题、模型、token、消息数。 */
function summarize(file: string): SessionRowVO {
  const header = readHeader(file)
  const entries = readEntries(file)
  const row: SessionRowVO = {
    id: header?.id ?? path.basename(file, '.jsonl'),
    path: file,
    title: '',
    cwd: header?.cwd ?? '',
    work_dir: header?.cwd ?? '',
    model: '',
    updated_at: entries.length > 0 ? (entries[entries.length - 1].timestamp ?? '') : (header?.timestamp ?? ''),
    size: 0,
    messages: 0,
    tokens: 0,
    cost: 0,
  }
  try {
    row.size = fs.statSync(file).size
  } catch {
    /* 读不到就算了 */
  }
  for (const e of entries) {
    const msg = e.message
    if (msg?.usage) addUsage(row, msg.usage)
    if (e.type === 'usage' && e.usage) addUsage(row, e.usage)
    if (msg?.role === 'user' && !row.title) {
      const text = textOf(msg.content).trim()
      if (text) row.title = text.slice(0, 80)
    }
    if (msg?.role === 'assistant') {
      row.messages += 1
      if (msg.model) row.model = String(msg.model)
    } else if (msg?.role === 'user' || msg?.role === 'toolResult') {
      row.messages += 1
    }
    if (e.type === 'model_change' && e.provider && e.modelId) {
      row.model = `${e.provider}/${e.modelId}`
    }
  }
  const info = entries.filter((e) => e.type === 'session_info').pop()
  if (info?.name) row.title = String(info.name)
  if (!row.title) row.title = '（空会话）'
  return row
}

function addUsage(row: SessionRowVO, usage: PiUsage): void {
  row.tokens += usage.totalTokens ?? 0
  row.cost += usage.cost?.total ?? 0
}

/** 会话清单：指定工作区只扫它的目录，否则扫全部。 */
export function sessionsList(workspace?: string): { sessions: SessionRowVO[] } {
  const root = sessionsRoot()
  const dirs: string[] = []
  if (workspace) {
    const dir = path.join(root, encodedWorkspace(workspace))
    if (fs.existsSync(dir)) dirs.push(dir)
  } else {
    try {
      dirs.push(
        ...fs
          .readdirSync(root, { withFileTypes: true })
          .filter((d) => d.isDirectory())
          .map((d) => path.join(root, d.name)),
      )
    } catch {
      return { sessions: [] }
    }
  }
  const rows: SessionRowVO[] = []
  for (const dir of dirs) {
    let files: string[] = []
    try {
      files = fs.readdirSync(dir).filter((f) => f.endsWith('.jsonl'))
    } catch {
      continue
    }
    for (const f of files) {
      try {
        rows.push(summarize(path.join(dir, f)))
      } catch {
        /* 单个坏文件不影响清单 */
      }
    }
  }
  rows.sort((a, b) => b.updated_at.localeCompare(a.updated_at))
  return { sessions: rows }
}

/** 会话详情：清单行 + 逐条消息。 */
export function sessionDetail(file: string): SessionDetailVO {
  const resolved = guardSession(file)
  const row = summarize(resolved)
  const entries = readEntries(resolved)
  const items: SessionItemVO[] = []
  for (const e of activePath(entries)) {
    const msg = e.message
    if (!msg) continue
    if (msg.role === 'user') {
      items.push({ kind: 'user', role: 'user', text: textOf(msg.content), timestamp: e.timestamp })
    } else if (msg.role === 'assistant') {
      items.push({
        kind: 'assistant',
        role: 'assistant',
        text: textOf(msg.content),
        model: msg.model ? String(msg.model) : undefined,
        timestamp: e.timestamp,
        cost: msg.usage?.cost?.total,
        tokens: msg.usage?.totalTokens,
      })
    } else if (msg.role === 'toolResult') {
      items.push({
        kind: 'tool',
        role: 'toolResult',
        text: textOf(msg.content),
        tool_name: msg.toolName ? String(msg.toolName) : undefined,
        tool_call_id: msg.toolCallId ? String(msg.toolCallId) : undefined,
        is_error: msg.isError === true,
        timestamp: e.timestamp,
      })
    }
  }
  return { ...row, thinking: true, items }
}

// ---- 隐藏台账 ----

function hideKey(file: string): string {
  return `chat.hide:${file}`
}

/** 当前会话的隐藏 hid 集合。 */
export function hiddenSet(file: string): Set<string> {
  const raw = getSetting(hideKey(file))
  if (!raw) return new Set()
  try {
    const parsed: unknown = JSON.parse(raw)
    return new Set(Array.isArray(parsed) ? (parsed as string[]) : [])
  } catch {
    return new Set()
  }
}

function saveHidden(file: string, set: Set<string>): void {
  saveSettings({ [hideKey(file)]: JSON.stringify([...set]) })
}

/** 隐藏一轮。 */
export function hideTurn(file: string, hid: string): void {
  const set = hiddenSet(file)
  set.add(hid)
  saveHidden(file, set)
}

/** 取消隐藏。 */
export function unhideTurn(file: string, hid: string): void {
  const set = hiddenSet(file)
  set.delete(hid)
  saveHidden(file, set)
}

// ---- 历史轮 ----

function usageOf(u?: PiUsage): TurnUsageVO | undefined {
  if (!u) return undefined
  return {
    input_tokens: u.input ?? 0,
    output_tokens: u.output ?? 0,
    cache_read_tokens: u.cacheRead ?? 0,
    cache_write_tokens: u.cacheWrite ?? 0,
  }
}

function blocksFromAssistant(msg: PiMessage): { blocks: ChatBlockVO[]; tool_ids: Set<string> } {
  const blocks: ChatBlockVO[] = []
  const tool_ids = new Set<string>()
  const content = Array.isArray(msg.content) ? (msg.content as Record<string, unknown>[]) : []
  for (const b of content) {
    if (b.type === 'text' && typeof b.text === 'string') {
      blocks.push({ kind: 'text', text: b.text })
    } else if (b.type === 'thinking' && typeof b.thinking === 'string') {
      blocks.push({ kind: 'thinking', text: b.thinking })
    } else if (b.type === 'toolCall' && typeof b.id === 'string') {
      tool_ids.add(b.id)
      blocks.push({
        kind: 'tool',
        tool_call_id: b.id,
        tool_name: String(b.name ?? ''),
        args: (b.arguments as Record<string, unknown>) ?? {},
        status: 'done',
        output: '',
      })
    }
  }
  return { blocks, tool_ids }
}

function applyToolResult(block: ChatBlockVO, msg: PiMessage): void {
  block.output = textOf(msg.content)
  block.is_error = msg.isError === true
  block.status = msg.isError === true ? 'error' : 'done'
  const details = msg.details as Record<string, unknown> | undefined
  const diff = details && typeof details.diff === 'string' ? details.diff : undefined
  if (diff) block.diff = parseDiffLines(diff)
}

/** 活动分支 → 历史轮（隐藏的轮次剔除）。 */
export function buildHistory(file: string): HistoryTurnVO[] {
  const entries = readEntries(file)
  return historyFromEntries(entries, hiddenSet(file))
}

/** 给 chat 服务复用：任意条目集 + 隐藏集 → 历史轮。 */
export function historyFromEntries(entries: PiEntry[], hidden: Set<string>): HistoryTurnVO[] {
  const turns: HistoryTurnVO[] = []
  let assistant: HistoryTurnVO | null = null
  let tool_ids = new Set<string>()
  const flush = (): void => {
    assistant = null
    tool_ids = new Set()
  }
  for (const e of activePath(entries)) {
    if (e.type === 'compaction') {
      flush()
      turns.push({
        role: 'engine-mark',
        blocks: [{ kind: 'text', text: `已压缩上下文（原 ${e.tokensBefore ?? '?'} tokens）` }],
        hid: e.id,
      })
      continue
    }
    if (e.type === 'custom_message' && e.display === true) {
      flush()
      const text = typeof e.content === 'string' ? e.content : ''
      if (text) turns.push({ role: 'engine-mark', blocks: [{ kind: 'text', text }], hid: e.id })
      continue
    }
    const msg = e.message
    if (!msg) continue
    if (msg.role === 'user') {
      flush()
      const text = textOf(msg.content)
      if (text.trim()) turns.push({ role: 'user', blocks: [{ kind: 'text', text }], hid: e.id })
    } else if (msg.role === 'assistant') {
      const built = blocksFromAssistant(msg)
      assistant = {
        role: 'assistant',
        blocks: built.blocks,
        model: msg.model ? String(msg.model) : undefined,
        usage: usageOf(msg.usage),
        hid: e.id,
      }
      tool_ids = built.tool_ids
      turns.push(assistant)
    } else if (msg.role === 'toolResult') {
      const id = String(msg.toolCallId ?? '')
      const block = assistant && tool_ids.has(id) ? (assistant as HistoryTurnVO).blocks.find((b) => b.tool_call_id === id) : undefined
      if (block) applyToolResult(block, msg)
      else {
        const standalone: ChatBlockVO = {
          kind: 'tool',
          tool_call_id: id,
          tool_name: String(msg.toolName ?? ''),
          status: msg.isError ? 'error' : 'done',
        }
        applyToolResult(standalone, msg)
        turns.push({ role: 'tool', blocks: [standalone], hid: e.id })
      }
    }
  }
  return turns.filter((t) => !t.hid || !hidden.has(t.hid))
}

/** 删除会话文件。 */
export function sessionDelete(file: string): { ok: boolean } {
  const resolved = guardSession(file)
  try {
    fs.rmSync(resolved, { force: true })
  } catch (err) {
    throw wrapError(CODE_SESSION_IO, '会话删除失败', err)
  }
  return { ok: true }
}

/** 改名：追加一条 session_info 条目（pi 的改名方式）。 */
export function sessionRename(file: string, name: string): { ok: boolean } {
  const resolved = guardSession(file)
  const entries = readEntries(resolved)
  const leaf = entries.length > 0 ? entries[entries.length - 1] : null
  const entry = {
    type: 'session_info',
    id: Math.random().toString(16).slice(2, 10),
    parentId: leaf?.id ?? null,
    timestamp: new Date().toISOString(),
    name,
  }
  try {
    fs.appendFileSync(resolved, `${JSON.stringify(entry)}\n`, 'utf8')
  } catch (err) {
    throw wrapError(CODE_SESSION_IO, '会话改名失败', err)
  }
  return { ok: true }
}

/** 导出 HTML：历史轮渲染为单文件。 */
export function sessionExport(file: string): SessionExportVO {
  const resolved = guardSession(file)
  const history = buildHistory(resolved)
  const esc = (s: string): string =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const rows = history
    .map((turn) => {
      const label =
        turn.role === 'user' ? '用户' : turn.role === 'assistant' ? '助手' : turn.role === 'tool' ? '工具' : '系统'
      const body = turn.blocks
        .map((b) => {
          if (b.kind === 'tool') {
            return `<div class="tool"><b>${esc(b.tool_name ?? 'tool')}</b><pre>${esc(b.output ?? '')}</pre></div>`
          }
          if (b.kind === 'thinking') return `<div class="think">${esc(b.text ?? '')}</div>`
          return `<div class="md">${esc(b.text ?? '')}</div>`
        })
        .join('\n')
      const meta = turn.model ? ` <small>${esc(turn.model)}</small>` : ''
      return `<section class="${turn.role}"><h3>${label}${meta}</h3>${body}</section>`
    })
    .join('\n')
  const title = path.basename(resolved, '.jsonl')
  const html = `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>${esc(title)}</title>
<style>body{font-family:system-ui,sans-serif;max-width:860px;margin:24px auto;padding:0 16px;color:#1e293b}
section{margin:18px 0;padding:12px 16px;border-radius:10px;background:#f8fafc}
section.user{background:#e0e7ff}pre{white-space:pre-wrap;word-break:break-all;background:#fff;padding:8px;border-radius:6px}
.think{color:#64748b;font-style:italic}small{color:#64748b}</style></head><body><h1>${esc(title)}</h1>${rows}</body></html>`
  return { file_name: `${title}.html`, content: html }
}

/** 会话总数（健康面板用）。 */
export function sessionCount(): number {
  return sessionsList().sessions.length
}
