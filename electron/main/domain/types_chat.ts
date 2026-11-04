//! 聊天域类型：实时块、历史轮、提问板与聊天状态；与前端 types/api.ts 对齐。
import type { WbCaps } from './types_core'

/** 块种类。 */
export type BlockKind = 'text' | 'thinking' | 'tool'

/** 差异行。 */
export interface DiffLineVO {
  line_kind: 'file' | 'hunk' | 'add' | 'del' | 'ctx'
  old_no: number
  new_no: number
  line_text: string
}

/** 聊天内容块：流式与历史共用。 */
export interface ChatBlockVO {
  kind: BlockKind
  text?: string
  tool_call_id?: string
  tool_name?: string
  args?: Record<string, unknown>
  intent?: string
  output?: string
  is_error?: boolean
  status?: 'pending' | 'running' | 'done' | 'error'
  streaming?: boolean
  duration_ms?: number
  file_path?: string
  diff?: DiffLineVO[]
}

/** 历史轮用量。 */
export interface TurnUsageVO {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
}

/** 历史轮：会话回放的唯一来源。 */
export interface HistoryTurnVO {
  role: 'user' | 'assistant' | 'tool' | 'engine-mark'
  blocks: ChatBlockVO[]
  model?: string
  usage?: TurnUsageVO
  hid?: string
}

/** 模型信息。 */
export interface ModelInfoVO {
  id: string
  name: string
  provider: string
  api: string
  reasoning?: boolean
  contextWindow?: number
  maxTokens?: number
  input?: string[]
  cost?: { input: number; output: number; cacheRead: number; cacheWrite: number }
}

/** 上下文用量。 */
export interface ContextUsageVO {
  tokens: number | null
  contextWindow: number
  percent: number | null
}

/** 会话统计。 */
export interface SessionStatsVO {
  sessionId: string
  userMessages: number
  assistantMessages: number
  toolCalls: number
  totalMessages: number
  tokens: { input: number; output: number; cacheRead: number; cacheWrite: number; total: number }
  cost: number
  contextUsage?: ContextUsageVO
}

/** 排队消息。 */
export interface QueuedMessagesVO {
  steering: string[]
  followUp: string[]
  liveSteered: number
}

/** 斜杠命令。 */
export interface SlashCommandVO {
  name: string
  source: string
  description?: string
  aliases?: string[]
  input?: { hint?: string }
  subcommands?: { name: string; description?: string; usage?: string }[]
}

/** 聊天状态：绑定哪个引擎的哪个会话。 */
export interface ChatStateVO {
  kind: 'omp' | 'pi'
  session: string
  session_file: string
  work_dir: string
  engine_running: boolean
  streaming: boolean
  model?: ModelInfoVO
  thinking_level: string
  levels: string[]
  context?: ContextUsageVO
  tokens_per_second?: number
  auto_compaction: boolean
  queued?: QueuedMessagesVO
  stats?: SessionStatsVO
  messages: unknown[]
  history: HistoryTurnVO[]
  commands: SlashCommandVO[]
  profile: string
  caps: WbCaps
  bind_err?: string
}

/** 内核提问请求。 */
export interface AskRequestVO {
  id: string
  method: 'confirm' | 'select' | 'input' | 'editor' | 'ask' | 'cancel'
  title?: string
  message?: string
  placeholder?: string
  prefill?: string
  options?: string[]
  optionDetails?: { description?: string }[]
  timeout?: number
  targetId?: string
  questions?: {
    id: string
    question: string
    header?: string
    multi?: boolean
    recommended?: string
    options: { label: string; description?: string; preview?: string }[]
  }[]
}

/** 用户对提问的应答。 */
export interface UIResponseVO {
  type?: string
  id: string
  value?: string
  confirmed?: boolean
  cancelled?: boolean
  timedOut?: boolean
  answers?: { id: string; selectedOptions: string[]; customInput?: string }[]
}

/** 聊天附件。 */
export interface ChatAttachmentVO {
  kind: 'file' | 'image'
  file_path?: string
  file_name?: string
  image_base64?: string
  image_mime?: string
}

/** pi 事件翻译出的实时帧负载。 */
export interface ChatPatchVO {
  run_id: string
  mode: 'delta' | 'block' | 'blocks'
  index: number
  kind: BlockKind
  delta?: string
  block?: ChatBlockVO
  blocks?: ChatBlockVO[]
}

/** 从 pi 的 Model 对象提取模型视图；字段缺失时逐层兜底。 */
export function toModelInfo(raw: unknown): ModelInfoVO | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const m = raw as Record<string, unknown>
  const id = str(m.id) ?? str(m.modelId) ?? ''
  if (!id) return undefined
  const cost = m.cost as Record<string, number> | undefined
  return {
    id,
    name: str(m.name) ?? id,
    provider: str(m.provider) ?? '',
    api: str(m.api) ?? '',
    reasoning: m.reasoning === true,
    contextWindow: num(m.contextWindow) ?? num(m.context_window),
    maxTokens: num(m.maxTokens) ?? num(m.max_tokens),
    input: Array.isArray(m.input) ? (m.input as string[]) : undefined,
    cost: cost
      ? {
          input: cost.input ?? 0,
          output: cost.output ?? 0,
          cacheRead: cost.cacheRead ?? 0,
          cacheWrite: cost.cacheWrite ?? 0,
        }
      : undefined,
  }
}

function str(v: unknown): string | undefined {
  return typeof v === 'string' ? v : undefined
}

function num(v: unknown): number | undefined {
  return typeof v === 'number' ? v : undefined
}
