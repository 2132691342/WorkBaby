// SSE 载荷的类型边界：把 JSON 解析出来的 unknown 树窄化成界面用的类型。
// 全是纯函数、不碰 store；纪律是先校验再取字段，绝不 `as` 一个形状直接读成员——
// 后端加删字段都会在这里被挡住，否则界面读到 undefined 还不知道发生了什么。
import type {
  AppProgress,
  AskRequest,
  ChatBlock,
  ChatPatch,
  SlashCommand,
} from '@/types/api'

/** 一个应用的一次长任务现场：命令、逐行输出、当前阶段。 */
export interface AppProgressLog {
  action: string
  phase: string
  command: string
  lines: string[]
  message: string
}

/** 一帧 SSE：信封 + 载荷。 */
export interface StreamFrame {
  seq: number
  event: string
  data: Record<string, unknown>
}

/**
 * 一轮对话的用量。
 *
 * 字段名与引擎报上来的一致（驼峰），界面层再转成自己的形状。
 */
export interface TurnUsagePayload {
  input?: number
  output?: number
  cacheRead?: number
  cacheWrite?: number
}

/** 取一个字符串字段；不是字符串就给兜底值。 */
export const asString = (v: unknown, fallback = ''): string =>
  typeof v === 'string' ? v : fallback

/** 取一个数字字段；不是有限数就给兜底值。 */
export const asNumber = (v: unknown, fallback = 0): number =>
  typeof v === 'number' && Number.isFinite(v) ? v : fallback

/** 取一个字符串数组；不是数组就给空数组。 */
export const readStrings = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []

/**
 * 窄化一帧安装进度。action 与 phase 都必须是已知值，其余一律丢弃。
 *
 * 丢弃而不是兜底：界面只认「装 / 升级 / 卸载」与四个阶段，
 * 一个不认识的值交下去会让进度面板显示成一堆看不懂的日志。
 */
export function readAppProgress(v: Record<string, unknown>): AppProgress | null {
  const kind = asString(v.kind)
  if (kind !== 'omp' && kind !== 'pi') return null
  const action = asString(v.action)
  if (action !== 'install' && action !== 'update' && action !== 'uninstall') return null
  const phase = asString(v.phase)
  if (phase !== 'start' && phase !== 'line' && phase !== 'done' && phase !== 'fail') return null
  return {
    kind,
    action,
    phase,
    line: asString(v.line) || undefined,
    command: asString(v.command) || undefined,
    message: asString(v.message) || undefined,
  }
}

/** 命令清单只关心界面要显示的三段：名字 / 说明 / 输入提示。 */
export function readCommands(v: unknown): SlashCommand[] {
  if (!Array.isArray(v)) return []
  const out: SlashCommand[] = []
  for (const item of v) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const name = asString(record.name)
    if (!name) continue
    const input = record.input && typeof record.input === 'object' ? (record.input as Record<string, unknown>) : null
    out.push({
      name,
      source: asString(record.source),
      description: asString(record.description) || undefined,
      aliases: readStrings(record.aliases),
      input: input ? { hint: asString(input.hint) || undefined } : undefined,
    })
  }
  return out
}

/** 窄化一个内容块。kind 必须是三种之一，其余字段各按自己的类型收。 */
export function readBlock(v: unknown): ChatBlock | null {
  if (!v || typeof v !== 'object') return null
  const record = v as Record<string, unknown>
  const kind = asString(record.kind)
  if (kind !== 'text' && kind !== 'thinking' && kind !== 'tool') return null
  const status = asString(record.status)
  return {
    kind,
    text: asString(record.text),
    tool_call_id: asString(record.tool_call_id) || undefined,
    tool_name: asString(record.tool_name) || undefined,
    args: record.args,
    intent: asString(record.intent) || undefined,
    output: asString(record.output) || undefined,
    is_error: Boolean(record.is_error),
    status: status === 'pending' || status === 'running' || status === 'done' || status === 'error' ? status : undefined,
    streaming: Boolean(record.streaming),
    duration_ms: asNumber(record.duration_ms) || undefined,
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

/** 窄化一帧增量补丁。mode 必须是三种之一。 */
export function readPatch(data: Record<string, unknown>): ChatPatch | null {
  const mode = asString(data.mode)
  if (mode !== 'delta' && mode !== 'block' && mode !== 'blocks') return null
  const kind = asString(data.kind)
  return {
    run_id: asString(data.run_id),
    mode,
    index: asNumber(data.index),
    kind: kind === 'text' || kind === 'thinking' || kind === 'tool' ? kind : 'text',
    delta: asString(data.delta) || undefined,
    block: readBlock(data.block) ?? undefined,
    blocks: mode === 'blocks' ? readBlocks(data.blocks) : undefined,
  }
}

/**
 * 窄化一帧提问请求。
 *
 * 提问有四种形态（confirm / select / input / editor），它们共用一张卡，
 * 所以这里只收公共字段与一个 questions 数组（多问一��）。
 */
export function readAsk(v: unknown): AskRequest | null {
  if (!v || typeof v !== 'object') return null
  const record = v as Record<string, unknown>
  const id = asString(record.id)
  const method = asString(record.method)
  if (!id || !method) return null
  const questions = Array.isArray(record.questions) ? record.questions : []
  return {
    id,
    method: method as AskRequest['method'],
    title: asString(record.title) || undefined,
    message: asString(record.message) || undefined,
    placeholder: asString(record.placeholder) || undefined,
    prefill: asString(record.prefill) || undefined,
    options: readStrings(record.options),
    timeout: asNumber(record.timeout) || undefined,
    targetId: asString(record.targetId) || undefined,
    questions: questions.flatMap((q) => {
      if (!q || typeof q !== 'object') return []
      const item = q as Record<string, unknown>
      const options = Array.isArray(item.options) ? item.options : []
      return [
        {
          id: asString(item.id),
          question: asString(item.question),
          header: asString(item.header) || undefined,
          multi: Boolean(item.multi),
          recommended: typeof item.recommended === 'number' ? item.recommended : undefined,
          options: options.flatMap((o) => {
            if (!o || typeof o !== 'object') return []
            const opt = o as Record<string, unknown>
            return [
              {
                label: asString(opt.label),
                description: asString(opt.description) || undefined,
                preview: asString(opt.preview) || undefined,
              },
            ]
          }),
        },
      ]
    }),
  }
}

/**
 * 读一帧 SSE：信封本身也可能是坏的，坏帧直接丢，不断流。
 *
 * 丢而不是抛：一条坏帧不该让整条事件流断掉——
 * 而「一帧坏帧」在真实环境里是常态（进程被杀时最后半行 JSON）。
 */
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
