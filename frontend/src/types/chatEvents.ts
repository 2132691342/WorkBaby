// SSE 事件载荷：与后端 emit 的 JSON 逐字一致。
// 载荷形状只描述「事件说了什么」，不含界面状态，因此与 store 分开放。
import type { MessageVO } from './api'

/** chat:start */
export interface StartData {
  run_id: string
  session_id: string
}

/** chat:delta：增量类型 text / thinking */
export interface DeltaData {
  kind: 'text' | 'thinking'
  delta: string
}

/** chat:tool_start */
export interface ToolStartData {
  tool_call_id: string
  /** 工具名 */
  name: string
  /** 显示名；可为 null，前端按 label || name 取 */
  label: string | null
  args?: Record<string, unknown> | null
}

/** chat:tool_end */
export interface ToolEndData {
  tool_call_id: string
  name: string
  /** 结果文本（后端已截断） */
  result: string
  is_error: boolean
  duration_ms: number
  /** 这次调用改动的文件（相对工作区）；不产生改动的工具不带这个字段 */
  file_path?: string
  /** 差异行；后端带了 file_path 才可能有 */
  diff?: DiffLine[]
}

/** chat:context 里的用量读数（载荷内不带业务前缀） */
export interface ContextUsage {
  input: number
  output: number
  cached: number
  cache_write: number
  total: number
  /** 这一轮发出时的上下文占用 */
  context: number
  cost: number
}

/** chat:context */
export interface ContextData {
  usage: ContextUsage
  context_window: number
  /** 占用比（0-1） */
  ratio: number
}

/** chat:compressed */
export interface CompressedData {
  tokens_before: number
}

/** chat:done；message 目前恒为 null，定稿消息以快照回放为准 */
export interface DoneData {
  run_id: string
  message?: MessageVO | null
}

/** chat:error */
export interface ErrorData {
  run_id: string
  code: string
  message: string
}

/** chat:retry */
export interface RetryData {
  attempt: number
  max: number
  delay_ms: number
}

/** chat:notify */
export interface NotifyData {
  /** info / warning / error */
  level: string
  text: string
}

/** chat:status：空 text 即清除该 key */
export interface StatusData {
  key: string
  text: string
}

/** chat:widget：空 lines 即清除该 key */
export interface WidgetData {
  key: string
  lines: string[]
}

/** 一段 diff 行：工具改了什么。 */
export interface DiffLine {
  /** 行类型：add / del / ctx */
  line_kind: 'add' | 'del' | 'ctx'
  /** 行文本 */
  line_text: string
}

// 当前正在跑的工具调用：流式期间只做「展示」，chat:done 后用权威快照覆盖。
export interface ToolRun {
  tool_call_id: string
  tool: string
  label: string
  args: Record<string, unknown>
  running: boolean
  ok: boolean
  title: string
  output: string
  duration_ms: number
  /** 这次调用改动的文件（相对工作区），没有就是空串 */
  file_path: string
  /** 这次调用带回来的差异行 */
  diff: DiffLine[]
}

// 本轮的一条有序段：思考 / 正文 / 工具。顺序就是内核吐出来的顺序，
// 不按类型分组——分组会把工具卡全部顶到正文前面。
export type LivePart =
  | { key: string; kind: 'thinking'; text: string }
  | { key: string; kind: 'text'; text: string }
  | { key: string; kind: 'tool'; toolCallId: string }

// 引擎抛回来的一条提问：confirm / select / input / editor。
export interface PendingAsk {
  id: string
  method: string
  title: string
  message: string
  options: string[]
  /** 输入类提问的预置提示（多选题的「1,3; 自定义答案」就靠它）*/
  placeholder: string
  /** editor 的预填内容 */
  prefill: string
  /** 等待上限（毫秒）：0 表示不限时 */
  timeoutMs: number
}
