// 前端与后端的契约类型（模型 / 服务商 / 技能 / 插件 / MCP）。字段名与 Go 侧 json tag 一一对应。

// ---------------- 模型与服务商 ----------------

export interface ModelInfo {
  model_id: string
  model_name: string
  provider_name: string
  api_protocol: string
  supports_reasoning: boolean
  context_window: number
  max_tokens: number
  input_kinds?: string[]
}

export interface ModelEntry {
  model_id: string
  model_name?: string
  api_protocol?: string
  supports_reasoning?: boolean
  context_window?: number
  max_tokens?: number
  input_kinds?: string[]
}

export interface ProviderView {
  provider_name: string
  base_url: string
  api_protocol: string
  auth_kind: string
  /** 掩码后的密钥：界面永远拿不到原文 */
  api_key_masked: string
  has_key: boolean
  models: ModelEntry[]
  model_count: number
}

/** pi 的默认模型与默认思考档：管「新会话从哪个模型起步」，与当前会话无关。 */
export interface ModelDefaults {
  default_provider?: string
  default_model?: string
  default_thinking_level?: string
  thinking_levels?: string[]
  settings_file: string
}

export interface ProvidersView {
  providers: ProviderView[]
  config_path: string
  read_error?: string
}

/** 向上游探测的入参。空字段由后端从已保存的配置里取。 */
export interface ProviderProbeREQ {
  provider_name?: string
  base_url?: string
  api_protocol?: string
  api_key?: string
}

export interface ProviderProbeRESP {
  probe_ok: boolean
  model_count: number
  models?: ModelEntry[]
  probe_message: string
}

/** 对话页的模型下拉：一条可选模型。 */
export interface ModelRow {
  provider_name: string
  model_id: string
  model_name: string
  api_protocol: string
  context_window: number
  max_tokens: number
  supports_reasoning: boolean
  input_kinds?: string[]
}

// ---------------- 技能与插件 ----------------

export type SkillSourceKind = 'builtin' | 'bundled' | 'managed' | 'project'

export interface SkillRow {
  skill_name: string
  skill_dir_name: string
  skill_desc: string
  skill_path: string
  skill_dir: string
  skill_source: SkillSourceKind
  skill_group: string
  skill_requires?: string
  skill_enabled: boolean
  skill_editable: boolean
  skill_modified: boolean
}

export interface SkillListData {
  skills: SkillRow[]
  recommended_total: number
  recommended_installed: number
  available: SkillAvailable[]
}

/** 推荐清单里还没装的一条技能。 */
export interface SkillAvailable {
  skill_dir_name: string
  skill_group: string
  skill_desc: string
  skill_requires?: string
}

export type PluginStateKind = 'installed' | 'disabled' | 'missing' | 'failed'

export interface PluginRow {
  plugin_name: string
  plugin_version: string
  plugin_source: string
  plugin_path: string
  plugin_desc?: string
  plugin_target_version?: string
  plugin_latest?: string
  plugin_upgradable: boolean
  resource_kinds: string[]
  plugin_recommended: boolean
  plugin_declared: boolean
  plugin_enabled: boolean
  plugin_installed: boolean
  plugin_state: PluginStateKind
  plugin_error?: string
}

export interface PluginListData {
  plugins: PluginRow[]
  registry_path: string
  install_root: string
  recommended_total: number
  recommended_installed: number
  busy: boolean
}

// ---------------- MCP ----------------

export interface McpServer {
  server_name: string
  transport_kind: string
  command_line?: string
  server_url?: string
  mcp_desc?: string
  mcp_work_dir?: string
  mcp_env_text?: string
  mcp_timeout?: number
  mcp_enabled: boolean
  scope_kind: string
  config_path: string
}

export interface McpListData {
  servers: McpServer[]
  user_path: string
  project_path?: string
  read_errors?: string[]
}
