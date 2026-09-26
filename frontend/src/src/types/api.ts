/**
 * 统一响应包裹：code（0 = OK，非 0 为错误码）、message、data。
 * 全域字段 snake_case，与后端 json tag 严格一致。
 */
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

/** 后端 /api/v1/meta/health 返回结构。 */
export interface HealthInfo {
  status: string
  phase: string
  uptime_ms: number
  db_enabled: boolean
  providers: number
}

/** 后端 /api/v1/meta/runtime 返回结构（内置运行时状态）。 */
export interface RuntimeStatus {
  home: string
  bundled_dir: string
  ready: boolean
  error?: string
  assets: RuntimeAssetStatus[]
}

/** 单个内置运行时资产（node / python / powershell）状态。 */
export interface RuntimeAssetStatus {
  id: string
  version: string
  ready: boolean
  path: string
  executable: string
  archive: string
  archive_found: boolean
  error?: string
}

/** 后端 /api/v1/kv/{k} 返回结构（V1 meta 表）。 */
export interface MetaEntry {
  k: string
  v: string
  updated_at: number
}

/** 工具：判断业务成功（code === 0）。 */
export function isOk<T>(resp: ApiResponse<T>): boolean {
  return resp.code === 0
}

// ===== 聊天 =====

/** 会话（GET /api/v1/chat/sessions 返回 SessionRESP）。 */
export interface Session {
id: string
name: string
user_id: string
provider_id: string
model: string
/** 逻辑文件夹树关联 id（历史字段，UI 不再直接使用） */
workspace_id: string | null
/** 会话绑定的外部工作目录（绝对路径；空 = 默认工作区）。 */
workspace_path: string | null
active: boolean
message_count: number
last_message_at: number | null
metadata_json: string | null
status: string
/** 会话级工具权限模式（restricted/default/auto_edit/yolo）；空 = 跟随全局设置 */
permission_mode: string | null
/** 会话类别（normal 主会话 / side 辅助对话；辅助对话不进侧栏，只在右栏辅助面板呈现） */
kind: string | null
/** 置顶：侧栏列表最前 */
pinned: boolean
/** 分叉来源会话 ID；根会话为空 */
parent_id: string | null
/** 分叉点消息 seq；根会话为 0 */
branch_point: number
created_at: number
updated_at: number
}

/** 分页响应（PageRESP<T>）。 */
export interface Page<T> {
  items: T[]
  total: number
}

/** 参数取值来源层级。 */
export type ParamSource = 'provider' | 'default' | 'builtin' | 'request'

/** 当前会话实际生效的运行参数（GET /api/v1/chat/sessions/:id/params）。 */
export interface EffectiveParams {
  session_id: string
  provider_id: string
  model: string
  temperature: number
  temperature_from: ParamSource
  thinking_effort: string
  thinking_from: ParamSource
  context_window: number
  context_window_from: ParamSource
  compression_ratio: number
  compression_from: ParamSource
  context_budget: number
  max_input_chars: number
}

/** 消息（MessageRESP）。 */
export interface Message {
  id: string
  session_id: string
  seq: number
  run_id: string | null
  role: string
  content: string
  /** 思考过程（thinking 模型的 reasoning 文本；未产生思考时为 null）。 */
  thinking?: string | null
  tool_call_id: string | null
  tool_calls_json: string | null
  status: string
  /** 消息分层：'ui' = 仅界面可见（不进 LLM 上下文）；空 = 正常参与上下文。 */
  context_scope?: string | null
  stop_reason: string | null
  model: string | null
  input_tokens?: number | null
  output_tokens?: number | null
  /** 命中缓存的输入 token（input_tokens 的一个子维度；同一批 token 不叠加）。 */
  cache_read_tokens?: number | null
  total_tokens?: number | null
  latency_ms?: number | null
  cost?: string | null
  created_at: number
  updated_at: number
  /** 持久化过程块：assistant 消息的工具调用/结果/产物，刷新后复现。 */
  blocks?: MessageBlock[] | null
  /** user 消息附件（粘贴/拖拽/选择的图片与文件）。 */
  attachments?: MessageAttachment[] | null
}

