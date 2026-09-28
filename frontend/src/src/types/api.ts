// 与后端 RESP 的 json tag 一一对应；字段名一律 snake_case。

export interface Resp<T> {
  code: number
  message: string
  data: T
}

export interface UsageVO {
  input: number
  output: number
  total: number
  latency_ms: number
}

export interface ToolCall {
  id: string
  name: string
  label?: string
  args?: Record<string, unknown>
}

export interface MessageVO {
  id: string
  role: 'user' | 'assistant' | 'tool'
  type: string
  thinking?: string
  content?: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  tool_name?: string
  is_error?: boolean
  stop_reason?: string
  usage?: UsageVO
  created_at: number
  latency_ms?: number
}

export interface SessionVO {
  id: string
  title: string
  workspace: string
  provider_id: string
  model: string
  permission: string
  leaf_entry_id: string
  message_count: number
  total_tokens: number
  created_at: number
  updated_at: number
}

export interface SessionDetailVO {
  session: SessionVO
  messages: MessageVO[]
}

export interface SendMessageRESP {
  run_id: string
  entry_id: string
  session_id: string
}

/** @ 引用：只传工作目录内的相对路径，助手读得到才会用 */
export interface AttachmentREQ {
  path: string
  name: string
}

export interface ApprovalVO {
  id: string
  session_id: string
  tool_call_id: string
  tool: string
  label: string
  args: Record<string, unknown>
  risk: string
  reason: string
  status: string
  created_at: number
  decided_at: number
}

export interface ProviderVO {
  id: string
  name: string
  api: string
  base_url: string
  has_key: boolean
  models: string[]
  is_default: boolean
  enabled: boolean
  created_at: number
}

export interface SkillVO {
  id: string
  name: string
  description: string
  location: string
  source: string
  enabled: boolean
}

export interface KnowledgeDocVO {
  id: string
  path: string
  title: string
  ext: string
  size: number
  chunks: number
  status: string
  error?: string
  created_at: number
}

export interface SearchHitVO {
  doc_id: string
  title: string
  path: string
  seq: number
  content: string
  score: number
}

export interface BootstrapVO {
  version: string
  contract_version: number
  default_provider_id: string
  default_model: string
  workspace: string
  permission: string
  python_ready: boolean
  settings: Record<string, string>
}

// SSE 事件载荷
export interface StartData { run_id: string; session_id: string }
export interface DeltaData { entry_id: string; kind: string; delta: string }
export interface ToolStartData { tool_call_id: string; tool: string; label: string; args: Record<string, unknown> }
export interface ToolEndData { tool_call_id: string; ok: boolean; title: string; output: string; duration_ms: number }
export interface ApprovalData {
  approval_id: string
  tool_call_id: string
  tool: string
  label: string
  args: Record<string, unknown>
  risk: string
  reason: string
}
export interface CompressedData { tokens_before: number; tokens_after: number }
export interface DoneData { entry_id: string; stop_reason: string; usage?: UsageVO }
export interface ErrorData { code: number; message: string }
export interface GapData { reason: string }

/** 上下文水位：ratio 为 0-100 的整数，窗口认不出来时是 0 */
export interface ContextData {
  used: number
  window: number
  ratio: number
  kept?: number
}

export interface Envelope<T = unknown> {
  seq: number
  event: string
  data: T
}

/** SSE 事件的判别联合：切换 event 就能把 data 收窄到对应载荷，调用方无需强转。 */
export type ServerEvent =
  | (Envelope<StartData> & { event: 'chat:start' })
  | (Envelope<DeltaData> & { event: 'chat:delta' })
  | (Envelope<ToolStartData> & { event: 'chat:tool_start' })
  | (Envelope<ToolEndData> & { event: 'chat:tool_end' })
  | (Envelope<ApprovalData> & { event: 'chat:approval' })
  | (Envelope<CompressedData> & { event: 'chat:compressed' })
  | (Envelope<ContextData> & { event: 'chat:context' })
  | (Envelope<DoneData> & { event: 'chat:done' })
  | (Envelope<ErrorData> & { event: 'chat:error' })
  | (Envelope<GapData> & { event: 'chat:gap' })

// ---- /stats ----

export interface StatsTotals {
  input: number
  output: number
  total: number
  calls: number
  sessions: number
  latency_ms: number
  avg_latency_ms: number
}

/** 长度恒等于 days，缺的日子后端已补零 */
export interface StatsDailyItem {
  date: string
  input: number
  output: number
  total: number
}

export interface StatsModelItem {
  model: string
  total: number
  calls: number
}

export interface StatsSessionItem {
  session_id: string
  title: string
  total: number
}

export interface StatsRESP {
  days: number
  totals: StatsTotals
  daily: StatsDailyItem[]
  models: StatsModelItem[]
  sessions: StatsSessionItem[]
}
