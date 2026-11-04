//! 核心视图类型：引擎、应用、偏好、配置原文；与前端 types/api.ts 对齐。

/** 引擎种类：本壳只服务 pi，omp 保留为前端兼容的空壳。 */
export type EngineKind = 'omp' | 'pi'

/** 引擎能力集：UI 按能力隐藏入口，不给就等于没有这个功能。 */
export interface WbCaps {
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
  session_clone: boolean
  plugin_toggle: boolean
  plugin_upgrade: boolean
}

/** 引擎身份：安装状态与来源渠道。 */
export interface EngineIdentityVO {
  kind: EngineKind
  exe: string
  version: string
  source: 'custom' | 'path' | ''
  channel: string
  caps: WbCaps
}

/** 应用渠道。 */
export type AppChannel = 'npm' | 'pnpm' | 'bun' | 'wb-installer' | 'pi-installer' | 'unknown' | ''

/** 应用列表行：pi 本体与内置运行时都以行形式出现。 */
export interface AppRowVO {
  kind: EngineKind
  app_name: string
  installed: boolean
  version: string
  exe: string
  source: string
  channel: AppChannel
  channel_label: string
  latest_version: string
  latest_note?: string
  outdated: boolean
  install_command: string
  update_command: string
  uninstall_command: string
  docs_url: string
  can_install: boolean
  can_update: boolean
  can_uninstall: boolean
  blocker?: string
  busy: boolean
  update_preview?: string
  uninstall_preview?: string
}

/** 引擎目录布局。 */
export interface EngineLayoutVO {
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

/** 存活引擎实例。 */
export interface RunRowVO {
  kind: EngineKind
  work_dir: string
  pid: number
  session_id: string
  running: boolean
  protocol: string
  started_at: number
  call_count: number
  last_error?: string
  busy: boolean
}

/** 引擎安装态信息（含布局）。 */
export interface EngineInfoVO {
  kind: EngineKind
  exe: string
  version: string
  source: 'custom' | 'path' | ''
  channel: string
  caps: WbCaps
  installed: boolean
  layout: EngineLayoutVO
}

/** 引擎状态总览。 */
export interface EngineStatusVO {
  engines: EngineInfoVO[]
  running: RunRowVO[]
  install: Record<string, string>
  detecting: boolean
  detected_at: number
}

/** 单引擎健康详情。 */
export interface EngineHealthVO {
  engine_kind: EngineKind
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
  protocol: string
  started_at: number
  call_count: number
  last_error?: string
  session_count: number
  skill_count: number
  plugin_count: number
  mcp_count: number
  session_count_known: boolean
  skill_count_known: boolean
  plugin_count_known: boolean
  mcp_count_known: boolean
  layout_resolved: boolean
  probe_unknown_reason?: string
  config_files: ConfigFileVO[]
  root_dir: string
  agent_dir: string
  dir_exists: boolean
  caps: Record<string, boolean>
}

/** 全局健康面板。 */
export interface HealthDataVO {
  engines: EngineHealthVO[]
  active_kind: EngineKind
  workspace: string
  refreshing: boolean
  detecting: boolean
  ready: boolean
  detected_at: number
  total_sessions: number
  total_skills: number
  total_plugins: number
  total_mcp: number
  totals_partial: boolean
  live_engines: number
  max_engines: number
  engine_runs: EngineRunVO[]
}

/** 引擎运行实例详情。 */
export interface EngineRunVO {
  engine_kind: EngineKind
  engine_label: string
  work_dir: string
  process_id: number
  protocol_version: string
  started_at: number
  call_count: number
  last_error?: string
  busy: boolean
  session_id: string
}

/** 启动总包：前端第一屏的全部依据。 */
export interface BootstrapVO {
  version: string
  engines: EngineIdentityVO[]
  caps: Record<EngineKind, WbCaps>
  active: EngineKind
  workspace: string
  profile: string
  settings: Record<string, string>
  layouts: Record<string, EngineLayoutVO>
  workspaces: WorkspaceRowVO[]
  skills: Record<string, number>
  sessions: Record<string, number>
  install: Record<string, string>
}

/** 工作区历史行。 */
export interface WorkspaceRowVO {
  workspace_path: string
  exists: boolean
  updated_at?: string
}

/** 应用偏好。 */
export interface AppPrefsVO {
  values: Record<string, string>
  auto_start: boolean
  restore_session: boolean
  last_session_file: string
  last_session_title?: string
  last_engine_kind: EngineKind | ''
  data_dir: string
  log_dir: string
  close_action: 'tray' | 'quit'
  tray_hint_pending: boolean
}

/** 配置文件条目。 */
export interface ConfigFileVO {
  file_path: string
  file_label: string
  exists: boolean
  size_bytes: number
}

/** 配置文件清单。 */
export interface ConfigFileListVO {
  engine_kind: EngineKind | ''
  entries: ConfigFileVO[]
}

/** 配置原文。 */
export interface ConfigTextVO {
  file_path: string
  text: string
  truncated: boolean
  found: boolean
}

/** pi 引擎的完整能力集。 */
export const PI_CAPS: WbCaps = {
  subagents: false,
  goals: false,
  todos: false,
  login: false,
  fragments: false,
  queue_edit: false,
  queue_clear: true,
  live_steer_count: false,
  skill_toggle: true,
  extensions: true,
  export_html: true,
  slash_commands: true,
  session_clone: true,
  plugin_toggle: true,
  plugin_upgrade: true,
}

/** omp 空能力：没有引擎就没有任何功能。 */
export const EMPTY_CAPS: WbCaps = {
  subagents: false,
  goals: false,
  todos: false,
  login: false,
  fragments: false,
  queue_edit: false,
  queue_clear: false,
  live_steer_count: false,
  skill_toggle: false,
  extensions: false,
  export_html: false,
  slash_commands: false,
  session_clone: false,
  plugin_toggle: false,
  plugin_upgrade: false,
}
