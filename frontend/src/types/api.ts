// 前端与后端的契约类型。改字段形态必须同时把 backend/server.ContractVersion +1。
// 一个域一个段落，字段名与 Go 侧的 json tag 一一对应。

export interface Envelope<T> {
  code: string
  message: string
  detail?: string
  data: T
}

// ---------------- 引擎 ----------------

export type EngineKind = 'omp' | 'pi'

/** 能力表：界面按它隐藏入口，不渲染灰按钮。 */
export interface EngineCaps {
  subagents: boolean
  goals: boolean
  todos: boolean
  login: boolean
  fragments: boolean
  queue_edit: boolean
  queue_clear: boolean
  live_steer_count: boolean
  skill_toggle: boolean
  extensions: boolean
  export_html: boolean
  slash_commands: boolean
  /** 复制当前会话开新分支（clone RPC）；omp 没有 */
  session_clone: boolean
  /** 插件启停（omp CLI：plugin enable/disable）；pi 没有 */
  plugin_toggle: boolean
  /** 插件升级（omp CLI：plugin upgrade）；pi 没有 */
  plugin_upgrade: boolean
}

export interface EngineIdentity {
  kind: EngineKind
  exe: string
  version: string
  /** 引擎从哪来：显式路径 / PATH */
  source: 'custom' | 'path' | ''
  /** 安装渠道；只决定「卸载按钮存不存在」，不参与拉起与升级 */
  channel: AppChannel
  caps: EngineCaps
}

/** 安装渠道。unknown 表示看得见文件但不知道谁装的。 */
export type AppChannel = 'npm' | 'pnpm' | 'bun' | 'wb-installer' | 'pi-installer' | 'unknown' | ''

/** 应用页的一行：一个引擎的安装事实与可做的动作。后端已经把「能不能点」算完了。 */
export interface AppRow {
  kind: EngineKind
  app_name: string
  installed: boolean
  version: string
  exe: string
  source: 'custom' | 'path' | ''
  channel: AppChannel
  /** 渠道的人话说明，界面直接显示 */
  channel_label: string
  latest_version: string
  /** 查不到最新版的原因（一句话），空表示查到了 */
  latest_note?: string
  outdated: boolean
  install_command: string
  /** 空表示这个引擎没有自升级命令 */
  update_command: string
  /** 空表示这条渠道没有非交互式卸载 */
  uninstall_command: string
  docs_url: string
  can_install: boolean
  can_update: boolean
  can_uninstall: boolean
  /** 不能操作的原因（一句话），空表示没阻碍 */
  blocker?: string
  busy: boolean
  /** 升级命令原文：动手之前显示它 */
  update_preview?: string
  /** 卸载命令原文 */
  uninstall_preview?: string
}

/** 安装进度帧（SSE `app:progress`）。逐行推，前端 append 到日志面板。 */
export interface AppProgress {
  kind: EngineKind
  action: 'install' | 'update' | 'uninstall'
  phase: 'start' | 'line' | 'done' | 'fail'
  line?: string
  command?: string
  message?: string
}

export interface EngineLayout {
  kind: EngineKind
  root: string
  agent: string
  sessions: string
  skills: string
  plugins: string
  models: string
  settings: string
  exists: boolean
}

export interface RunRow {
  kind: EngineKind
  work_dir: string
  pid: number
  session_id: string
  running: boolean
  protocol: number
  started_at: number
  call_count: number
  last_error?: string
  busy: boolean
}

/** 单个引擎的健康快照：探测 + 进程 + 落盘计数 + 配置现状，一屏要用的全部事实。 */
export interface EngineHealth {
  engine_kind: string
  engine_label: string
  installed: boolean
  exe_path: string
  version: string
  channel_kind: string
  channel_label: string
  latest_version: string
  outdated: boolean
  install_hint: string
  running: boolean
  busy: boolean
  pid: number
  protocol: number
  started_at: number
  call_count: number
  last_error?: string
  session_count: number
  skill_count: number
  plugin_count: number
  mcp_count: number
  /** 四个计数各自的「是不是真扫出来的」。false 时显示「—」而不是那个数字：
   *  拿到的是零值，而零值不等于「有 0 个」。 */
  session_count_known: boolean
  skill_count_known: boolean
  plugin_count_known: boolean
  mcp_count_known: boolean
  /** 落盘布局有没有解出来；解不出来时上面三个路径字段为空 */
  layout_resolved: boolean
  /** 探测没法给出「装没装」的原因：missing / bad-path / timeout。空串表示结论可信 */
  probe_unknown_reason?: string
  config_files: ConfigFile[]
  root_dir: string
  agent_dir: string
  dir_exists: boolean
  caps: Record<string, boolean>
}

