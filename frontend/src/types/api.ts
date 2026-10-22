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
  /** 命中上游缓存的输入量（与 input 并列，不含在 input 里） */
  cached: number
  /** 写入上游缓存的输入量；同样不含在 input 里 */
  cache_write?: number
  total: number
  /** 这一轮发出时上下文占用的窗口量 */
  context: number
  cost: number
}

export interface ToolCall {
  id: string
  name: string
  /** 显示名；界面按 label || name 取 */
  label?: string
  args?: Record<string, unknown>
}

export interface MessageImage {
  mime: string
  /** 纯 base64，不含 data: 前缀 */
  base64: string
}

export interface MessageBlock {
  /** text = 正文；thinking = 思考；tool = 工具调用（结果在紧随其后的 tool 消息里） */
  kind: 'text' | 'thinking' | 'tool'
  text?: string
  tool_call_id?: string
  tool_name?: string
  label?: string
  args?: Record<string, unknown>
}

export interface MessageVO {
  id: string
  role: 'user' | 'assistant' | 'tool' | 'notice'
  /** 条目类型：text / tool / notice */
  type: string
  thinking?: string
  content?: string
  /** 正文 / 思考 / 工具调用的原始顺序；渲染按它来，不按字段类型分组 */
  blocks?: MessageBlock[]
  tool_calls?: ToolCall[]
  tool_call_id?: string
  tool_name?: string
  is_error?: boolean
  /** 仅 assistant：stop / length / toolUse / aborted / error */
  stop_reason?: string
  images?: MessageImage[]
  usage?: UsageVO
  created_at: number
  hidden?: boolean
  /** 非空时渲染成分隔提示条（已压缩 / 换模型）而不是气泡 */
  notice?: string
}

