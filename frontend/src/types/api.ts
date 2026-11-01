// 与后端 domain 类型的 serde json 字段逐字一致；字段一律 snake_case 且带业务前缀。

export interface Resp<T> {
  /** 业务码是数字字符串（"0" 成功）；与 HTTP 状态码的 int 从类型上分开 */
  code: string
  message: string
  data: T
}

// ---- 会话与消息 ----

export interface UsageVO {
  usage_input: number
  usage_output: number
  /** 命中上游缓存的输入量（与 usage_input 并列） */
  usage_cached: number
  /** 写入上游缓存的输入量 */
  usage_cache_write: number
  /** 总量 */
  usage_total: number
  /** 这一轮发出时的上下文窗口占用 */
  usage_context: number
  /** 成本（美元） */
  usage_cost: number
}

export interface ToolCallVO {
  tool_call_id: string
  tool_name: string
  tool_label?: string
  tool_args?: Record<string, unknown>
}

export interface MessageBlock {
  /** text = 正文；thinking = 思考；tool = 工具调用（结果在紧随其后的 tool 消息里） */
  block_kind: string
  block_text?: string
  tool_call_id?: string
  tool_name?: string
  tool_label?: string
  tool_args?: Record<string, unknown>
}

export interface MessageVO {
  message_id: string
  /** 真实会话条目 id（ENT_ 前缀）：分支点用它；乐观插入的消息目前还没有 */
  message_entry_id?: string
  message_role: 'user' | 'assistant' | 'tool' | 'notice'
  /** 条目类型：text / tool / notice */
  message_type: string
  message_thinking?: string
  message_content?: string
  /** 正文 / 思考 / 工具调用的原始顺序；渲染按它来，不按字段类型分组 */
  message_blocks: MessageBlock[]
  message_tool_calls: ToolCallVO[]
  message_tool_call_id?: string
  message_tool_name?: string
  message_is_error?: boolean
  /** 仅 assistant：stop / length / tool_use / aborted / error */
  message_stop_reason?: string
  message_usage?: UsageVO
  message_created_at: number
  message_hidden?: boolean
  /** 非空时渲染成分隔提示条（已压缩 / 换模型）而不是气泡 */
  message_notice?: string
}

export interface SessionVO {
  session_id: string
  session_file: string
  session_title: string
  session_workspace: string
  session_provider: string
  session_model: string
  session_thinking_level: string
  session_message_count: number
  session_total_tokens: number
  session_cost: number
  session_running: boolean
  session_created_at: number
  session_updated_at: number
}

export interface SessionDetailRESP {
  detail_session: SessionVO
  detail_messages: MessageVO[]
  /** 当前叶子条目 id */
  detail_leaf_entry_id?: string | null
  /** 可作分支点的用户消息条目 id */
  detail_fork_entry_ids?: string[]
  /** 可切回去的其它分支末端 */
  detail_alt_leaf_ids?: string[]
}

export interface SendMessageRESP {
  resp_run_id: string
  resp_session_id: string
  resp_queued: boolean
}

/** @ 引用：相对工作区的路径；粘贴的图片直接带 base64 */
export interface AttachmentREQ {
  attach_path: string
  attach_name: string
  attach_image_base64?: string
}

/** 引擎抛回来的问题（confirm / select / input / editor） */
export interface AskData {
  id: string
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
  /** answered / cancelled / expired */
  reason: string
}

// ---- 模型服务 ----

export interface ModelVO {
  model_id: string
  model_name: string
  model_reasoning: boolean
  model_vision: boolean
  model_context_window: number
  model_max_tokens: number
}

export interface ProviderVO {
  /** models.json 里的键 */
  provider_id: string
  provider_name: string
  provider_api: string
  provider_base_url: string
  provider_has_key: boolean
  /** 掩码：••••abcd */
  provider_key_hint: string
  provider_models: ModelVO[]
  provider_is_default: boolean
}

export interface ProviderListRESP {
  resp_providers: ProviderVO[]
  resp_default_provider: string
  resp_default_model: string
  resp_config_path: string
}

export interface ModelREQ {
  model_id: string
  model_name: string
  model_reasoning: boolean
  model_vision: boolean
  model_context_window: number
  model_max_tokens: number
}

export interface UpsertProviderREQ {
  req_provider_id?: string
  req_provider_name: string
  req_provider_api: string
  req_provider_base_url: string
  req_provider_api_key?: string
  req_models: ModelREQ[]
}

export interface TestProviderRESP {
  resp_ok: boolean
  resp_model: string
  resp_message: string
}

export interface RevealProviderRESP {
  resp_provider_id: string
  resp_api_key: string
}

export interface AvailableModelVO {
  model_id: string
  model_name: string
  model_provider: string
  model_context_window: number
  model_max_tokens: number
  model_reasoning: boolean
  model_vision: boolean
}

// ---- 技能 ----

export interface SkillVO {
  /** 技能名即身份 */
  skill_name: string
  skill_description: string
  skill_description_zh?: string
  skill_description_en?: string
  skill_version?: string
  /** builtin / user / workspace */
  skill_source: string
  skill_location: string
  skill_enabled: boolean
}

export interface SkillImportRESP {
  resp_imported: number
  resp_skipped: string[]
}

export interface SkillContentRESP {
  resp_content: string
}

// ---- 工具 ----

export interface ToolVO {
  tool_name: string
  tool_label: string
  tool_description: string
  /** builtin / mcp */
  tool_source: string
  tool_enabled: boolean
  tool_read_only: boolean
}

// ---- 内置功能 ----

export interface FeatureVO {
  feature_code: string
  feature_name: string
  feature_description: string
  feature_enabled: boolean
  feature_tools: string[]
  /** p1 / p2 */
  feature_phase: string
}