/**
 * 一个存活的引擎进程。
 *
 * 键是「引擎种类 + 工作目录」：同一个引擎可能在多个目录各有一个进程，
 * 所以停一个必须两个字段都给——只给 kind 会停错那个。
 */
export interface EngineRun {
  engine_kind: EngineKind
  engine_label: string
  work_dir: string
  process_id: number
  protocol_version: number
  started_at: number
  call_count: number
  last_error?: string
  busy: boolean
  session_id: string
}

/** 一轮进行中的实时读数。`phase` 为空串表示引擎还没报过。 */
export interface ChatLive {
  run_id: string
  phase: '' | 'thinking' | 'text' | 'tool'
  tool_name: string
  tool_call_id: string
  output_tokens: number
}

/** 两个引擎的健康快照 + 全局计数。概览页首屏用。 */
export interface HealthData {
  engines: EngineHealth[]
  active_kind: string
  workspace: string
  /** 有后台补验在跑：屏幕上这些数字可能比实际旧一点 */
  refreshing: boolean
  /** 引擎探测进行中（含「从没探过」）：此时不能说「没装」 */
  detecting: boolean
  /** 这一屏的事实已经全部到手（探测完成 + 没有后台补验）。
   *  为假时界面显示「—」而不是 0：「0 个会话」是一个断言，而那时只是还没数完。 */
  ready: boolean
  /** 上次探测完成的时刻；0 表示从没探过 */
  detected_at: number
  total_sessions: number
  total_skills: number
  total_plugins: number
  total_mcp: number
  /** 上面四个合计里至少有一项没读到真实值：合计偏小，不是数全了 */
  totals_partial: boolean
  /** 当前存活的引擎实例数（键是「引擎 + 工作目录」） */
  live_engines: number
  /** 实例数上限 */
  max_engines: number
  /** 存活实例的明细：哪个引擎、在哪个目录、PID、跑了多久、忙不忙 */
  engine_runs: EngineRun[]
}

export interface Bootstrap {
  version: string
  engines: EngineIdentity[]
  caps: Record<string, EngineCaps>
  active: EngineKind
  workspace: string
  profile: string
  settings: Record<string, string>
  layouts: Record<string, EngineLayout>
  workspaces: WorkspaceRow[]
  skills: Record<string, number>
  sessions: Record<string, number>
  install: Record<string, string>
}

export interface EngineInfo extends EngineIdentity {
  installed: boolean
  layout: {
    root: string
    agent: string
    sessions: string
    skills: string
    plugins: string
    models: string
    settings: string
    exists: boolean
  }
}

export interface EngineStatus {
  engines: EngineInfo[]
  running: RunRow[]
  install: Record<string, string>
  /** 探测正在进行（含「从没探过」）：此时不能说「没装」 */
  detecting: boolean
  /** 上次探测完成的时刻（Unix 毫秒）；0 表示从没探过。
   *
   * 它是「要不要重拉」的判据：engine:detected 那一帧可能丢（探测早于 SSE 连接），
   * 而时间戳是存在后端的事实，不依赖任何一帧有没有送到。 */
  detected_at: number
}

export interface WorkspaceRow {
  workspace_path: string
  exists: boolean
  updated_at?: string
}

// ---------------- 应用偏好 ----------------

/** 设置页看到的全部开关状态。自动开机自启以注册表为准，不以设置文件为准。 */
export interface AppPrefs {
  values: Record<string, string>
  auto_start: boolean
  restore_session: boolean
  last_session_file: string
  last_session_title?: string
  last_engine_kind: EngineKind | ''
  data_dir: string
  log_dir: string
  /** 点窗口关闭按钮时做什么 */
  close_action: 'tray' | 'quit'
  /** 还没给过「收托盘」的首次提示 */
  tray_hint_pending: boolean
}

/** 一个可查看的引擎配置文件。 */
export interface ConfigFile {
  file_path: string
  file_label: string
  exists: boolean
  size_bytes: number
}