/** 消息附件（MessageAttachment）：图片可随消息发给多模态模型。 */
export interface MessageAttachment {
  id: string
  name: string
  mime: string
  size: number
  kind: 'image' | 'file'
  /** 前端直读地址（/files/files/{id}）。 */
  url: string
}

/** 消息块类型（与后端 domain.MessageBlockKind 对齐）。
 *  'text' 为正文片段：模型在两次工具调用之间的叙述按真实位置落块，
 * 这样刷新回看时的顺序与流式期一致（message.content 仍是全文，供复制/导出用）。 */
export type MessageBlockKind = 'thinking' | 'text' | 'tool_call' | 'tool_result' | 'artifact' | 'genui' | 'skill'

/** 消息块（MessageBlockRESP）：块即行，payload 为 JSON 字符串。 */
export interface MessageBlock {
  id: string
  message_id: string
  seq: number
  kind: MessageBlockKind
  payload: string
  created_at: number
}

/** 本轮命中的 Skill（chat:skill 事件与 skill 消息块同构）。 */
export interface SkillHit {
  name: string
  /** builtin / download / custom。 */
  source?: string
  version?: string
  description?: string
  /** 本轮放开的工具白名单；空 = 不限制。 */
  tools?: string[]
  /** 注入正文字符数。 */
  injected_chars?: number
}

/** 流式聊天请求体（ChatStreamReq）。 */
export interface ChatStreamReq {
  message: string
  session_id?: string
  provider_id?: string
  model_id?: string
  file_ids?: string[]
  /** 采样温度（可选 0.0~2.0；请求级覆盖 provider 级与全局默认 0.2）。 */
  temperature?: number | null
  /** 思考强度（可选 off/low/medium/high；请求级覆盖 provider 级与全局默认 medium）。 */
  thinking_effort?: string | null
  /** 检查点续跑：设置后走 POST /chat/runs/{id}/resume（路径上的 id 是 runID），其余字段忽略。 */
  resume_run_id?: string
}

/** MidTurn Queue 一条消息。 */
export interface QueuedMessage {
  id: string
  text: string
}

/** SSE 事件（AgentEventMapper → FrontendEventFormatter 的内层事件）。 */
export interface ChatStreamEvent {
  type: string
  data: unknown
}

// ===== 后端跨端事件载荷契约（与 internal/domain/chat_event.go 一一对应） =====
// 流式高频增量（chat:stream / chat:thinking）仍走极简 map（零转换），只有
//「形状容易被写歪」的跨端事件在此定义强类型，前后端共用同一份形状。

/** 工具调用载荷（chat:tool；后端 domain.ChatToolCallEvent）。 */
export interface ChatToolCallPayload {
  id: string
  name: string
  /** 原始 JSON 字符串（不是对象）——两处曾因形态不一让前端写防御式兜底。 */
  arguments: string
  /** 工具自述动作（"正在编辑 app.ts"）。 */
  activity?: string
  /** 子 Agent 委派来源标签。 */
  agent?: string
}

/** 工具结果载荷（chat:tool-result；后端 domain.ChatToolResultEvent）。 */
export interface ChatToolResultPayload {
  id: string
  name: string
  content?: string
  /** 故障错误；与 refused 互斥（refused 是策略拒绝，非故障）。 */
  error?: string
  duration_ms?: number
  agent?: string
  ui_hint?: string
  data?: Record<string, unknown>
  /** 执行期元数据：cwd / same_failure_count / adaptive_hint / truncated_bytes。 */
  meta?: Record<string, string>
  refused?: boolean
  refused_reason?: string
}

/** 审批 / 补问载荷（chat:approval；后端 domain.ChatApprovalEvent，两种 kind 同形状）。 */
export interface ChatApprovalPayload {
  id: string
  /** 审批：可读命令；补问：问题正文。 */
  command: string
  reason?: string
  /** needs_approval | irreversible | input_required。 */
  risk: string
  /** 不可逆与补问恒 false（后端零值），前端据此隐藏「本会话允许」选项。 */
  can_remember: boolean
}

/** 本轮用量（chat:stats；后端 domain.ChatStatsEvent）。 */
export interface ChatStatsPayload {
  turn: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  total_tokens: number
  latency_ms: number
}

