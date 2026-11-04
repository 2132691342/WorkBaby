// 前端与后端的契约类型（引擎 / 引导 / SSE 基础）。字段名与 Go 侧 json tag 一一对应。
// 改字段形态必须同时把 backend/service.ContractVersion +1。

export interface Envelope<T> {
  /** 业务码（数字字符串），"0" 即成功 */
  code: string
  /** 人话描述，界面直接显示 */
  message: string
  /** 补充上下文，只进诊断面板 */
  detail?: string
  /** 载荷 */
  data: T
}

/** 引擎种类。本壳只装一个引擎，但保留这个字段让形状不依赖「只有一个」。 */
export type EngineKind = 'pi'

/** 能力表：界面按它隐藏入口，不渲染灰按钮。 */
export interface EngineCaps {
  subagents: boolean
  goals: boolean
  todos: boolean
  login: boolean
  queue_clear: boolean
  queue_edit: boolean
  skill_toggle: boolean
  extensions: boolean
  export_html: boolean
  slash_commands: boolean
  session_clone: boolean
  plugin_toggle: boolean
  plugin_upgrade: boolean
}

export interface EngineIdentity {
  engine_kind: EngineKind
  engine_label: string
  engine_exe: string
  engine_version: string
  installed: boolean
  caps: EngineCaps
}

export interface EngineLayout {
  engine_kind: EngineKind
  root_dir: string
  agent_dir: string
  sessions_dir: string
  skills_dir: string
  plugins_dir: string
  settings_file: string
  dir_exists: boolean
}

/** 内置运行时与 pi 本体的就绪状态。 */
export interface RuntimeStatus {
  node_ready: boolean
  node_version: string
  git_ready: boolean
  git_version: string
  python_ready: boolean
  python_version: string
  pi_ready: boolean
  pi_version: string
  provisioning: boolean
}

/** 应用页的一行：一个运行时 / 引擎的安装事实与可做的动作。 */
export interface AppRow {
  app_key: string
  app_name: string
  app_version: string
  app_path: string
  installed: boolean
  builtin: boolean
  can_install: boolean
  can_update: boolean
  can_uninstall: boolean
  busy: boolean
  blocker?: string
  install_note?: string
}

/** 安装进度帧（SSE `app:progress`）。逐行推，前端 append 到日志面板。 */
export interface AppProgress {
  progress_kind: string
  progress_action: string
  progress_phase: 'start' | 'line' | 'done' | 'fail'
  progress_line?: string
  progress_message?: string
  plugin_name?: string
}

/** 一个存活的引擎进程。 */
export interface EngineRun {
  pid: number
  running: boolean
  busy: boolean
  session_file: string
  work_dir: string
  started_at: number
  call_count: number
  last_error?: string
}

export interface WorkspaceRow {
  workspace_path: string
  workspace_exists: boolean
  updated_at: number
}

/** 设置页看到的全部开关状态。自动开机自启以注册表为准。 */
export interface AppPrefs {
  pref_values: Record<string, string>
  auto_start: boolean
  restore_session: boolean
  last_session_file: string
  close_action: 'tray' | 'quit'
  data_dir: string
  log_dir: string
}

export interface ConfigFile {
  config_name: string
  config_label: string
  file_path: string
  file_exists: boolean
  size_bytes: number
}

/** 引擎配置原文。found 为假表示「还没生成过」，不是错误。 */
export interface ConfigText {
  file_path: string
  file_text: string
  truncated: boolean
  found: boolean
}

export interface Bootstrap {
  app_version: string
  app_contract: number
  engines: EngineIdentity[]
  active_engine: EngineKind
  workspace: string
  settings: Record<string, string>
  layouts: EngineLayout[]
  workspaces: WorkspaceRow[]
  runtime: RuntimeStatus
  prefs: AppPrefs
}

/** 引擎健康快照：探测 + 进程 + 落盘计数 + 配置现状，一屏要用的全部事实。 */
export interface EngineHealth {
  engine_kind: EngineKind
  engine_label: string
  engine_exe: string
  engine_version: string
  installed: boolean
  running: boolean
  busy: boolean
  process_id: number
  started_at: number
  call_count: number
  last_error?: string
  session_count: number
  skill_count: number
  plugin_count: number
  mcp_count: number
  config_files: ConfigFile[]
  caps: EngineCaps
}

/** 概览页首屏要用的全部事实。 */
export interface HealthData {
  health_engines: EngineHealth[]
  active_engine: EngineKind
  workspace: string
  runtime: RuntimeStatus
  total_sessions: number
  total_skills: number
  total_plugins: number
  total_mcp: number
  live_engines: number
  memory_used_mb: number
  detected_at: number
}

export interface EngineStatus {
  engines: EngineIdentity[]
  layouts: EngineLayout[]
  runtime: RuntimeStatus
  instance: EngineRun
  detected_at: number
}

/** 一条 SSE 帧。seq 由 hub 单调发号，断线按它补帧。 */
export interface StreamFrame {
  seq: number
  event: string
  data: Record<string, unknown>
  sent_at: number
}