export interface ConfigFileList {
  engine_kind: EngineKind | ''
  entries: ConfigFile[]
}

/** 引擎配置原文。found 为假表示「还没生成过」，不是错误。 */
export interface ConfigText {
  file_path: string
  text: string
  truncated: boolean
  found: boolean
}

// ---------------- 会话 ----------------

export interface SessionRow {
  id: string
  path: string
  title: string
  cwd: string
  work_dir: string
  model: string
  updated_at: string
  size: number
  messages: number
  tokens: number
  cost: number
}

export interface SessionItem {
  kind: 'user' | 'assistant' | 'tool' | 'system'
  role: string
  text?: string
  thinking?: string
  blocks?: RawBlock[]
  model?: string
  tool_name?: string
  tool_call_id?: string
  is_error?: boolean
  timestamp?: number
  cost?: number
  tokens?: number
}

export interface SessionDetail extends SessionRow {
  thinking: string
  items: SessionItem[]
}

// ---------------- 对话 ----------------

export type BlockKind = 'text' | 'thinking' | 'tool'

export interface RawBlock {
  type?: string
  text?: string
  thinking?: string
  id?: string
  name?: string
  arguments?: unknown
  intent?: string
}

export interface ChatBlock {
  kind: BlockKind
  text?: string
  tool_call_id?: string
  tool_name?: string
  args?: unknown
  intent?: string
  output?: string
  is_error?: boolean
  status?: 'pending' | 'running' | 'done' | 'error'
  streaming?: boolean
  duration_ms?: number
  /** 被改动的文件路径；只有改文件的工具才有 */
  file_path?: string
  /** 引擎吐出的差异；只有改文件的工具才有 */
  diff?: DiffLine[]
}

/** 差异行。两个引擎的差异格式不同，后端已归一成这一种。 */
export interface DiffLine {
  line_kind: 'file' | 'hunk' | 'add' | 'del' | 'ctx'
  old_no: number
  new_no: number
  line_text: string
}

export interface Turn {
  run_id: string
  user_message: string
  blocks: ChatBlock[]
  model?: string
  error?: string
  stop_reason?: string
}

export interface ModelInfo {
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

export interface ContextUsage {
  tokens: number | null
  contextWindow: number
  percent: number | null
}

export interface TokenCounts {
  input: number
  output: number
  reasoning: number
  cacheRead: number
  cacheWrite: number
  total: number
}

export interface SessionStats {
  sessionId: string
  userMessages: number
  assistantMessages: number
  toolCalls: number
  totalMessages: number
  tokens: TokenCounts
  cost: number
  contextUsage?: ContextUsage
}

export interface QueuedMessages {
  steering: string[]
  followUp: string[]
  liveSteered: number
}

/**
 * 一个附件。两类走两条完全不同的路：
 * - file：只把路径拼进 prompt，引擎用自己的读文件工具去看；
 * - image：走引擎 prompt 的 images 字段（唯一能传二进制的通道）。
 */
export interface ChatAttachment {
  kind: 'file' | 'image'
  file_path?: string
  file_name?: string
  image_base64?: string
  image_mime?: string
}

export interface SlashCommand {
  name: string
  source: string
  description?: string
  aliases?: string[]
  input?: { hint?: string }
  subcommands?: { name: string; description?: string; usage?: string }[]
}

export interface HistoryTurn {
  /** engine-mark 是界面层的分隔标记（切引擎时插的），不是引擎消息 */
  role: 'user' | 'assistant' | 'tool' | 'engine-mark'
  blocks: ChatBlock[]
  model?: string
  /** 这一条助手消息的用量；界面在每条回复下摆读数 */
  usage?: TurnUsage
  /**
   * 这一轮的隐藏标识（内容指纹 # 出现序号），「删除一轮」时回传给后端。
   * 由内容算出，刷新与重启之后仍指得准；直播轮没有它，因此不可删。
   */
  hid?: string
}

/** 一条回复的用量读数。 */
export interface TurnUsage {
  /** 输入 token（不含缓存命中与写入） */
  input_tokens: number
  /** 输出 token */
  output_tokens: number
  /** 缓存命中的 token */
  cache_read_tokens: number
  /** 写入缓存的 token */
  cache_write_tokens: number
}

export interface ChatState {
  kind: EngineKind
  session: string
  session_file: string
  work_dir: string
  engine_running: boolean
  streaming: boolean
  model?: ModelInfo
  thinking_level: string
  levels: string[]
  context?: ContextUsage
  tokens_per_second?: number
  auto_compaction: boolean
  queued?: QueuedMessages
  stats?: SessionStats
  messages: unknown[]
  history: HistoryTurn[]
  commands: SlashCommand[]
  profile: string
  caps: EngineCaps
  /** 没能把引擎切到请求的会话时的原因原话；空表示切好了 */
  bind_err?: string
}

// 引擎的提问卡片（四个基元 + ask 工具）
export interface AskRequest {
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
    recommended?: number
    options: { label: string; description?: string; preview?: string }[]
  }[]
}