/** 一条 PI 会话：id 与 file 都来自 PI 的会话文件 */
export interface SessionVO {
  id: string
  file: string
  title: string
  workspace: string
  provider: string
  model: string
  thinking_level: string
  message_count: number
  total_tokens: number
  cost: number
  running: boolean
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

/** @ 引用：相对工作区的路径；粘贴的图片直接带 base64 */
export interface AttachmentREQ {
  path: string
  name: string
  image_base64?: string
}

/** 引擎抛回来的问题（confirm / select / input） */
export interface AskData {
  id: string
  /** confirm / select / input / editor：PI 只有这四个提问基元，多选题走扩展自己的 input 回退 */
  method: string
  title: string
  message: string
  options: string[]
  placeholder: string
  prefill: string
  /** 等待上限（毫秒）：0 表示不限时 */
  timeout_ms: number
}

export interface AskClosedData {
  id: string
  reason: string
}

// ---- 模型服务（PI 的 models.json）----

export interface ModelVO {
  id: string
  name: string
  reasoning: boolean
  vision: boolean
  context_window: number
  max_tokens: number
  thinking_levels?: string[]
}

export interface ProviderVO {
  /** models.json 里的键，也是 PI 的 provider 名 */
  id: string
  name: string
  api: string
  base_url: string
  has_key: boolean
  /** 掩码：sk-1••••abcd */
  key_hint: string
  models: ModelVO[]
  is_default: boolean
}

export interface ProviderListRESP {
  providers: ProviderVO[]
  default_provider: string
  default_model: string
  path: string
}

export interface ModelREQ {
  id: string
  name: string
  reasoning: boolean
  vision: boolean
  context_window: number
  max_tokens: number
  thinking_levels?: string[]
}

export interface UpsertProviderREQ {
  id?: string
  name: string
  api: string
  base_url: string
  api_key?: string
  models: ModelREQ[]
}

export interface TestProviderRESP {
  ok: boolean
  model: string
  message: string
}

export interface AvailableModelVO {
  id: string
  name: string
  provider: string
  context_window: number
  max_tokens: number
  reasoning: boolean
  vision: boolean
}

// ---- 技能 ----

export interface SkillVO {
  id: string
  name: string
  description: string
  when_to_use?: string[]
  location: string
  /** global = 用户目录；workspace = 工作区；package = 插件包自带 */
  source: string
  enabled: boolean
}

// ---- 插件与包 ----

export interface ExtensionVO {
  name: string
  path: string
  kind: string
  enabled: boolean
}

export interface PackageVO {
  source: string
  name: string
  path: string
  missing: boolean
  /** npm / git / local：只有前两种能被 pi update 搬到新版本 */
  kind: string
  /** 本地装着的版本；git 与本地目录可能读不到 */
  version: string
  /** 联网查到的最新版本；空表示未检查或查不到 */
  latest_version: string
  updateable: boolean
}

export interface ExtensionListRESP {
  dir: string
  settings_path: string
  entries: ExtensionVO[]
  packages: PackageVO[]
}

// ---- 工具 ----

export interface ToolVO {
  name: string
  description: string
  /** builtin / extension / mcp */
  source: string
  server?: string
  enabled: boolean
}

export interface ToolListRESP {
  tools: ToolVO[]
  /** session = 来自最近一次会话的工具声明；static = 没有会话时的内置表 */
  snapshot: string
  snapshot_at: number
}

// ---- MCP ----

export interface MCPServerVO {
  name: string
  scope: string
  source: string
  enabled: boolean
  exposure: string
  transport: string
  /** connected / error / unknown 等，来自 pi mcp list */
  state: string
  error: string
  tools: string[]
}

export interface MCPListRESP {
  servers: MCPServerVO[]
  global_path: string
  project_path: string
  global_raw: string
  project_raw: string
}

// ---- 设置 ----

export interface BackgroundVO {
  image: string
  opacity: number
  blur: number
}

export interface PISettingsVO {
  default_provider: string
  default_model: string
  default_thinking_level: string
  default_tools: string[]
  compaction_reserve: number
  compaction_keep: number
  agent_dir: string
  settings_path: string
  models_path: string
  mcp_path: string
  sessions_dir: string
  extensions_dir: string
  skills_dir: string
}

export interface WorkspacesRESP {
  workspaces: string[]
  current: string
}

// ---- 运行时与环境 ----

export interface RuntimeInfo {
  node_exe: string
  node_source: string
  node_version: string
  node_error: string
  python_exe: string
  python_source: string
  python_version: string
  python_error: string
  pi_exe: string
  pi_source: string
  pi_version: string
  pi_error: string
}

/** PI 运行环境闸门快照 */
export interface PiEnvVO {
  node_ready: boolean
  node_path: string
  node_version: string
  node_source: string
  node_error: string
  python_ready: boolean
  python_path: string
  python_version: string
  python_source: string
  pi_ready: boolean
  pi_path: string
  pi_version: string
  pi_source: string
  pi_error: string
  agent_dir: string
  model_ready: boolean
  /** 三项全满足才能开始对话 */
  ready: boolean
  /** 首个不满足项：node / pi / model / 空 */
  blocking: string
  /** 最近一次安装 / 升级失败的原因；空表示没有失败记录 */
  install_error: string
  /** 命令执行环境：git-bash / msys / cygwin / powershell / none */
  shell_kind: string
  shell_path: string
  shell_usable: boolean
  shell_error: string
  /** 终端里能不能直接敲 pi */
  terminal_ready: boolean
  terminal_path: string
  terminal_on_path: boolean
  terminal_error: string
}

// ---- 引擎（PI 与运行时）----

export interface EngineStatusVO {
  pi_ready: boolean
  pi_version: string
  pi_path: string
  pi_source: string
  /** true = 这份 PI 是 WorkBaby 自己装的，升级写回本应用的前缀 */
  managed: boolean
  /** PI 的包目录（版本文件与升级落点） */
  install_root: string
  /** 当前活着的 PI 进程数：升级后它们会重启 */
  running: number
  node_ready: boolean
  node_version: string
  node_path: string
  node_source: string
  node_error: string
  python_ready: boolean
  python_version: string
  python_path: string
  python_source: string
  agent_dir: string
  model_ready: boolean
  /** 已声明的插件包数量 */
  packages: number
  blocking: string
  install_error: string
  /** 命令执行环境：git-bash / msys / cygwin / powershell / none */
  shell_kind: string
  shell_path: string
  shell_usable: boolean
  shell_error: string
  /** 终端里能不能直接敲 pi */
  terminal_ready: boolean
  terminal_path: string
  terminal_on_path: boolean
  terminal_error: string
}

export interface PackageLatestVO {
  source: string
  latest_version: string
  update_available: boolean
  err: string
}

export interface EngineLatestVO {
  pi_version: string
  registry_error: string
  update_available: boolean
  packages: PackageLatestVO[]
}

export interface BootstrapVO {
  version: string
  contract_version: number
  default_provider: string
  default_model: string
  workspace: string
  /** 环境闸门：ready=false 时进向导且不 spawn PI */
  pi_env?: PiEnvVO
  settings: Record<string, string>
}

// ---- 统计 ----

export interface StatsTotals {
  input: number
  output: number
  cached: number
  cache_write: number
  total: number
  cost: number
  calls: number
  sessions: number
  messages: number
}

export interface StatsDailyItem {
  /** 桶键：天粒度 YYYY-MM-DD；小时粒度 YYYY-MM-DD HH:00 */
  date: string
  input: number
  output: number
  cached: number
  cache_write: number
  total: number
  cost: number
  calls: number
}

export interface StatsModelItem {
  provider: string
  model: string
  input: number
  output: number
  cached: number
  cache_write: number
  total: number
  cost: number
  calls: number
}

export interface StatsSessionItem {
  session_id: string
  title: string
  workspace: string
  total: number
  cost: number
  updated_at: number
}

export type StatsSpan = '24h' | 'today' | '7d' | '14d' | '30d'

export interface StatsRESP {
  /** 请求的时间范围：24h / today / 7d / 14d / 30d */
  span: StatsSpan
  /** daily 的聚合粒度：day 按天；24h 与 today 按小时 */
  granularity: 'day' | 'hour'
  totals: StatsTotals
  /** 长度恒等于桶数（天数或小时数），缺的桶后端已补零 */
  daily: StatsDailyItem[]
  models: StatsModelItem[]
  sessions: StatsSessionItem[]
}

// ---- 帮助文档 ----

export interface HelpDocVO {
  name: string
  title: string
}

export interface HelpDocRESP {
  name: string
  title: string
  content: string
}

// ---- 工作区文件 ----

export interface FileEntryVO {
  name: string
  dir: boolean
}

export interface FileListVO {
  path: string
  entries: FileEntryVO[]
}

// ---- Git（工作区版本视图）----

export interface GitFileVO {
  path: string
  /** M 改过 / A 新增 / D 删除 / R 改名 / C 复制 / T 类型变化 / ? 未跟踪 / U 冲突 */
  status: string
  staged: boolean
  /** 该文件有未解决冲突；解决后要显式标记，否则 git 不接受提交 */
  conflict: boolean
  additions: number
  deletions: number
}

export interface GitRemoteVO {
  name: string
  url: string
}

export interface GitStatusVO {
  available: boolean
  reason: string
  is_repo: boolean
  workspace: string
  /** 仓库根；工作区是仓库子目录时与 workspace 不同 */
  repo_root: string
  branch: string
  /** 上游引用（如 origin/main）；没有上游为空 */
  upstream: string
  ahead: number
  behind: number
  /** none / merge / rebase / cherry-pick / revert */
  state: string
  state_detail: string
  remotes: GitRemoteVO[]
  files: GitFileVO[]
  conflict: boolean
  git: string
}

export interface GitDiffLineVO {
  kind: string
  old_no: number
  new_no: number
  text: string
}

export interface GitDiffVO {
  path: string
  truncated: boolean
  dropped: number
  lines: GitDiffLineVO[]
}

export interface GitCommitVO {
  hash: string
  short: string
  subject: string
  author: string
  when: number
  files: number
  additions: number
  deletions: number
}

export interface GitCommitDetailVO {
  hash: string
  short: string
  subject: string
  body: string
  author: string
  when: number
  parents: string[]
  files: GitFileVO[]
  diff?: GitDiffVO
}

export interface GitBranchVO {
  name: string
  current: boolean
  /** 远端分支所属远端名；本地分支为空 */
  remote: string
  ahead: number
  behind: number
}

/** 推送的强制级别。默认 none，逐级需要更多确认 */
export type GitForce = 'none' | 'lease' | 'force'

export interface GitPushREQ {
  remote?: string
  branch?: string
  /** 把这次推送的远端建立为上游（首次推送必须） */
  set_upstream?: boolean
  force?: GitForce
  /** 强制推送时必须与 branch 完全一致的分支名 */
  confirm?: string
}

export interface GitPullREQ {
  remote?: string
  branch?: string
  rebase?: boolean
}

export interface GitCommitRESP {
  status: GitStatusVO
  hash: string
}

export interface GitMessageRESP {
  message: string
  err: string
}

export interface CommandVO {
  name: string
  description: string
  /** extension = 扩展注册；prompt = 提示模板；skill = 技能 */
  source: string
}

// ---- SSE 事件载荷 ----

export interface StartData { run_id: string; session_id: string }
export interface DeltaData { entry_id: string; kind: string; delta: string }
export interface ToolStartData { tool_call_id: string; tool: string; label: string; args: Record<string, unknown> }
export interface ToolEndData { tool_call_id: string; ok: boolean; title: string; output: string; duration_ms: number }
export interface CompressedData { tokens_before: number; tokens_after: number }
export interface UserData { entry_id: string; content: string }
export interface DoneData {
  entry_id: string
  stop_reason: string
  usage?: UsageVO
  max_tokens?: number
}
export interface ErrorData { code: string; message: string }
export interface RetryData { attempt: number; max: number; message: string }
export interface SwitchedData { session_id: string; reason: string }
export interface SetupData { phase: string; progress: number; message: string }
export interface GapData { reason: string }
export interface StoppedData { reason: string }

export interface NotifyData { level: 'info' | 'warning' | 'error'; message: string }
export interface StatusData { key: string; text: string }
export interface WidgetData { key: string; lines: string[]; placement: string }
export interface TitleData { title: string }

export interface Envelope<T = unknown> {
  seq: number
  event: string
  data: T
}

export interface ContextData {
  used: number
  window: number
  ratio: number
  known: boolean
  reserve?: number
}

/** SSE 事件的判别联合：切换 event 就能把 data 收窄到对应载荷。 */
export type ServerEvent =
  | (Envelope<StartData> & { event: 'chat:start' })
  | (Envelope<DeltaData> & { event: 'chat:delta' })
  | (Envelope<ToolStartData> & { event: 'chat:tool_start' })
  | (Envelope<ToolEndData> & { event: 'chat:tool_end' })
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
  | (Envelope<AskData> & { event: 'chat:ask' })
  | (Envelope<AskClosedData> & { event: 'chat:ask_closed' })
  | (Envelope<NotifyData> & { event: 'chat:notify' })
  | (Envelope<StatusData> & { event: 'chat:status' })
  | (Envelope<WidgetData> & { event: 'chat:widget' })
  | (Envelope<TitleData> & { event: 'chat:title' })
