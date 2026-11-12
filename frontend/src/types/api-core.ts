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

/** 引擎种类。引擎已内置于本进程，保留字段让形状不依赖「只有一个」。 */
export type EngineKind = 'native'

/** 能力表：界面按它隐藏入口，不渲染灰按钮。 */
export interface EngineCaps {
  subagents: boolean
  goals: boolean
  todos: boolean
  queue_clear: boolean
  queue_edit: boolean
  skill_toggle: boolean
  export_html: boolean
  slash_commands: boolean
  session_clone: boolean
}

export interface EngineIdentity {
  engine_kind: EngineKind
  engine_label: string
  engine_exe: string
  engine_script?: string
  engine_source: string
  engine_version: string
  installed: boolean
  caps: EngineCaps
}

export interface EngineLayout {
  engine_kind: EngineKind
  agent_dir: string
  sessions_dir: string
  skills_dir: string
  settings_file: string
  dir_exists: boolean
}

/** 外部依赖的检测状态：引擎在进程内，这里只报用户环境里的工具链。 */
export interface RuntimeStatus {
  node_ready: boolean
  git_ready: boolean
  git_version: string
  python_ready: boolean
  engine_ready: boolean
  detection_hint?: string
  detecting: boolean
}

/** 应用页的一行：一个外部依赖的检测事实与可做的动作。 */
export interface AppRow {
  app_key: string
  app_name: string
  app_version: string
  app_path: string
  installed: boolean
  required: boolean
  app_source: string
  can_locate: boolean
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

/** 一个存活的引擎实例。引擎在进程内，所以没有 PID。 */
export interface EngineRun {
  engine_kind: EngineKind
  running: boolean
  busy: boolean
  session_file: string
  work_dir: string
  started_at: number
}

export interface WorkspaceRow {
  workspace_path: string
  workspace_exists: boolean
  /** 是否处于 git 仓库内 */
  workspace_is_repo?: boolean
  /** 当前分支名；非仓库为空 */
  workspace_branch?: string
  /** 未提交改动数量 */
  workspace_dirty?: number
  /** 该工作区下的会话数 */
  session_count?: number
  updated_at: number
}

/** 工作区清单：当前工作区 + 历史（带仓库与会话概况）。 */
export interface WorkspaceList {
  workspace: string
  workspace_rows: WorkspaceRow[]
  active_engine: string
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

/** 引擎健康快照：计数 + 配置现状，一屏要用的全部事实。 */
export interface EngineHealth {
  engine_kind: EngineKind
  engine_label: string
  engine_version: string
  installed: boolean
  running: boolean
  busy: boolean
  session_count: number
  skill_count: number
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
  total_mcp: number
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