export interface UIResponse {
  type?: string
  id: string
  value?: string
  confirmed?: boolean
  cancelled?: boolean
  timedOut?: boolean
  answers?: { id: string; selectedOptions: string[]; customInput?: string }[]
}

// ---------------- 模型与服务商 ----------------

export interface ModelRow {
  kind: EngineKind
  provider: string
  id: string
  name: string
  api: string
  context_window: number
  max_tokens: number
  reasoning: boolean
  input?: string[]
}

export interface ModelEntry {
  model_id: string
  model_name?: string
  api?: string
  reasoning?: boolean
  input?: string[]
  context_window?: number
  max_tokens?: number
}

export interface ProviderView {
  /** 这条服务商属于哪个引擎：两边配置完全独立，同名可以各有一份 */
  engine_kind: EngineKind
  provider_name: string
  base_url: string
  api: string
  auth: string
  api_key: string
  has_key: boolean
  models: ModelEntry[]
}

export interface ProvidersView {
  providers: ProviderView[]
  config_path: string
}

/**
 * 向上游探测的入参。
 *
 * 字段全可选：不填的项由后端从已保存的服务商配置里取。
 * 密钥尤其如此——界面上拿到的是掩码，原值只在后端的配置里。
 */
export interface ProviderProbeREQ {
  engine_kind?: EngineKind
  /** 服务商名：给了就从配置里取地址、协议与密钥 */
  provider_name?: string
  base_url?: string
  api?: string
  api_key?: string
  auth?: string
}

export interface LoginProvider {
  id: string
  name: string
  available: boolean
  authenticated: boolean
}

// ---------------- 技能与插件 ----------------

export interface SkillRow {
  name: string
  description: string
  path: string
  dir: string
  source: 'project' | 'user' | 'managed' | 'plugin'
  globs?: string[]
  always_apply?: boolean
  hidden?: boolean
  enabled: boolean
  editable: boolean
  kind: EngineKind
}

/** 矩阵行：一个技能（按名合并两引擎的扫描结果），cells 是每个引擎那份的原文 */
export interface SkillMatrixRow {
  name: string
  description: string
  /** 分组用：两边的来源取优先级最高的那个（与引擎加载优先级一致） */
  group_source: SkillRow['source']
  cells: Partial<Record<EngineKind, SkillRow>>
}

/** 一条已装插件。清单来自引擎自己的登记文件，不是扫 node_modules 的结果。 */
export interface PluginRow {
  engine_kind: string
  plugin_name: string
  plugin_version: string
  plugin_source: string
  plugin_path: string
  plugin_desc?: string
  resource_kinds: string[]
  scope_kind: string
  enabled: boolean
  /** 包目录是否真的存在：清单里登记了但目录被删掉时是 false */
  installed: boolean
}

export interface PluginListData {
  plugins: PluginRow[]
  registry_path: string
  read_error?: string
}

// ---------------- MCP ----------------

/** 一个 MCP 服务器。两个引擎共用 mcp.json 的 mcpServers 形状。 */
export interface McpServer {
  /** 这条服务器属于哪个引擎：两边读各自的 mcp.json，同名可以各有一条 */
  engine_kind: EngineKind
  server_name: string
  transport_kind: string
  command_line?: string
  server_url?: string
  desc?: string
  enabled: boolean
  /** 被顶层停用名单压住：界面要说清为什么点不动 */
  disabled_by_deny: boolean
  scope_kind: string
  config_path: string
  live_status?: string
  live_error?: string
  tool_names?: string[]
}