/** 上下文压缩（chat:compressed；后端 domain.ChatCompressedEvent）。
 *  确定性折叠不产出 recovery refs——该字段是历史遗留的前端期待（后端从未下发），已移除。 */
export interface ChatCompressedPayload {
  removed_messages: number
  /** 压缩器标识：内核只保留确定性折叠一种（"micro"）。 */
  filter_key?: string
  truncated?: boolean
  cutoff_at?: number
  summary?: string
}

/** 告警（chat:warn；后端 domain.ChatWarnEvent，三种 kind 共用同一形状）。
 *  kind: unbacked_claim | file_out_of_sandbox | agent_model_override；扩展字段按 kind 生效。 */
export interface ChatWarnPayload {
  kind: string
  message: string
  rel_path?: string
  path?: string
  workspace?: string
  sandbox?: string
  agent?: string
  session_model?: string
  model?: string
}

/** 建流瞬时错误退避重试（chat:retry；后端 domain.ChatRetryEvent）。 */
export interface ChatRetryPayload {
  attempt: number
  delay_ms: number
}

/** run 内错误（chat:error；后端 domain.ChatErrorEvent）。与 ChatDoneEvent 共用终态通道。 */
export interface ChatErrorPayload {
  code: number
  message: string
}

/** 子 Agent 委派起始（chat:subagent-start；后端 domain.ChatSubagentStartEvent）。 */
export interface ChatSubagentStartPayload {
  sub_run_id: string
  agent?: string
}

/** 子 Agent 委派收尾（chat:subagent-done；后端 domain.ChatSubagentDoneEvent）。 */
export interface ChatSubagentDonePayload {
  sub_run_id: string
  agent?: string
  reason: string
}

/** 子 Agent 委派中途错误（chat:subagent-error；后端 domain.ChatSubagentErrorEvent）。 */
export interface ChatSubagentErrorPayload {
  sub_run_id: string
  agent?: string
  message: string
}

/** 队列取出可见性（chat:queue-drained；后端 domain.ChatQueueDrainedEvent）。
 *  PI Phase 3：前端 / 审计感知「用户消息已入队并被消费」。 */
export interface ChatQueueDrainedPayload {
  queue: 'steering' | 'follow_up'
  count: number
  mode?: string
  turn: number
}

/** 流式 stats 事件 payload（usage/耗时/缓存命中/cost）。 */
export interface ChatStats {
  input_tokens?: number
  output_tokens?: number
  /** 命中缓存的输入 token（输入的子维度，不叠加）。 */
  cache_read_tokens?: number
  /** 写入缓存的输入 token（Anthropic cache_creation；其余模型多为 0）。 */
  cache_creation_tokens?: number
  total_tokens?: number
  latency_ms?: number
  cost?: string
}

/** 交付文件（present_files 工具 → artifact 事件）。 */
export interface ArtifactFile {
  path: string
  name: string
  kind: string
  size?: number
  url?: string
}

/** artifact 事件 payload（ChatEventBus 发布的交付事件）。 */
export interface ArtifactPayload {
  files: ArtifactFile[]
  explanation?: string
}

/** 工作区文件（GET /api/v1/chat/workspace/:id/files）。 */
export interface WorkspaceFile {
  path: string
  name: string
  ext: string
  kind: string
  size: number
  url: string
  modified_at: number
}

// ===== 资源页 =====

/** LLM 模型配置（GET /api/v1/ai-provider 返回 AiProviderRESP，api_key 已脱敏）。 */
export interface AiProvider {
  id: string
  name: string
  kind: string
  api_key_masked: string
  base_url: string | null
  model: string
  alias: string | null
  tier: string
  enabled: boolean
  context_window: number | null
  /** 单次最大输出 token；0/空 = 不限制。 */
  max_output_tokens: number | null
  /** 压缩比例（0.1~1.0，上下文已使用占比超过该值即触发压缩；缺省 0.9）。 */
  compress_ratio: number | null
  /** 默认采样温度（0.0~2.0，可选；null=走全局默认 0.2）。 */
  temperature: number | null
  /** 默认核采样（0~1；0 = 走全局默认）。 */
  top_p: number | null
  /** 默认思考强度（off/low/medium/high，可选；null=走全局默认 medium）。 */
  thinking_effort: string | null
  /** 思维参数协议方言（空 = 自动探测）。 */
  thinking_style: string | null
  /** 实际生效方言（auto 探测结果，展示用）。 */
  thinking_style_resolved: string | null
  supports_tool_call: boolean | null
  supports_vision: boolean | null
  supports_reasoning: boolean | null
  tool_call_effective: boolean
  vision_effective: boolean
  reasoning_effective: boolean
  capabilities_json: string | null
  pricing_json: string | null
  created_at: number
  updated_at: number
}

