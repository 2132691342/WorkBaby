// 前端与后端的契约类型（会话 / 对话）。字段名与 Go 侧 json tag 一一对应。
// 命名纪律：字段一律带业务前缀（session_title 而不是 title）——模板里一眼看出读的是哪个对象。

import type { EngineCaps, EngineKind } from './api-core'
import type { ModelInfo } from './api-data'

// ---------------- 会话 ----------------

export interface SessionRow {
  session_id: string
  session_path: string
  session_title: string
  session_cwd: string
  session_model: string
  updated_at: number
  size_bytes: number
  message_count: number
  total_tokens: number
  cost_usd: number
}

export interface SessionItem {
  item_kind: 'user' | 'assistant' | 'tool'
  item_role: string
  item_text?: string
  item_model?: string
  tool_name?: string
  tool_call_id?: string
  tool_is_error?: boolean
  created_at: number
  item_tokens?: number
  cost_usd?: number
}

export interface SessionDetail {
  session: SessionRow
  items: SessionItem[]
}

export interface SessionList {
  sessions: SessionRow[]
}

/** 会话 HTML 导出的出参：自包含单文件，前端 Blob 下载。 */
export interface SessionExport {
  file_name: string
  file_content: string
}

// ---------------- 对话 ----------------

export type BlockKind = 'text' | 'thinking' | 'tool'

/** 差异行。工具结果与 git 归一成同一种形状。 */
export interface DiffLine {
  line_kind: 'file' | 'hunk' | 'add' | 'del' | 'ctx'
  old_no: number
  new_no: number
  line_text: string
}

export interface ChatBlock {
  block_kind: BlockKind
  block_text?: string
  tool_call_id?: string
  tool_name?: string
  tool_args?: unknown
  tool_output?: string
  tool_status?: 'pending' | 'running' | 'done' | 'error'
  tool_is_error?: boolean
  duration_ms?: number
  file_path?: string
  diff_lines?: DiffLine[]
}

/** 一条回复的用量读数。 */
export interface TurnUsage {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cost_usd: number
}

export interface HistoryTurn {
  /** engine-mark 是界面层的分隔标记，不是引擎消息 */
  turn_role: 'user' | 'assistant' | 'tool' | 'engine-mark'
  turn_blocks: ChatBlock[]
  turn_model?: string
  turn_usage?: TurnUsage
  /** 这一轮的条目 ID：「删除一轮」时回传给后端 */
  turn_id?: string
}

export interface ContextUsage {
  usage_tokens: number | null
  context_window: number
  usage_percent: number | null
}

export interface SessionStats {
  session_id: string
  user_messages: number
  assistant_messages: number
  tool_calls: number
  total_messages: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  total_tokens: number
  cost_usd: number
  context_usage?: ContextUsage
}

export interface QueuedMessages {
  steering: string[]
  follow_up: string[]
}

// 一个附件：file 只把路径拼进 prompt（引擎自己用读工具看）；
// image 走 prompt 的 images 字段（唯一的二进制通道）。
export interface ChatAttachment {
  attachment_kind: 'file' | 'image' | 'session'
  file_path?: string
  file_name?: string
  image_base64?: string
  image_mime?: string
  attachment_body?: string
}

export interface SlashCommand {
  command_name: string
  command_source: string
  command_desc?: string
  command_body?: string
}

export interface ChatState {
  engine_kind: EngineKind
  session_file: string
  work_dir: string
  engine_running: boolean
  streaming: boolean
  model?: ModelInfo
  thinking_level: string
  thinking_levels: string[]
  context_usage?: ContextUsage
  auto_compaction: boolean
  queued: QueuedMessages
  stats?: SessionStats
  history: HistoryTurn[]
  todos?: TodoItem[]
  /** 是否处于计划模式：由会话文件里的 plan-mode 开关条目回放而来 */
  plan_mode: boolean
  commands: SlashCommand[]
  caps: EngineCaps
  /** 没能把引擎切到请求的会话时的原因原话；空表示切好了 */
  bind_err?: string
}

/** 待办面板上的一条任务（由引擎 todo 工具调用派生）。 */
export interface TodoItem {
  todo_id: number
  todo_subject: string
  todo_status: 'pending' | 'in_progress' | 'completed'
  todo_active_form?: string
}

/** 引擎的提问卡片。 */
export interface AskRequest {
  ask_id: string
  ask_method: 'confirm' | 'select' | 'input' | 'editor'
  ask_title?: string
  ask_message?: string
  ask_placeholder?: string
  ask_prefill?: string
  ask_options?: string[]
  ask_timeout_ms?: number
}

export interface AnswerItem {
  question_id: string
  selected_options: string[]
  custom_input?: string
}

export interface UIResponse {
  ask_id: string
  value?: string
  confirmed?: boolean
  cancelled?: boolean
  answers?: AnswerItem[]
}

/** 增量补丁帧（SSE `chat:patch`）。 */
export interface ChatPatch {
  run_id: string
  mode: 'delta' | 'block'
  index: number
  kind: BlockKind
  delta?: string
  block?: ChatBlock
}
