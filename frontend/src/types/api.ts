// 与后端 domain 类型的 serde json 字段逐字一致；字段一律 snake_case 且带业务前缀。

import type { DiffLine } from './chatEvents'

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
  /** 用户消息显式调用的技能名：回放时渲染成技能芯片 */
  message_skill?: string | null
  /** 用户消息携带的图片 */
  message_images?: MessageImageVO[]
  /** 工具改动的文件（相对工作区）；只有写文件类工具有 */
  message_file_path?: string
  /** 工具改动的差异行 */
  message_diff?: DiffLine[]
}

/** 消息携带的图片（纯 base64，界面拼 data URL 渲染） */
export interface MessageImageVO {
  image_mime: string
  image_base64: string
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
  /** 会话树：主干 + 挂在主干上的分支 */
  detail_tree?: SessionTreeNodeVO
}

/** 会话树节点。 */
export interface SessionTreeNodeVO {
  node_entry_id: string
  node_parent_entry_id?: string | null
  node_preview: string
  node_is_user: boolean
  /** 是否在当前路径上 */
  node_on_path: boolean
  node_children: SessionTreeNodeVO[]
}

export interface SendMessageRESP {
  resp_run_id: string
  resp_session_id: string
  resp_queued: boolean
}

/** 引擎抛回来的问题（confirm / select / input / editor） */
/** 问卷单题（method=questionnaire 时出现）。 */
export interface AskQuestion {
  question_title: string
  question_options: AskOption[]
  question_multi: boolean
}

/** 问卷选项。 */
export interface AskOption {
  option_label: string
  option_description: string
  option_preview?: string | null
}

export interface AskData {
  id: string
  method: string
  title: string
  message: string
  options: string[]
  placeholder: string
  prefill: string
  /** 问卷（method=questionnaire）：一次问多题，答案按行回传 */
  questions?: AskQuestion[]
  /** 等待上限（毫秒）：0 表示不限时 */
  timeout_ms: number
  /** 安全闸门的审批卡：被审的工具名 */
  approval_tool?: string
  /** 审批卡上要展示的那条命令 / 路径 */
  approval_summary?: string
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
  /** 提示词缓存是否开（Anthropic 系的 cache_control 断点） */
  provider_cache: boolean
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
  req_provider_cache?: boolean
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
  /** 有可配项时界面给配置入口 */
  feature_configurable: boolean
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

export * from './featurePanel'
export * from './providerPreset'
export * from './git'
