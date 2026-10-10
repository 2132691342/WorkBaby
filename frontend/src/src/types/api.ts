// 与后端 RESP 的 json tag 一一对应；字段名一律 snake_case。

export interface Resp<T> {
  code: number
  message: string
  data: T
}

export interface UsageVO {
  input: number
  output: number
  /** 命中上游缓存的输入量，命中率 = cached / input */
  cached: number
  total: number
  /** 这一轮发出时上下文占用的窗口量 */
  context: number
  latency_ms: number
}

export interface ToolCall {
  id: string
  name: string
  label?: string
  args?: Record<string, unknown>
}

export interface MessageImage {
  mime: string
  base64: string
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
  images?: MessageImage[]
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

/** @ 引用：只传工作目录内的相对路径，助手读得到才会用。
 *  粘贴的图片没有路径：image_base64 直接带数据（png/jpg/webp/gif）。 */
export interface AttachmentREQ {
  path: string
  name: string
  image_base64?: string
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

export interface FileEntryVO {
  name: string
  dir: boolean
}

/** 工作区目录列举：path 是相对工作区的目录，空串即根目录 */
export interface FileListVO {
  path: string
  entries: FileEntryVO[]
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
  /** 本次启动被按拒绝收口的上次残留确认；空数组表示没有 */
  expired_approvals?: ApprovalVO[]
}

/** 知识库重建任务的进度快照 */
export interface ReindexJobVO {
  running: boolean
  total: number
  done: number
  failed: number
  started_at: number
  finished_at: number
}

/** 自定义背景 */
export interface BackgroundVO {
  image: string
  opacity: number
  blur: number
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
/** 插话 / 排队消息已注入上下文并落库，界面据此把它补进时间线 */
export interface UserData { entry_id: string; content: string }
export interface DoneData {
  entry_id: string
  stop_reason: string
  usage?: UsageVO
  /** 本轮实际下发的输出预算；撞 length 时用它说清「撞的是哪个上限」 */
  max_tokens?: number
}
export interface ErrorData { code: number; message: string }
export interface GapData { reason: string }
export interface StoppedData { reason: string }

export interface Envelope<T = unknown> {
  seq: number
  event: string
  data: T
}

/** 上下文水位：ratio 为 0-100 的整数，分母是可用预算（窗口 - reserve），
 * 与助手开始整理较早内容的位置一致；known=false 表示窗口是估算值，读数加「约」前缀显示 */
export interface ContextData {
  used: number
  window: number
  ratio: number
  known: boolean
  reserve?: number
}

/** SSE 事件的判别联合：切换 event 就能把 data 收窄到对应载荷，调用方无需强转。 */
export type ServerEvent =
  | (Envelope<StartData> & { event: 'chat:start' })
  | (Envelope<DeltaData> & { event: 'chat:delta' })
  | (Envelope<ToolStartData> & { event: 'chat:tool_start' })
  | (Envelope<ToolEndData> & { event: 'chat:tool_end' })
  | (Envelope<ApprovalData> & { event: 'chat:approval' })
  | (Envelope<CompressedData> & { event: 'chat:compressed' })
  | (Envelope<UserData> & { event: 'chat:user' })
  | (Envelope<ContextData> & { event: 'chat:context' })
  | (Envelope<DoneData> & { event: 'chat:done' })
  | (Envelope<StoppedData> & { event: 'chat:stopped' })
  | (Envelope<ErrorData> & { event: 'chat:error' })
  | (Envelope<GapData> & { event: 'chat:gap' })

// ---- /tools ----

/** 工具目录项：助手当前能干什么、风险多大、是否启用 */
export interface ToolVO {
  name: string
  label: string
  category: string
  description: string
  risk: string
  approval: boolean
  mode: string
  params: string[]
  enabled: boolean
  builtin: boolean
}

// ---- /models/capability ----

/** 模型能力画像。known=false 表示本地没有该模型资料，窗口为缺省估算值；
 * window_override=true 表示窗口是你手填的，此时输出预算按窗口 1/8 走（不受厂商上限钳制） */
export interface ModelCapability {
  id: string
  context_window: number
  max_output: number
  window_override: boolean
  thinking: boolean
  vision: boolean
  tool_call: boolean
  known: boolean
  note: string
}

/** 模型级配置：目录 + 用户覆写合并后的最终值。
 * temperature / top_p 为 -1 表示「未设置」（跟随上游默认）；0 是合法的确定性取值。 */
export interface ModelConfigVO {
  provider_id: string
  model: string
  context_window: number
  window_known: boolean
  max_output: number
  temperature: number
  top_p: number
  vision: boolean
  tool_call: boolean
  thinking: boolean
}

export interface UpsertModelConfigREQ {
  provider_id: string
  model: string
  context_window: number
  /** -1 = 未设置（跟随上游默认）；合法取值 0 起 */
  temperature: number
  /** -1 = 未设置（跟随上游默认）；合法取值 0 起 */
  top_p: number
  vision: boolean
  tool_call: boolean
}

// ---- /runtime ----

/** 内置运行时状态。python_error 非空时给出可执行的原因，而不是一句「不可用」 */
export interface RuntimeInfo {
  python_exe: string
  /** bundled = 内置运行时；system = 用系统已装的那份；空 = 未就绪 */
  python_source: string
  python_version: string
  python_error: string
  powershell_exe: string
  powershell_source: string
  powershell_version: string
  powershell_error: string
}

// ---- /stats ----

export interface StatsTotals {
  input: number
  output: number
  cached: number
  total: number
  calls: number
  sessions: number
  latency_ms: number
  avg_latency_ms: number
  /** 0~1 的缓存命中率 */
  cache_hit_rate: number
  /** 每次调用发出时的上下文占用均值 / 峰值 */
  avg_context: number
  peak_context: number
}

/** 长度恒等于 days，缺的日子后端已补零 */
export interface StatsDailyItem {
  date: string
  input: number
  output: number
  cached: number
  total: number
}

export interface StatsModelItem {
  model: string
  total: number
  calls: number
  input: number
  output: number
  cached: number
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

// ---- /docs（内置帮助文档）----

export interface HelpDocVO {
  name: string
  title: string
}

export interface HelpDocRESP {
  name: string
  title: string
  /** 原始 Markdown，渲染走 utils/md.ts */
  content: string
}
