//! 事件翻译：pi 的会话事件 → 前端 SSE 词汇（chat:*）与实时块表。
import { broadcast, broadcastRun, newRunId } from '../../sse/hub'
import { nowMs } from '../../pkg/fsutil'
import { parseDiffLines } from '../../domain/types_data'
import type { ChatBlockVO } from '../../domain/types_chat'

/** pi 事件里的 usage 形状。 */
interface PiUsageWire {
  input?: number
  output?: number
  cacheRead?: number
  cacheWrite?: number
  cost?: { total?: number }
}

interface ToolWire {
  id?: string
  name?: string
  arguments?: Record<string, unknown>
}

/**
 * 一次运行（run）的实时翻译器：维护块表，把 pi 增量事件转成前端帧。
 * 生命周期：chat:start → patch/turn → turn_end → done/settled。
 */
export class ChatStream {
  run_id = ''
  blocks: ChatBlockVO[] = []
  user_message = ''
  streaming = false
  private usage: PiUsageWire | null = null

  /** 用户发消息后调：登记用户话术，进入流式。 */
  begin(message: string): string {
    this.run_id = newRunId()
    this.blocks = []
    this.user_message = message
    this.usage = null
    this.streaming = true
    broadcastRun('chat:start', this.run_id, { started_at: nowMs() })
    return this.run_id
  }

  /** pi 会话事件入口；UI 请求不在这处理。 */
  onEvent(ev: Record<string, unknown>): void {
    const type = String(ev.type ?? '')
    switch (type) {
      case 'message_update':
        this.onMessageUpdate(ev)
        break
      case 'message_end':
        this.onMessageEnd(ev)
        break
      case 'tool_execution_start':
        this.onToolStart(ev)
        break
      case 'tool_execution_update':
        this.onToolUpdate(ev)
        break
      case 'tool_execution_end':
        this.onToolEnd(ev)
        break
      case 'turn_end':
        this.onTurnEnd(ev)
        break
      case 'agent_settled':
        this.onSettled(ev)
        break
      case 'queue_update':
        broadcast('chat:queue', {
          engine_kind: 'pi',
          steering: asStrings(ev.steering),
          followUp: asStrings(ev.followUp),
        })
        break
      case 'compaction_start':
        broadcast('chat:compaction', { state: 'start', engine_kind: 'pi' })
        break
      case 'compaction_end':
        broadcast('chat:compaction', { state: 'done', engine_kind: 'pi' })
        break
      case 'auto_retry_start':
        broadcast('chat:retry', {
          attempt: ev.attempt,
          max: ev.maxAttempts,
          delay_ms: ev.delayMs,
          engine_kind: 'pi',
        })
        break
      case 'auto_retry_end':
        broadcast('chat:retry_end', { engine_kind: 'pi' })
        break
      case 'extension_error':
        broadcast('chat:notify', {
          level: 'error',
          message: `扩展出错：${String(ev.error ?? '')}`,
          engine_kind: 'pi',
        })
        break
      case 'session_info_changed':
        broadcast('session:title', { title: ev.name ?? '', engine_kind: 'pi' })
        break
      default:
        break
    }
  }

  private onMessageUpdate(ev: Record<string, unknown>): void {
    const usage = ev.usage as PiUsageWire | undefined
    if (usage) this.usage = usage
    const inner = ev.assistantMessageEvent as Record<string, unknown> | undefined
    if (!inner) return
    const kind = String(inner.type ?? '')
    const delta = typeof inner.delta === 'string' ? inner.delta : ''
    const contentIndex = typeof inner.contentIndex === 'number' ? inner.contentIndex : this.blocks.length
    if (kind === 'text_delta' && delta) {
      this.appendDelta(contentIndex, 'text', delta)
    } else if (kind === 'thinking_delta' && delta) {
      this.appendDelta(contentIndex, 'thinking', delta)
    } else if (kind === 'toolcall_start') {
      const block: ChatBlockVO = {
        kind: 'tool',
        tool_call_id: String(inner.id ?? ''),
        tool_name: String(inner.toolName ?? ''),
        args: {},
        status: 'pending',
      }
      this.blocks[contentIndex] = block
      this.emitPatch('block', contentIndex, 'tool', undefined, block)
    } else if (kind === 'toolcall_end') {
      const call = inner.toolCall as ToolWire | undefined
      const block = this.blocks[contentIndex]
      if (block && call) {
        block.tool_call_id = String(call.id ?? block.tool_call_id)
        block.tool_name = String(call.name ?? block.tool_name)
        block.args = call.arguments ?? {}
        this.emitPatch('block', contentIndex, 'tool', undefined, block)
      }
    }
  }