/** 可切换模型（GET /api/v1/ai-provider/available 返回 AvailableModelRESP）。 */
export interface AvailableModel {
  id: string
  name: string
  kind: string
  model: string
  alias: string | null
  tier: string
  enabled: boolean
  context_window?: number | null
  max_output_tokens?: number | null
  compress_ratio?: number | null
  temperature?: number | null
  thinking_effort?: string | null
  thinking_style?: string | null
  tool_call?: boolean
  vision?: boolean
  reasoning?: boolean
}

/** Provider Kind 元数据（GET /api/v1/ai-provider/kinds 返回 ProviderKindsRESP）。
 *  后端 domain.ProviderKindMeta 字段一一对应；前端不写死 kind 列表，
 *  Settings 下拉直接从后端拉，避免"前端看得见、后端没实现"的契约漂移。 */
export interface AiProviderKind {
  kind: string
  label: string
  base_url_default: string
  model_placeholder: string
}

/** 常见模型服务预设（GET /api/v1/ai-provider/presets）：新增 provider 一键预填。 */
export interface ProviderPreset {
  name: string
  kind: string
  base_url: string
  models: string[]
  note?: string
}

/** 免审授权（GET /api/v1/chat/approval-grants）：「本会话允许」的持久化授权。 */
export interface ApprovalGrant {
  id: string
  command: string
  risk: string
  created_at: number
}

/** 会话目标（目标模式）：每轮结束自动校验，未达成携带下一步动作自动续跑。 */
export interface SessionGoal {
  text: string
  status: 'active' | 'paused' | 'done' | string
  round: number
  max_rounds: number
  next_step?: string
  done_because?: string
  updated_at: number
}

/** Provider 熔断/就绪状态（GET /api/v1/ai-provider/circuit-status）。 */
export interface CircuitState {
  id: string
  name: string
  enabled: boolean
  state: 'CLOSED' | 'OPEN' | 'HALF_OPEN' | string
  consecutive_failures: number
  retry_in_ms: number
  /** 未就绪原因（apiKey 缺失/解密失败/kind 未知等）；就绪时为空。 */
  reason?: string
}

/** 模型配置创建/更新请求体。 */
export interface AiProviderReq {
  name: string
  kind: string
  api_key?: string
  base_url?: string
  model: string
  alias?: string | null
  tier?: string
  enabled?: boolean
  context_window?: number | null
  max_output_tokens?: number | null
  compress_ratio?: number | null
  capabilities_json?: string | null
  pricing_json?: string | null
  temperature?: number | null
  top_p?: number | null
  thinking_effort?: string | null
  thinking_style?: string | null
  supports_tool_call?: boolean | null
  supports_vision?: boolean | null
  supports_reasoning?: boolean | null
}

/** 联网搜索配置（DuckDuckGo，无需 api_key；仅 enabled 生效）。 */
export interface WebSearchConfig {
  enabled: string
  engine: string
  api_key: string
}

/** 文件（GET /api/v1/files/search 返回 FileRESP）。 */
export interface FileInfo {
  id: string
  name: string
  original_name: string
  file_type: string
  mime_type: string
  size: number
  status: string
  session_id: string | null
  folder_id: string | null
  created_at: number
}

// ===== 本机会话令牌（无账号体系）=====

/**
 * {@code GET /api/v1/auth/session} 返回结构。
 *
 * <p>WorkBaby 是本机单用户桌面助手：没有注册 / 登录 / 刷新令牌。
 */
