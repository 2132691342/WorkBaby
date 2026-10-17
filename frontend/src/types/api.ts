// 与后端 RESP 的 json tag 一一对应；字段名一律 snake_case。

export interface Resp<T> {
  /** 业务码是数字字符串（"0" 成功）；与 HTTP 状态码的 int 从类型上分开 */
  code: string
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
  /** 显示名；界面按 label || name 取，缺省回落到工具名 */
  label?: string
  args?: Record<string, unknown>
}

export interface MessageImage {
  mime: string
  /** 纯 base64，不含 data: 前缀 */
  base64: string
}

export interface MessageVO {
  id: string
  role: 'user' | 'assistant' | 'tool' | 'notice'
  /** 条目类型：text / tool / notice */
  type: string
  thinking?: string
  content?: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  tool_name?: string
  is_error?: boolean
  /** 仅 assistant：stop / length / toolUse / aborted / error */
  stop_reason?: string
  images?: MessageImage[]
  usage?: UsageVO
  created_at: number
  latency_ms?: number
  /** 扩展注入的隐藏内容：不渲染成气泡 */
  hidden?: boolean
  /** 非空时渲染成分隔提示条（已压缩 / 分支摘要 / 换模型）而不是气泡 */
  notice?: string
}

export interface SessionVO {
  id: string
  title: string
  workspace: string
  provider_id: string
  model: string
  /** off / minimal / low / medium / high / xhigh / max；空 = 跟随 PI 默认 */
  thinking_level: string
  /** auto / ask_write / readonly */
  permission: string
  /** PI 的会话 uuid；懒创建，发过消息后才有 */
  pi_session_id: string
  /** PI 的 JSONL 绝对路径 */
  pi_session_file: string
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
  /** low / medium / high */
  risk: string
  /** 为什么要问这一句 */
  reason: string
  /** pending / approved / denied */
  status: string
  created_at: number
  /** 0 = 还没决策 */
  decided_at: number
}

export interface ProviderVO {
  id: string
  name: string
  /** PI 的协议 id：openai-completions / openai-responses / anthropic-messages / google-generative-ai */
  api: string
  base_url: string
  /** 密钥已配置（不回传明文） */
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
  /** builtin = 随程序内置；global = 用户安装 */
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
  /** pending / indexed / failed */
  status: string
  error?: string
  created_at: number
}

export interface SearchHitVO {
  doc_id: string
  title: string
  path: string
  /** 命中所在切片序号 */
  seq: number
  content: string
  /** 相关度占位：当前检索按命中顺序返回，恒为 0 */
  score: number
}

export interface BootstrapVO {
  version: string
  contract_version: number
  default_provider_id: string
  default_model: string
  workspace: string
  permission: string
  /** 环境闸门：ready=false 时进向导且不 spawn PI */
  pi_env?: PiEnvVO
  settings: Record<string, string>
  /** 本次启动被按拒绝收口的上次残留确认；空数组表示没有 */
  expired_approvals?: ApprovalVO[]
}

/** PI 运行环境闸门快照 */
export interface PiEnvVO {
  node_ready: boolean
  node_path: string
  node_version: string
  node_source: string
  node_error: string
  pi_ready: boolean
  pi_path: string
  pi_version: string
  pi_source: string
  pi_error: string
  model_ready: boolean
  /** 三项全满足才能开始对话 */
  ready: boolean
  /** 首个不满足项：node / pi / model / 空 */
  blocking: string
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

/** 一条长期记忆 */
export interface MemoryVO {
  id: string
  content: string
  /** manual = 用户在界面上写的，auto = 助手在对话里记下的 */
  source: string
  enabled: boolean
  created_at: number
  updated_at: number
}

export interface ListMemoriesRESP {
  items: MemoryVO[]
}

/** 自定义背景 */
export interface BackgroundVO {
  image: string
  opacity: number
  blur: number
}

// SSE 事件载荷。code / reason 类字段的取值见各接口的错误码表与 docs/ERROR-CODES.md。
export interface StartData { run_id: string; session_id: string }
/** kind：text = 正文，thinking = 思考过程 */
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
  /** stop / length（撞输出上限）/ toolUse / aborted / error */
  stop_reason: string
  usage?: UsageVO
  /** 模型声明的输出上限；撞 length 时用它说清「撞的是哪个上限」 */
  max_tokens?: number
}
export interface ErrorData { code: string; message: string }
/** PI 正在自动重试上游：没有这个事件，界面在重试期间看上去就是卡死 */
export interface RetryData { attempt: number; max: number; message: string }
/** 会话被切走（回退分叉 / 工作区重建进程），前端必须拉新快照 */
export interface SwitchedData { session_id: string; reason: string }
/** 环境安装进度（npm 装 PI 可能几十秒） */
export interface SetupData { phase: string; progress: number; message: string }
/** reason：replay_overflow = 重放缓冲里已没有 Last-Event-ID 之后的事件 */
export interface GapData { reason: string }
/** reason：恒为 user（托盘与快捷键也归为用户主动停止） */
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
  | (Envelope<RetryData> & { event: 'chat:retry' })
  | (Envelope<SwitchedData> & { event: 'chat:switched' })
  | (Envelope<SetupData> & { event: 'chat:setup' })

