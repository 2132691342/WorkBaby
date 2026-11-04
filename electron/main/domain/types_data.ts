//! 数据域类型：会话、文件、git、模型、技能、插件、MCP、文档；与前端 types/api.ts 对齐。

/** 会话列表行。 */
export interface SessionRowVO {
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

/** 会话消息条目（详情页兜底渲染用）。 */
export interface SessionItemVO {
  kind: 'user' | 'assistant' | 'tool' | 'system'
  role: string
  text?: string
  thinking?: string
  model?: string
  tool_name?: string
  tool_call_id?: string
  is_error?: boolean
  timestamp?: string
  cost?: number
  tokens?: number
}

/** 会话详情。 */
export interface SessionDetailVO extends SessionRowVO {
  thinking: boolean
  items: SessionItemVO[]
}

/** 会话导出。 */
export interface SessionExportVO {
  file_name: string
  content: string
}

// ---- 模型与服务商 ----

/** 模型行。 */
export interface ModelRowVO {
  kind: 'omp' | 'pi'
  provider: string
  id: string
  name: string
  api: string
  context_window: number
  max_tokens: number
  reasoning: boolean
  input?: string[]
}

/** 模型条目（服务商配置内）。 */
export interface ModelEntryVO {
  model_id: string
  model_name?: string
  api?: string
  reasoning?: boolean
  input?: string[]
  context_window?: number
  max_tokens?: number
}

/** 服务商视图。 */
export interface ProviderViewVO {
  engine_kind: 'omp' | 'pi'
  provider_name: string
  base_url: string
  api: string
  auth: string
  api_key: string
  has_key: boolean
  models: ModelEntryVO[]
}

/** 服务商总览。 */
export interface ProvidersViewVO {
  providers: ProviderViewVO[]
  config_path: string
}

/** 服务商探测请求。 */
export interface ProviderProbeREQ {
  engine_kind?: 'omp' | 'pi'
  provider_name?: string
  base_url?: string
  api?: string
  api_key?: string
  auth?: string
}

/** 探测结果。 */
export interface ProviderProbeRESP {
  models: ModelEntryVO[]
  config_path: string
}

/** 连通性测试结果。 */
export interface ProviderTestRESP {
  ok: boolean
  model_count: number
  message: string
}

/** 登录服务商。 */
export interface LoginProviderVO {
  id: string
  name: string
  available: boolean
  authenticated: boolean
}

// ---- 技能 ----

/** 技能行。 */
export interface SkillRowVO {
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
  kind: string
}

// ---- 插件 ----

/** 插件行。 */
export interface PluginRowVO {
  engine_kind: 'omp' | 'pi'
  plugin_name: string
  plugin_version: string
  plugin_source: string
  plugin_path: string
  plugin_desc?: string
  resource_kinds: string[]
  scope_kind: string
  enabled: boolean
  installed: boolean
}

/** 插件清单。 */
export interface PluginListDataVO {
  plugins: PluginRowVO[]
  registry_path: string
  read_error?: string
}

// ---- MCP ----

/** MCP 服务端行。 */
export interface McpServerVO {
  engine_kind: 'omp' | 'pi'
  server_name: string
  transport_kind: string
  command_line?: string
  server_url?: string
  desc?: string
  enabled: boolean
  disabled_by_deny: boolean
  scope_kind: string
  config_path: string
  live_status?: string
  live_error?: string
  tool_names?: string[]
}

/** MCP 清单。 */
export interface McpListDataVO {
  engine_kind: 'omp' | 'pi'
  servers: McpServerVO[]
  user_path: string
  project_path?: string
  read_errors?: string[]
  live_checked: boolean
}

/** MCP 保存请求。 */
export interface McpSaveREQ {
  engine_kind?: 'omp' | 'pi'
  server_name: string
  command_text?: string
  server_url?: string
  transport_kind?: string
  desc?: string
  env_list?: string[]
  work_dir?: string
  target_project?: boolean
}

// ---- 文件 ----

/** 文件条目。 */
export interface FileEntryVO {
  entry_name: string
  dir: boolean
  size?: number
}

/** 目录清单。 */
export interface FileListVO {
  dir_path: string
  entries: FileEntryVO[]
  truncated: boolean
}

/** 文件命中。 */
export interface FileHitVO {
  file_path: string
  entry_name: string
}

/** 搜索结果。 */
export interface FileSearchVO {
  query: string
  hits: FileHitVO[]
  truncated: boolean
}

/** 文件内容。 */
export interface FileReadVO {
  file_path: string
  content: string
  size: number
  binary: boolean
  truncated: boolean
}

// ---- Git ----

/** git 变更码。 */
export type GitChange = 'M' | 'A' | 'D' | 'R' | 'C' | 'T' | '?' | 'U'

/** 变更文件。 */
export interface GitFileVO {
  file_path: string
  change_code: GitChange
  staged: boolean
  conflict: boolean
  additions: number
  deletions: number
}

/** 远端。 */
export interface GitRemoteVO {
  remote_name: string
  remote_url: string
}

/** git 状态。 */
export interface GitStatusVO {
  available: boolean
  unavailable_reason?: string
  is_repo: boolean
  workspace: string
  repo_root: string
  branch_name: string
  upstream_ref: string
  ahead: number
  behind: number
  repo_state: string
  repo_state_detail: string
  remotes: GitRemoteVO[]
  changed_files: GitFileVO[]
  conflict: boolean
  git_version: string
}

/** git 差异。 */
export interface GitDiffVO {
  file_path: string
  truncated: boolean
  dropped: number
  diff_lines: DiffLineVO[]
}

import type { DiffLineVO } from './types_chat'

/** 提交行。 */
export interface GitCommitVO {
  commit_hash: string
  short_hash: string
  subject: string
  author_name: string
  commit_time: string
  file_count: number
  additions: number
  deletions: number
}

/** 提交详情。 */
export interface GitCommitDetailVO extends GitCommitVO {
  body: string
  parent_hash: string[]
  changed_file: GitFileVO[]
  diff?: GitDiffVO
}

/** 分支行。 */
export interface GitBranchVO {
  branch_name: string
  current: boolean
  remote_name: string
  ahead: number
  behind: number
}

/** 提交结果。 */
export interface GitCommitResultVO {
  status: string
  commit_hash: string
}

// ---- 文档与子代理 ----

/** 帮助文档条目。 */
export interface HelpDocVO {
  doc_name: string
  doc_title: string
}

/** 帮助文档内容。 */
export interface HelpDocRESP {
  doc_content: string
}

/** 子代理行（pi 无子代理，恒空）。 */
export interface AgentRowVO {
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
  progress: Record<string, unknown>
}

/** 子代理转录页。 */
export interface TranscriptPageVO {
  entries: Record<string, unknown>[]
  messages: Record<string, unknown>[]
  next: number
  reset: boolean
}

/** 把统一差异文本解析为差异行。 */
export function parseDiffLines(diff: string, limit = 400): DiffLineVO[] {
  const out: DiffLineVO[] = []
  let old_no = 0
  let new_no = 0
  for (const raw of diff.split('\n')) {
    const line = raw.replace(/\r$/, '')
    if (line.startsWith('diff --git')) {
      out.push({ line_kind: 'file', old_no: 0, new_no: 0, line_text: line })
    } else if (line.startsWith('@@')) {
      const m = /@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line)
      old_no = m ? Number(m[1]) : 0
      new_no = m ? Number(m[2]) : 0
      out.push({ line_kind: 'hunk', old_no: 0, new_no: 0, line_text: line })
    } else if (line.startsWith('+')) {
      out.push({ line_kind: 'add', old_no: 0, new_no, line_text: line.slice(1) })
      new_no += 1
    } else if (line.startsWith('-')) {
      out.push({ line_kind: 'del', old_no, new_no: 0, line_text: line.slice(1) })
      old_no += 1
    } else {
      out.push({ line_kind: 'ctx', old_no, new_no, line_text: line.startsWith(' ') ? line.slice(1) : line })
      old_no += 1
      new_no += 1
    }
    if (out.length >= limit) break
  }
  return out
}