export interface SessionToken {
  access_token: string
  token_type: string
  /** 0 = 不过期（随后端进程生命周期）。 */
  expires_in: number
  user_id: string
  username: string
}

// ===== 斜杠命令 =====

/** 斜杠命令元数据（GET /api/v1/chat/commands）。 */
export interface SlashCommand {
  name: string
  args: string
  desc: string
  group: 'session' | 'model' | 'agent' | 'system' | 'custom' | string
  client_only: boolean
  /** 自定义命令的提示词模板（group=custom；选中即灌入输入框，`$ARGUMENTS`/`$1..$n` 发送前展开）。 */
  prompt?: string
  /** 命令来源：builtin（内置）/ user（设置页）/ file（{home}/commands/*.md）。 */
  source?: string
}

/** 命令列表出参。 */
export interface CommandListRESP {
  items: SlashCommand[]
  total: number
}

/** 会话压缩结果（POST /api/v1/chat/sessions/:id/compact）。 */
export interface CompactResultRESP {
  session_id: string
  compacted: number
  freed_chars: number
  kept_recent: number
  total_before: number
  /** 保留指示是否已钉进上下文（Instructions 非空且落库成功） */
  pinned: boolean
  /** 估算释放的 token（rune/4 近似） */
  freed_tokens: number
  /** 单条更新失败数；>0 表示部分压缩失败 */
  failed: number
}

/** 会话压缩入参。 */
export interface CompactREQ {
  instructions?: string
  keep_recent?: number
}

// ===== 上下文占用 =====

/** 上下文占用分段。 */
export interface ContextSegment {
  key: 'system' | 'memory' | 'tools' | 'history' | string
  title: string
  tokens: number
  /** 占上下文窗口的千分比（避免前端浮点误差） */
  ratio: number
}

/** 会话上下文占用快照。 */
export interface ContextUsageRESP {
  session_id: string
  model: string
  context_window: number
  /** 压缩触发预算（窗口 × 压缩比例；0 = 未启用） */
  context_budget: number
  used_tokens: number
  free_tokens: number
  used_ratio: number
  segments: ContextSegment[]
  message_count: number
  tool_count: number
  /** true 表示历史段为字符近似估算（未拿到 provider 实测 input_tokens） */
  estimated: boolean
}

// ===== 工作区文件树（真实磁盘目录，懒加载） =====

/** 工作区单层目录条目（GET /api/v1/chat/workspace/:id/ls）。 */
export interface WorkspaceEntry {
  /** 相对工作区根，slash 分隔；目录以 / 结尾便于判型 */
  path: string
  name: string
  is_dir: boolean
  size: number
  ext: string
  kind: string
}

/** 单层列目录响应。 */
export interface WorkspaceListRESP {
  items: WorkspaceEntry[]
  truncated: boolean
  /** 工作区根绝对路径（「添加到聊天」拼绝对路径引用用）。 */
  root: string
}

// ===== 工作目录信任 =====

export type TrustState = 'allow' | 'ask' | 'deny'

/** 单条信任登记。 */
export interface TrustEntry {
  path: string
  state: TrustState
  updated_at: number
}

/** 单目录信任解析结果。Source 为实际命中的祖先目录（可与入参不同）。 */
export interface TrustResolveRESP {
  path: string
  state: TrustState
  source: string
  matched: boolean
}

// ===== Todo（计划工具共享状态） =====

/** 单条计划项（todo 工具的共享状态；前端进度卡 = state 快照 + chat:todo 事件）。 */
export interface TodoItem {
  id: string
  title: string
  done: boolean
}

/** 会话计划快照。 */
export interface TodoStateRESP {
  session_id?: string
  items: TodoItem[]
  done_count: number
  total: number
}

// ===== 文件变更 =====

/** 文件变更动作（与后端 domain.ChangeAction 对齐）。 */
export type FileChangeAction = 'create' | 'modify' | 'delete'

/** 单条变更（GET /api/v1/chat/sessions/:id/changes 列表项）。 */
export interface FileChange {
  id: string
  session_id: string
  run_id: string
  tool_name: string
  rel_path: string
  action: FileChangeAction
  bytes_before: number
  bytes_after: number
  added_lines: number
  removed_lines: number
  rolled_back: boolean
  created_at: number
}

