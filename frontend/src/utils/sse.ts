// SSE 载荷的类型边界：把 JSON 解析出来的 unknown 树窄化成界面用的类型。
// 全是纯函数、不碰 store；纪律是先校验再取字段，绝不 `as` 一个形状直接读成员——
// 后端加删字段都会在这里被挡住，否则界面读到 undefined 还不知道发生了什么。
import type { AppProgress, AskRequest, ChatBlock, ChatPatch, DiffLine, SlashCommand, TodoItem } from '@/types/api'

/** 一个长任务的一次现场：动作、逐行输出、当前阶段。 */
export interface AppProgressLog {
  action: string
  phase: string
  lines: string[]
  message: string
}

/** 一帧 SSE：信封 + 载荷。 */
export interface StreamFrame {
  seq: number
  event: string
  data: Record<string, unknown>
}

/** 一轮对话的用量（后端已归一成 snake_case）。 */
export interface TurnUsagePayload {
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  cost_usd?: number
}

/** 取一个字符串字段；不是字符串就给兜底值。 */
export const asString = (v: unknown, fallback = ''): string => (typeof v === 'string' ? v : fallback)

/** 取一个数字字段；不是有限数就给兜底值。 */
export const asNumber = (v: unknown, fallback = 0): number =>
  typeof v === 'number' && Number.isFinite(v) ? v : fallback

/** 取一个布尔字段。 */
export const asBool = (v: unknown, fallback = false): boolean => (typeof v === 'boolean' ? v : fallback)

/** 取一个字符串数组；不是数组就给空数组。 */
export const readStrings = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []

// 窄化一帧安装进度：阶段必须是已知值，其余丢弃而不是兜底——不认识的值
// 会让进度面板显示成一堆看不懂的日志。
export function readAppProgress(v: Record<string, unknown>): AppProgress | null {
  const kind = asString(v.progress_kind)
  if (!kind) return null
  const action = asString(v.progress_action)
  if (!action) return null
  const phase = asString(v.progress_phase)
  if (phase !== 'start' && phase !== 'line' && phase !== 'done' && phase !== 'fail') return null
  return {
    progress_kind: kind,
    progress_action: action,
    progress_phase: phase,
    progress_line: asString(v.progress_line) || undefined,
    progress_message: asString(v.progress_message) || undefined,
    plugin_name: asString(v.plugin_name) || undefined,
  }
}

/** 命令清单只关心界面要显示的三段：名字 / 来源 / 说明。 */
export function readCommands(v: unknown): SlashCommand[] {
  if (!Array.isArray(v)) return []
  const out: SlashCommand[] = []
  for (const item of v) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const name = asString(record.command_name)
    if (!name) continue
    out.push({
      command_name: name,
      command_source: asString(record.command_source),
      command_desc: asString(record.command_desc) || undefined,
    })
  }
  return out
}

/** 窄化一行差异。 */
function readDiffLine(v: unknown): DiffLine | null {
  if (!v || typeof v !== 'object') return null
  const record = v as Record<string, unknown>
  const kind = asString(record.line_kind)
  if (kind !== 'file' && kind !== 'hunk' && kind !== 'add' && kind !== 'del' && kind !== 'ctx') return null
  return {
    line_kind: kind,
    old_no: asNumber(record.old_no),
    new_no: asNumber(record.new_no),
    line_text: asString(record.line_text),
  }
}

/** 窄化一组差异行。 */
export function readDiffLines(v: unknown): DiffLine[] {
  if (!Array.isArray(v)) return []
  const out: DiffLine[] = []
  for (const item of v) {
    const line = readDiffLine(item)
    if (line) out.push(line)
  }
  return out
}

/** 窄化一个内容块。种类必须是三种之一，其余字段各按自己的类型收。 */
export function readBlock(v: unknown): ChatBlock | null {
  if (!v || typeof v !== 'object') return null
  const record = v as Record<string, unknown>
  const kind = asString(record.block_kind)
  if (kind !== 'text' && kind !== 'thinking' && kind !== 'tool') return null
  const status = asString(record.tool_status)
  return {
    block_kind: kind,
    block_text: asString(record.block_text),
    tool_call_id: asString(record.tool_call_id) || undefined,
    tool_name: asString(record.tool_name) || undefined,
    tool_args: record.tool_args,
    tool_output: asString(record.tool_output) || undefined,
    tool_is_error: asBool(record.tool_is_error),
    tool_status:
      status === 'pending' || status === 'running' || status === 'done' || status === 'error' ? status : undefined,
    duration_ms: asNumber(record.duration_ms) || undefined,
    file_path: asString(record.file_path) || undefined,
    diff_lines: record.diff_lines ? readDiffLines(record.diff_lines) : undefined,
  }
}

/** 窄化一组内容块；坏块丢掉，其余照收。 */
export function readBlocks(v: unknown): ChatBlock[] {
  if (!Array.isArray(v)) return []
  const out: ChatBlock[] = []
  for (const item of v) {
    const block = readBlock(item)
    if (block) out.push(block)
  }
  return out
}

/** 窄化一帧增量补丁。mode 必须是两种之一。 */
export function readPatch(data: Record<string, unknown>): ChatPatch | null {
  const mode = asString(data.mode)
  if (mode !== 'delta' && mode !== 'block') return null
  const kind = asString(data.block_kind)
  return {
    run_id: asString(data.run_id),
    mode,
    index: asNumber(data.index),
    kind: kind === 'text' || kind === 'thinking' || kind === 'tool' ? kind : 'text',
    delta: asString(data.delta) || undefined,
    block: readBlock(data.block) ?? undefined,
  }
}

// 窄化一帧提问请求：四种形态（confirm / select / input / editor）共用一张卡，
// 这里只收公共字段。
export function readAsk(v: Record<string, unknown>): AskRequest | null {
  const id = asString(v.ask_id)
  const method = asString(v.ask_method)
  if (!id || !method) return null
  return {
    ask_id: id,
    ask_method: method as AskRequest['ask_method'],
    ask_title: asString(v.ask_title) || undefined,
    ask_message: asString(v.ask_message) || undefined,
    ask_placeholder: asString(v.ask_placeholder) || undefined,
    ask_prefill: asString(v.ask_prefill) || undefined,
    ask_options: readStrings(v.ask_options),
    ask_timeout_ms: asNumber(v.ask_timeout_ms) || undefined,
  }
}

// 窄化一组待办任务：状态必须是三种之一——模型偶尔给别的词，交到面板会
// 渲染成既不是待办也不是完成的样子。
export function readTodos(v: unknown): TodoItem[] {
  if (!Array.isArray(v)) return []
  const out: TodoItem[] = []
  for (const item of v) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const subject = asString(record.todo_subject)
    const status = asString(record.todo_status)
    if (!subject) continue
    if (status !== 'pending' && status !== 'in_progress' && status !== 'completed') continue
    out.push({
      todo_id: asNumber(record.todo_id),
      todo_subject: subject,
      todo_status: status,
      todo_active_form: asString(record.todo_active_form) || undefined,
    })
  }
  return out
}

// 读一帧 SSE：信封本身也可能坏，坏帧直接丢不断流——进程被杀时最后半行
// JSON 这种坏帧在真实环境里是常态。
export function readFrame(raw: string): StreamFrame | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null
  const record = parsed as Record<string, unknown>
  const data = record.data && typeof record.data === 'object' ? (record.data as Record<string, unknown>) : {}
  return { seq: asNumber(record.seq), event: asString(record.event), data }
}