/** 矩阵行：一个服务器（按名合并两引擎的定义），cells 是每个引擎那份的原文 */
export interface McpMatrixRow {
  server_name: string
  transport_kind: string
  command_line?: string
  server_url?: string
  desc?: string
  cells: Partial<Record<EngineKind, McpServer>>
}

export interface McpListData {
  engine_kind: string
  servers: McpServer[]
  user_path: string
  project_path?: string
  read_errors?: string[]
  live_checked: boolean
}

// ---------------- 工作区 ----------------

export interface FileEntry {
  entry_name: string
  dir: boolean
  size?: number
}

export interface FileList {
  dir_path: string
  entries: FileEntry[]
  truncated: boolean
}

/** 全工作区搜索的一个命中：@ 引用直接用 file_path */
export interface FileHit {
  file_path: string
  entry_name: string
}

/** 会话 HTML 导出的出参：自包含单文件，前端 Blob 下载 */
export interface SessionExport {
  file_name: string
  content: string
}

export interface FileSearch {
  query: string
  hits: FileHit[]
  truncated: boolean
}

export interface FileRead {
  file_path: string
  content: string
  size: number
  binary: boolean
  truncated: boolean
}

export type GitChange = 'M' | 'A' | 'D' | 'R' | 'C' | 'T' | '?' | 'U'

export interface GitFile {
  file_path: string
  change_code: GitChange
  staged: boolean
  conflict: boolean
  additions: number
  deletions: number
}

export interface GitRemote {
  remote_name: string
  remote_url: string
}

export interface GitStatus {
  available: boolean
  unavailable_reason: string
  is_repo: boolean
  workspace: string
  repo_root: string
  branch_name: string
  upstream_ref: string
  ahead: number
  behind: number
  repo_state: string
  repo_state_detail: string
  remotes: GitRemote[]
  changed_files: GitFile[]
  conflict: boolean
  git_version: string
}

export interface GitDiffLine {
  line_kind: 'file' | 'hunk' | 'add' | 'del' | 'ctx'
  old_no: number
  new_no: number
  line_text: string
}

export interface GitDiff {
  file_path: string
  truncated: boolean
  dropped: number
  diff_lines: GitDiffLine[]
}

export interface GitCommit {
  commit_hash: string
  short_hash: string
  subject: string
  author_name: string
  commit_time: number
  file_count: number
  additions: number
  deletions: number
}

/** 单个提交的完整内容：元信息 + 改动文件 + 逐行差异。 */
export interface GitCommitDetail {
  commit_hash: string
  short_hash: string
  subject: string
  body: string
  author_name: string
  commit_time: number
  parent_hash: string[]
  changed_file: GitFile[]
  diff?: GitDiff
}

export interface GitBranch {
  branch_name: string
  current: boolean
  remote_name: string
  ahead: number
  behind: number
}

export interface GitCommitResult {
  status: GitStatus
  commit_hash: string
}

// ---------------- SSE ----------------

export interface StreamEvent {
  seq: number
  event: string
  data: Record<string, unknown>
  sent_at: number
}

export interface ChatPatch {
  run_id: string
  mode: 'delta' | 'block' | 'blocks'
  index: number
  kind: BlockKind
  delta?: string
  block?: ChatBlock
  blocks?: ChatBlock[]
}

// ---------------- 子代理（仅 omp） ----------------

export interface AgentProgress {
  completion: number
  stage: string
  current_tool: string
  recent_tools: string[]
  tool_count: number
  tokens: number
  context_used: number
  context_max: number
  cost: number
  duration_ms: number
  model: string
}

export interface AgentRow {
  id: string
  index: number
  agent: string
  agent_source: string
  description: string
  status: string
  task: string
  session_file: string
  parent_tool_call_id: string
  last_update: number
  progress: AgentProgress
}

export interface TranscriptPage {
  entries: Record<string, unknown>[]
  messages: Record<string, unknown>[]
  next: number
  reset: boolean
}

/** 内置帮助文档条目。 */
export interface HelpDocVO {
  /** 文档名，路由与拉取正文都用它 */
  doc_name: string
  /** 展示标题 */
  doc_title: string
}

/** 帮助文档正文。 */
export interface HelpDocRESP {
  /** Markdown 原文 */
  doc_content: string
}