// ---- MCP ----

export interface MCPServerVO {
  mcp_name: string
  mcp_scope: string
  mcp_source: string
  mcp_enabled: boolean
  mcp_exposure: string
  mcp_transport: string
  /** 连接目标：stdio 为启动命令行，http 为 URL */
  mcp_target: string
  /** connected / error / unknown */
  mcp_state: string
  mcp_error: string
  mcp_tools: string[]
}

export interface MCPListRESP {
  resp_servers: MCPServerVO[]
  resp_global_path: string
  resp_workspace_path: string
  resp_global_raw: string
  resp_workspace_raw: string
}

// ---- 引导与设置 ----

export interface SettingKV {
  setting_key: string
  setting_value: string
}

export interface BootVO {
  boot_version: string
  boot_data_dir: string
  boot_workspace: string
  boot_ready: boolean
  boot_block_reason: string
  boot_default_provider: string
  boot_default_model: string
  boot_settings: Record<string, string>
}

export interface BackgroundVO {
  resp_background_file: string
  resp_background_opacity: number
  resp_background_blur: number
}

// ---- 统计 ----

export interface StatsDailyItem {
  daily_day: string
  daily_input: number
  daily_output: number
  daily_cache_read: number
  daily_cache_write: number
  daily_cost: number
}

export interface StatsModelItem {
  model_provider: string
  model_id: string
  model_input: number
  model_output: number
  model_cost: number
}

export interface StatsRESP {
  resp_daily: StatsDailyItem[]
  resp_by_model: StatsModelItem[]
}

// ---- 帮助文档 ----

export interface HelpDocVO {
  doc_name: string
  doc_title: string
}

export interface HelpDocRESP {
  resp_name: string
  resp_content: string
}

// ---- 工作区文件 ----

export interface FileEntryVO {
  entry_name: string
  entry_path: string
  entry_is_dir: boolean
  entry_size: number
  entry_mtime: number
}

export interface FileBytesRESP {
  /** 文件名（带扩展名，前端据此选渲染器） */
  resp_file_name: string
  resp_mime: string
  /** 文件字节的 base64 */
  resp_content_base64: string
  resp_size: number
}

export interface FileListRESP {
  resp_path: string
  resp_entries: FileEntryVO[]
}

// ---- 斜杠命令 ----

export interface CommandVO {
  command_name: string
  command_description: string
  command_source: string
}

// ---- 上下文分解 ----

export interface ContextSegmentVO {
  segment_name: string
  segment_tokens: number
}

export interface ContextBreakdownRESP {
  resp_segments: ContextSegmentVO[]
  resp_total_tokens: number
  resp_context_window: number
  resp_ratio: number
}

// ---- Git（工作区版本视图）----

export interface GitFileVO {
  git_path: string
  /** M 改过 / A 新增 / D 删除 / R 改名 / C 复制 / T 类型变化 / ? 未跟踪 / U 冲突 */
  git_status: string
  git_staged: boolean
  /** 该文件有未解决冲突；解决后要显式标记，否则 git 不接受提交 */
  git_conflict: boolean
  git_additions: number
  git_deletions: number
}

export interface GitRemoteVO {
  git_remote_name: string
  git_remote_url: string
}

export interface GitStatusVO {
  git_available: boolean
  git_reason: string
  git_is_repo: boolean
  git_workspace: string
  /** 仓库根；工作区是仓库子目录时与 workspace 不同 */
  git_repo_root: string
  git_branch: string
  /** 上游引用（如 origin/main）；没有上游为空 */
  git_upstream: string
  git_ahead: number
  git_behind: number
  /** none / merge / rebase / cherry-pick / revert */
  git_state: string
  git_state_detail: string
  git_remotes: GitRemoteVO[]
  git_files: GitFileVO[]
  git_conflict: boolean
  git_version: string
}

export interface GitDiffLineVO {
  git_line_kind: string
  git_line_old_no: number
  git_line_new_no: number
  git_line_text: string
}

export interface GitDiffVO {
  git_diff_path: string
  git_truncated: boolean
  git_dropped: number
  git_lines: GitDiffLineVO[]
}

export interface GitCommitVO {
  git_hash: string
  git_short: string
  git_subject: string
  git_author: string
  /** 提交时间（毫秒） */
  git_when: number
  git_files: number
  git_additions: number
  git_deletions: number
}

export interface GitCommitDetailVO {
  git_hash: string
  git_short: string
  git_subject: string
  git_body: string
  git_author: string
  git_when: number
  git_parents: string[]
  git_files: GitFileVO[]
  /** 单文件详情时附带的 diff */
  git_diff?: GitDiffVO
}

export interface GitBranchVO {
  git_name: string
  git_current: boolean
  /** 远端分支所属远端名；本地分支为空 */
  git_remote: string
  git_ahead: number
  git_behind: number
}

/** 推送的强制级别。默认 none，逐级需要更多确认 */
export type GitForce = 'none' | 'lease' | 'force'

export interface GitPushREQ {
  git_remote?: string
  git_branch?: string
  /** 把这次推送的远端建立为上游（首次推送必须） */
  git_set_upstream?: boolean
  git_force?: GitForce
  /** 强制推送时必须与分支名完全一致的分支名 */
  git_confirm?: string
  git_session_id?: string
}

export interface GitPullREQ {
  git_remote?: string
  git_branch?: string
  git_rebase?: boolean
  git_session_id?: string
}

export interface GitCommitRESP {
  resp_status: GitStatusVO
  resp_hash: string
}

export interface GitMessageRESP {
  resp_message: string
  resp_err: string
}