/** 单条变更详情（GET /api/v1/chat/changes/:cid）。 */
export interface FileChangeDetail extends FileChange {
  diff: string
  before_text: string
  after_text: string
  truncated: boolean
}

/** 变更列表出参。 */
export interface FileChangeListRESP {
  items: FileChange[]
  total: number
}

// ===== 工件 =====

/** 工件类别。 */
export type ArtifactKind = 'image' | 'doc' | 'code' | 'data' | 'file' | string

/** 工件条目（GET /api/v1/chat/sessions/:id/artifacts）。 */
export interface Artifact {
  id: string
  session_id: string
  run_id: string
  kind: ArtifactKind
  name: string
  rel_path: string
  mime_type: string
  size: number
  created_at: number
  updated_at: number
}

/** 工件列表出参。 */
export interface ArtifactListRESP {
  items: Artifact[]
  total: number
}

// ===== 后台任务 =====

/** 任务状态（与后端 domain.TaskState 对齐）。 */
export type TaskState = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

/** 单条后台任务（GET /api/v1/tasks）。 */
export interface BackgroundTask {
  id: string
  session_id: string
  agent: string
  prompt: string
  state: TaskState
  run_id: string
  result: string
  error: string
  created_at: number
  started_at: number
  finished_at: number
}

/** 后台任务列表出参。 */
export interface TaskListRESP {
  items: BackgroundTask[]
  total: number
}

/** 后台任务提交请求（POST /api/v1/tasks）。 */
export interface TaskSubmitREQ {
  session_id: string
  agent: string
  prompt: string
}

// ===== 审批恢复 =====

/** 未决审批快照（GET /api/v1/chat/approvals/pending；刷新页面后恢复）。 */
export interface ApprovalPending {
  id: string
  run_id: string
  session_id: string
  command: string
  reason: string
  risk: 'needs_approval' | 'irreversible' | string
  expires_at: number
  /** 是否允许「本会话允许」（不可逆操作恒 false）。 */
  can_remember?: boolean
}

// ===== Skills =====

/** Skill（GET /api/v1/skills 返回 SkillRESP）。 */
/** Skill 内置脚本（SKILL.md 包 scripts/ 目录；run_skill_script 工具执行）。 */
export interface SkillScript {
  name: string
  language: string
  code: string
}

export interface Skill {
  id: string
  name: string
  description: string | null
  when_to_use: string | null
  body: string
  allowed_tools: string[] | null
  scripts: SkillScript[] | null
  source_kind: string | null
  enabled: boolean
  created_at: number
  updated_at: number
}

/** Skill 创建/更新请求体。 */
export interface SkillReq {
  name: string
  description?: string
  when_to_use?: string
  body: string
  allowed_tools?: string[]
  scripts?: SkillScript[]
  enabled?: boolean
}

/** 技能包 zip 导入结果（POST /api/v1/skills/import-zip）。 */
export interface SkillZipResult {
  imported: string[]
  skipped: string[]
  failed: Array<{ path: string; error: string }>
}

// ===== Agent Profiles（自定义子智能体） =====

/** 自定义子智能体（GET /api/v1/agent-profiles 返回 AgentProfileRESP）。 */
export interface AgentProfile {
  id: string
  name: string
  description: string | null
  system_prompt: string | null
  tools_allow: string[] | null
  tools_deny: string[] | null
  memory_enable: boolean
  max_turns: number
  /** 固定使用的模型名；空 = 继承主 Agent 当前模型（模型名在会话的 Provider 上解析）。 */
  model?: string
  /** 推理强度 off/low/medium/high；空 = 跟随请求级与 Provider 设置。仅在指定了 model 时生效。 */
  thinking?: string
  enabled: boolean
  created_at: number
  updated_at: number
}

/** 自定义子智能体创建/更新请求体（POST /api/v1/agent-profiles，按 name upsert）。 */
export interface AgentProfileReq {
  name: string
  description?: string
  system_prompt?: string
  tools_allow?: string[]
  tools_deny?: string[]
  memory_enable?: boolean
  max_turns?: number
  model?: string
  thinking?: string
  enabled?: boolean
}

// ===== User Commands（自定义斜杠命令） =====