  private appendDelta(index: number, kind: 'text' | 'thinking', delta: string): void {
    let block = this.blocks[index]
    if (!block || block.kind !== kind) {
      block = { kind, text: '' }
      this.blocks[index] = block
    }
    block.text = (block.text ?? '') + delta
    this.emitPatch('delta', index, kind, delta)
  }

  private onMessageEnd(ev: Record<string, unknown>): void {
    const msg = ev.message as Record<string, unknown> | undefined
    if (!msg) return
    const role = String(msg.role ?? '')
    if (role === 'user') {
      this.user_message = textOf(msg.content)
      return
    }
    if (role !== 'assistant') return
    // 权威快照：以 message_end 的完整内容重建块表。
    const content = Array.isArray(msg.content) ? (msg.content as Record<string, unknown>[]) : []
    const next: ChatBlockVO[] = []
    for (const b of content) {
      if (b.type === 'text' && typeof b.text === 'string') next.push({ kind: 'text', text: b.text })
      else if (b.type === 'thinking' && typeof b.thinking === 'string') next.push({ kind: 'thinking', text: b.thinking })
      else if (b.type === 'toolCall') {
        const prev = this.blocks.find((x) => x.tool_call_id === String(b.id ?? ''))
        next.push(
          prev ?? {
            kind: 'tool',
            tool_call_id: String(b.id ?? ''),
            tool_name: String(b.name ?? ''),
            args: (b.arguments as Record<string, unknown>) ?? {},
            status: 'pending',
          },
        )
      }
    }
    this.blocks = next
    broadcastRun('chat:blocks', this.run_id, { blocks: this.blocks })
    if (msg.stopReason === 'error') {
      broadcastRun('chat:error', this.run_id, {
        message: String(msg.errorMessage ?? '模型调用失败'),
      })
      this.streaming = false
    }
  }

  private onToolStart(ev: Record<string, unknown>): void {
    const block = this.byToolCall(String(ev.toolCallId ?? ''))
    if (!block) return
    block.status = 'running'
    if (ev.args) block.args = ev.args as Record<string, unknown>
    this.emitPatch('block', this.blocks.indexOf(block), 'tool', undefined, block)
    broadcastRun('chat:live', this.run_id, {
      phase: 'tool',
      tool_name: String(ev.toolName ?? block.tool_name),
      tool_call_id: ev.toolCallId,
      output_tokens: this.usage?.output ?? 0,
    })
  }

  private onToolUpdate(ev: Record<string, unknown>): void {
    const block = this.byToolCall(String(ev.toolCallId ?? ''))
    if (!block) return
    const partial = ev.partialResult as { content?: { text?: string }[] } | undefined
    const text = partial?.content?.map((c) => c.text ?? '').join('') ?? ''
    if (text) block.output = text
    this.emitPatch('block', this.blocks.indexOf(block), 'tool', undefined, block)
  }

  private onToolEnd(ev: Record<string, unknown>): void {
    const block = this.byToolCall(String(ev.toolCallId ?? ''))
    if (!block) return
    const result = ev.result as { content?: { text?: string }[]; details?: Record<string, unknown> } | undefined
    block.output = result?.content?.map((c) => c.text ?? '').join('') ?? ''
    block.is_error = ev.isError === true
    block.status = ev.isError === true ? 'error' : 'done'
    if (typeof ev.durationMs === 'number') block.duration_ms = ev.durationMs
    const diff = result?.details && typeof result.details.diff === 'string' ? result.details.diff : undefined
    if (diff) block.diff = parseDiffLines(diff)
    const file = extractFilePath(block.args, result?.details)
    if (file) block.file_path = file
    this.emitPatch('block', this.blocks.indexOf(block), 'tool', undefined, block)
  }