// ---- /tools ----

/** 工具目录项：助手当前能干什么、风险多大、是否启用 */
export interface ToolVO {
  name: string
  label: string
  /** file / shell / code / web / data */
  category: string
  description: string
  /** low / medium / high；高风险 = 必须审批 */
  risk: string
  approval: boolean
  /** sequential / parallel：parallel 的调用会被并发执行 */
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
  /** true = max_output 是用户手填的覆写值；false = 按窗口派生 */
  max_output_set: boolean
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
  /** 0 = 按窗口派生（窗口 1/8，再与厂商硬上限取小）；>0 = 直接作为下发预算 */
  max_output: number
  /** -1 = 未设置（跟随上游默认）；合法取值 0 起 */
  temperature: number
  /** -1 = 未设置（跟随上游默认）；合法取值 0 起 */
  top_p: number
  vision: boolean
  tool_call: boolean
  /** 与内置目录取「或」：只能把目录认不出的推理模型补成推理型 */
  thinking: boolean
}

// ---- /runtime ----

/** 内置运行时状态。python_error 非空时给出可执行的原因，而不是一句「不可用」 */
export interface RuntimeInfo {
  node_exe: string
  /** bundled = 内置运行时；system = 用系统已装的那份；空 = 未就绪 */
  node_source: string
  node_version: string
  node_error: string
  pi_exe: string
  /** npm-global = 装在内置 npm 前缀；system = 在系统 PATH；env = 显式指定 */
  pi_source: string
  pi_version: string
  pi_error: string
  python_exe: string
  /** 取值同 node_source：bundled / system / 空 */
  python_source: string
  python_version: string
  python_error: string
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
  /** 本地日期，格式 YYYY-MM-DD */
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
  /** 统计窗口（天）：7 / 14 / 30 */
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

// ---- /git（工作区版本状态，只读）----

/** 改动文件。status：M 改过 / A 新增 / D 删除 / R 改名 / ? 还没被 Git 跟踪 */
export interface GitFileVO {
  path: string
  status: string
  /** 已放进待提交区（与「只改了工作区」是两种处置动作） */
  staged: boolean
  additions: number
  deletions: number
}

/** 工作区版本状态。available=false 时 reason 说清「没装 Git」还是「不是仓库」 */
export interface GitStatusVO {
  available: boolean
  reason: string
  is_repo: boolean
  workspace: string
  /** 分支名；空串 = 游离 HEAD（不在任何分支上） */
  branch: string
  ahead: number
  behind: number
  files: GitFileVO[]
  /** 系统里的 git 版本串，不可用为空 */
  git: string
}

/** 差异行。kind：file 文件头 / hunk 段头 / add 新增 / del 删除 / ctx 上下文 */
export interface GitDiffLineVO {
  kind: string
  /** 旧侧行号，0 = 该侧没有行号（hunk 头 / 新增行） */
  old_no: number
  /** 新侧行号，0 = 该侧没有行号（hunk 头 / 删除行） */
  new_no: number
  text: string
}

/** 一份差异。truncated 为真时 dropped 是被丢掉的行数 */
export interface GitDiffVO {
  path: string
  truncated: boolean
  dropped: number
  lines: GitDiffLineVO[]
}

/* ================= 指挥台（specs/16-21） ================= */

/** MCP 外部服务配置 */
export interface MCPServerVO {
  name: string
  command: string
  args: string[]
  /** 只列环境变量名，值不回传 */
  env_keys: string[]
  cwd: string
  disabled: boolean
  /** 单次调用超时（秒），0 = 用缺省 30 */
  timeout: number
  /** 上次探测是否成功；配合 last_probe_at 才有「没测过」与「测失败」之分 */
  last_probe_ok: boolean
  /** 上次探测时刻，0 = 从未探测 */
  last_probe_at: number
  /** 上次探测的错误，空 = 无错 */
  last_error: string
  tool_count: number
}
/** 保存 MCP 配置：后端先校验再落盘，解析不过一个字符都不写 */
export interface SaveMCPConfigREQ {
  raw: string
}
/** MCP 配置面板要的全部东西：文件在哪、原文、解析结果与解析错误 */
export interface MCPConfigRESP {
  path: string
  raw: string
  servers: MCPServerVO[]
  /** 空串 = 文件解析正常 */
  parse_error: string
}
export interface MCPTemplateVO {
  /** blank 空配置 / stdio 通用 stdio 样板 / example 可跑通的示例 */
  kind: string
  label: string
  hint: string
  raw: string
}
export interface ProbeMCPServerREQ {
  name: string
}
export interface MCPToolVO {
  name: string
  label: string
  description: string
  server: string
  parameters: Record<string, unknown>
  enabled?: boolean
}

/** 执行时间线：一次 run 里工具动作与正文长度。 */
export interface RunStep {
  kind: string
  name: string
  label: string
  at: number
  ms: number
  ok: boolean
  detail: string
}

export interface RunTimeline {
  chars: number
  steps: RunStep[]
}

/** 归档结果：只丢逐字对话，留结论 / 时间线 / 用量账。 */
export interface ArchiveVO {
  retention_days: number
  sessions: number
  detached_runs: number
}

/** 评估指标：自动采集，界面与回归对比都读它。 */
export interface EvalMetrics {
  input_tokens: number
  output_tokens: number
  cached_tokens: number
  latency_ms: number
  tool_calls: number
  tool_failures: number
  approvals: number
  truncated: boolean
}

export interface EvalVO {
  id: string
  run_id: string
  /** session（对话级） */
  scope: string
  scope_id: string
  score: number
  note: string
  metrics: EvalMetrics
  created_at: number
  score_delta: number
  total_delta: number
  orphaned: boolean
}

export interface BaselineVO {
  id: string
  eval_id: string
  scope: string
  scope_id: string
  score: number
  created_at: number
}

export interface CompareVO {
  baseline_id: string
  run_id: string
  score_delta: number
  total_delta: number
  base_summary: string
  run_summary: string
  base_metrics: EvalMetrics
  run_metrics: EvalMetrics
}

/** 一次 run 的账；正文不在这里，正文在消息里。 */
export interface RunVO {
  id: string
  session_id: string
  session_title: string
  provider_id: string
  model: string
  /** running / done / failed / aborted */
  status: string
  /** completed / length / max_turns / aborted / error */
  stop_reason: string
  turns: number
  tokens: number
  input: number
  output: number
  cached: number
  context: number
  max_tokens: number
  compressed: number
  started_at: number
  finished_at: number
  duration_ms: number
  can_resume: boolean
}

/** 轮末上下文快照：回答「第几轮时窗口里有多少」。 */
export interface ContextSnapshotDO {
  id: string
  run_id: string
  session_id: string
  turn: number
  used: number
  window: number
  compressed: number
  created_at: number
}

/** 一次运行前打的位点：回到它就是移动会话叶子，不删条目。 */
export interface CheckpointVO {
  id: string
  session_id: string
  run_id: string
  leaf_entry_id: string
  turn: number
  prompt: string
  created_at: number
}

export interface RunDetailVO extends RunVO {
  timeline?: RunTimeline
  snapshots: ContextSnapshotDO[]
  checkpoints: CheckpointVO[]
}
export interface HistoryMessage {
  role: string
  text: string
  created_at: number
}
export interface HistoryEntry {
  id: string
  /** 来源适配器：claude-code / codex / custom（custom = 用户自加的目录） */
  source: string
  project: string
  title: string
  path: string
  started_at: number
  message_count: number
  /** 逐条消息：只在点开某条历史（read）时填充 */
  messages?: HistoryMessage[]
}
export interface HistoryScanRESP {
  entries: HistoryEntry[]
  /** 解析失败或超大小上限而跳过的文件数 */
  skipped: number
  /** 扫描超过时间上限：结果是部分视图，界面要说明 */
  timed_out: boolean
}

/** 项目架构图（specs/21） */
export interface ArchNode {
  id: string
  label: string
  files: number
  /** 行数含注释，只作量级参考 */
  lines: number
  /** 被多少个模块依赖（地基模块这里最高） */
  indegree: number
  /** 从根出发的依赖深度 */
  depth: number
}
/** from 依赖 to */
export interface ArchEdge { from: string; to: string }
export interface ArchCoverage {
  /** 成功解析的源文件数 */
  parsed: number
  /** 含动态导入（import(...)）的文件数：这部分依赖静态解析不出来 */
  dynamic: number
  /** 跳过的文件数（二进制 / 超大 / 不支持的扩展名） */
  skipped: number
}
export interface ArchGraph {
  root: string
  nodes: ArchNode[]
  edges: ArchEdge[]
  coverage: ArchCoverage
}
/** kind：main = 程序入口；config = 工程配置文件 */
export interface ArchEntrypointVO { path: string; kind: string; label: string }