/** 自定义斜杠命令（GET /api/v1/chat/commands/custom）。 */
export interface UserCommand {
  id: string
  name: string
  prompt: string
  description: string | null
  created_at: number
  updated_at: number
}

/** 自定义斜杠命令创建/更新请求体（按 name upsert）。 */
export interface UserCommandReq {
  name: string
  prompt: string
  description?: string
}

// ===== MCP Servers =====

/** MCP Server（GET /api/v1/mcp/servers 返回 McpServerRESP）。 */
export interface McpServer {
  id: string
  name: string
  transport: string
  command: string | null
  args: string[] | null
  env: Record<string, string> | null
  base_url: string | null
  enabled: boolean
  tool_count: number
  ready: boolean
  error: string | null
  created_at: number
  updated_at: number
}

/** MCP Server 创建/更新请求体。 */
export interface McpServerReq {
  name: string
  transport?: string
  command?: string
  args?: string[]
  env?: Record<string, string>
  base_url?: string
  enabled?: boolean
}

// ===== Tasks =====

/** 任务项（MVP 仅列近期会话作为"任务快照"）。 */
export interface TaskItem {
  id: string
  name: string
  status: string
  message_count: number
  last_message_at: number | null
  created_at: number
}

// ===== Tools =====

/** 工具描述（仅展示元数据；Agent 调用走 /chat/stream）。 */
export interface ToolInfo {
  name: string
  group: string
  description: string
}

/**
 * 前端审批队列项。
 *
 * 与后端 ApprovalPending 的差别：流式事件只带决策必需字段（id / 命令 / 风险），
 * 不含 run_id / expires_at 等审计信息；队列统一用这个较窄的形状承载，
 * 启动恢复时用后端列表项填充（结构兼容）。
 */
export interface PendingApproval {
  id: string
  command: string
  reason: string
  risk: string
  can_remember?: boolean
}

/** 单条长期记忆（GET /api/v1/memory/list）。 */
export interface MemoryEntry {
  /** 分节：用户偏好 / 事实 / 流程 / 项目约定 */
  section: string
  text: string
  /** 仅检索结果带分数 */
  score?: number
}

/** 分节计数。 */
export interface MemorySection {
  section: string
  count: number
}

/** 记忆概览（GET /api/v1/memory）。 */
export interface MemoryOverview {
  /** MEMORY.md 路径（设置页「打开文件」用） */
  file: string
  entries: number
  sections: MemorySection[]
}

/** 记忆条目列表。 */
export interface MemoryList {
  file: string
  items: MemoryEntry[]
}

/** 追加一条记忆。 */
export interface MemoryWriteREQ {
  section: string
  content: string
}

/** 删除一条记忆（分节 + 正文精确匹配）。 */
export interface MemoryDeleteREQ {
  section: string
  text: string
}

/** GenUI 节点类型。 */
export type UiNodeKind =
  | 'stack' | 'grid' | 'row'
  | 'heading' | 'text' | 'divider'
  | 'badge' | 'card' | 'stat' | 'progress' | 'markdown' | 'code-block'
  | 'image'
  | 'button' | 'input'

/** GenUI 节点（递归）。 */
export interface GenUiNode {
  node_id: string
  kind: string
  props?: Record<string, unknown>
  children?: GenUiNode[]
}

/** GenUI 完整树。 */
export interface GenUiTree {
  schema_version: string
  root: GenUiNode
}

/** 文件夹。 */
export interface Folder {
  id: string
  name: string
  parent_id: string | null
  path: string
  description: string | null
  workspace_id: string | null
  child_count: number
  created_at: number
  updated_at: number
}

/** 文件夹树节点。 */
export interface FolderTreeNode {
  folder: Folder
  children: FolderTreeNode[]
}

/** 创建/更新文件夹请求。 */
export interface FolderReq {
  name: string
  parent_id?: string | null
  description?: string | null
  workspace_id?: string | null
}

/** 知识文档。 */
export interface KnowledgeDoc {
  id: string
  folder_id: string | null
  name: string
  source: string
  source_type: string
  mime: string
  size_bytes: number
  chunk_count: number
  status: string
  error_msg: string | null
  created_at: number
  updated_at: number
}