  private onTurnEnd(ev: Record<string, unknown>): void {
    const msg = ev.message as Record<string, unknown> | undefined
    if (msg) {
      const usage = msg.usage as PiUsageWire | undefined
      if (usage) this.usage = usage
    }
    broadcastRun('chat:turn', this.run_id, {
      user_message: this.user_message,
      blocks: this.blocks,
    })
    broadcastRun('chat:turn_end', this.run_id, {
      usage: {
        input: this.usage?.input ?? 0,
        output: this.usage?.output ?? 0,
        cacheRead: this.usage?.cacheRead ?? 0,
        cacheWrite: this.usage?.cacheWrite ?? 0,
        costUSD: this.usage?.cost?.total ?? 0,
      },
    })
  }

  private onSettled(ev: Record<string, unknown>): void {
    this.streaming = false
    broadcastRun('chat:settled', this.run_id, { aborted: ev.aborted === true })
    broadcastRun('chat:done', this.run_id, {})
  }

  /** UI 请求（notify/status/widget 等）不经块表，直接广播。 */
  onUiRequest(record: Record<string, unknown>): void {
    const method = String(record.method ?? '')
    switch (method) {
      case 'notify':
        broadcast('chat:notify', {
          level: String(record.notifyType ?? 'info'),
          message: String(record.message ?? ''),
          engine_kind: 'pi',
        })
        break
      case 'setStatus':
        broadcast('chat:status', {
          key: String(record.statusKey ?? ''),
          text: record.statusText != null ? String(record.statusText) : '',
          engine_kind: 'pi',
        })
        break
      case 'setWidget':
        broadcast('chat:widget', {
          key: String(record.widgetKey ?? ''),
          lines: Array.isArray(record.widgetLines) ? record.widgetLines.map(String) : [],
          engine_kind: 'pi',
        })
        break
      case 'setTitle':
        broadcast('session:title', { title: String(record.title ?? ''), engine_kind: 'pi' })
        break
      case 'set_editor_text':
        broadcast('chat:editor_text', { text: String(record.text ?? ''), engine_kind: 'pi' })
        break
      default:
        break
    }
  }

  private byToolCall(id: string): ChatBlockVO | undefined {
    if (!id) return undefined
    return this.blocks.find((b) => b.kind === 'tool' && b.tool_call_id === id)
  }

  private emitPatch(
    mode: 'delta' | 'block',
    index: number,
    kind: ChatBlockVO['kind'],
    delta?: string,
    block?: ChatBlockVO,
  ): void {
    broadcastRun('chat:patch', this.run_id, {
      mode,
      index,
      kind,
      ...(delta !== undefined ? { delta } : {}),
      ...(block ? { block } : {}),
    })
    if (kind === 'text') {
      broadcastRun('chat:live', this.run_id, {
        phase: 'text',
        output_tokens: this.usage?.output ?? 0,
      })
    } else if (kind === 'thinking') {
      broadcastRun('chat:live', this.run_id, {
        phase: 'thinking',
        output_tokens: this.usage?.output ?? 0,
      })
    }
  }
}

function textOf(content: unknown): string {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return ''
  return (content as Record<string, unknown>[])
    .filter((b) => b.type === 'text' && typeof b.text === 'string')
    .map((b) => b.text as string)
    .join('\n')
}

function asStrings(v: unknown): string[] {
  return Array.isArray(v) ? v.map(String) : []
}

function extractFilePath(
  args?: Record<string, unknown>,
  details?: Record<string, unknown>,
): string | undefined {
  for (const source of [args, details]) {
    for (const key of ['file_path', 'filePath', 'path', 'notebook_path']) {
      const v = source?.[key]
      if (typeof v === 'string' && v) return v
    }
  }
  return undefined
}