/** 创建/更新文档请求。 */
export interface KnowledgeDocReq {
  folder_id?: string | null
  name: string
  source: string
  source_type: string
}

// ===== Cron =====

/** 定时任务（GET /api/v1/cron/jobs）。 */
export interface CronJob {
  id: string
  name: string
  schedule: string
  workflow_id: string | null
  action: string
  enabled: boolean
  last_run_at: number | null
  last_status: string | null
  next_run_at: number | null
  created_at: number
  updated_at: number
}

/** 定时任务写入请求（后端 CronJobREQ：schedule + workflow_id）。 */
export interface CronJobReq {
  name: string
  schedule: string
  workflow_id?: string | null
  enabled?: boolean
}

// ===== 桌宠 =====

/** 桌宠配置（GET /api/v1/pet/config）。 */
export interface PetConfig {
  user_id: string
  enabled: boolean
  mode: string
  sprite_id: string | null
  position_x: number
  position_y: number
  scale: number
  bubble_enabled: boolean
  bubble_duration_ms: number
  /** 同一 sprite 复用为聊天区背景（与桌宠共享形象来源） */
  chat_background: boolean
  /** 背景不透明度百分比（0~100） */
  background_opacity: number
  /** 背景模糊像素（0 = 不模糊） */
  background_blur_px: number
  /** 桌宠形态下透明区域点击穿透（Windows 原生窗口区域裁剪） */
  click_through: boolean
  updated_at: number | null
}

/** 桌宠配置写入请求。 */
export interface PetConfigReq {
  enabled?: boolean
  mode?: string
  sprite_id?: string | null
  position_x?: number
  position_y?: number
  scale?: number
  bubble_enabled?: boolean
  bubble_duration_ms?: number
  chat_background?: boolean
  background_opacity?: number
  background_blur_px?: number
  click_through?: boolean
}

/** 桌宠 sprite（GET /api/v1/pet/sprites）。 */
export interface PetSprite {
  id: string
  name: string
  file_path: string | null
  format: string | null
  frame_count: number | null
  is_builtin: boolean
  created_at: number
}

/** 桌宠 sprite 写入请求。 */
export interface PetSpriteReq {
  name: string
  file_path?: string | null
  format?: string | null
  frame_count?: number | null
  is_builtin?: boolean
}

// ===== Channels =====

/** 通道类型（桌面单机：控制台 / 邮件 / webhook）。 */
export type ChannelType = 'CONSOLE' | 'EMAIL' | 'WEBHOOK'

/** 通道配置（GET /api/v1/channels）。 */
export interface ChannelConfig {
  id: string
  user_id: string | null
  channel_type: string
  enabled: boolean
  config_json: string | null
  webhook_url: string | null
  status: string
  last_error: string | null
  last_active_at: number | null
  created_at: number
  updated_at: number
}

/** 通道配置写入请求。 */
export interface ChannelConfigReq {
  channel_type: string
  enabled?: boolean
  config_json?: string | null
  webhook_url?: string | null
  /** 邮箱通道：收件人地址（结构化字段，最终组装为 config_json）。 */
  email_to?: string
  /** 邮箱通道：发件人显示名（结构化字段）。 */
  email_display_name?: string
}

/** 通道消息日志（GET /api/v1/channels/:id/messages）。 */
export interface ChannelMessageLog {
  id: string
  channel_id: string
  direction: string
  message_type: string | null
  user_external_id: string | null
  session_id: string | null
  content_summary: string | null
  status: string | null
  error_message: string | null
  created_at: number
}

// ===== Admin / Docs =====

/** 内置文档条目（GET /api/v1/docs）。 */
export interface DocItem {
  name: string
  title: string
}

/** 内置文档详情（GET /api/v1/docs/:name）。 */
export interface DocDetail {
  name: string
  title: string
  content: string
}

/** 管理后台概览（GET /api/v1/admin/overview）。 */
export interface AdminOverview {
  version: string
  phase: string
  home: string
  db_enabled: boolean
  tool_count: number
  providers: number
  sessions: number
  messages: number
  memory_episodes: number
  knowledge_docs: number
  [key: string]: unknown
}
