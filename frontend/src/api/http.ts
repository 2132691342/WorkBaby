// HTTP 客户端：一个 axios 实例 + 统一错误映射。
// 后端恒回 200，错误在 body.code 里，所以拦截器把 code !== '0' 也变成异常，
// 上层只需要 try/catch，不用每次判断 code。
import axios, { type AxiosInstance } from 'axios'
import i18n from '@/i18n'
import { getEndpoint } from './wails'
import { recordFailure } from '@/utils/diag'
import type {
  AppPrefs,
  AppRow,
  AskRequest,
  Bootstrap,
  ChatAttachment,
  ChatState,
  ConfigFile,
  ConfigText,
  EngineStatus,
  FileList,
  FileRead,
  FileSearch,
  GitBranch,
  GitCommit,
  GitCommitDetail,
  GitCommitResult,
  GitDiff,
  GitStatus,
  GitStash,
  HealthData,
  McpListData,
  ModelDefaults,
  ModelDefaultsSave,
  ModelEntry,
  ModelRow,
  WorkspaceList,
  ProviderProbeREQ,
  ProviderProbeRESP,
  ProvidersView,
  RuntimeStatus,
  SessionExport,
  SessionList,
  SkillListData,
  UIResponse,
  IndexSearchView,
  IndexStatusView,
  WebFetchView,
  WebSearchView,
  WebStatusView,
} from '@/types/api'

export class ApiError extends Error {
  code: string
  detail?: string
  constructor(code: string, message: string, detail?: string) {
    super(message)
    this.code = code
    this.detail = detail
  }
}

let client: AxiosInstance | null = null
let baseURL = ''
let token = ''

export async function ensureClient(): Promise<AxiosInstance> {
  if (client) return client
  const ep = await getEndpoint()
  baseURL = ep.wb_base_url
  token = ep.wb_token
  client = axios.create({
    baseURL: ep.wb_base_url,
    // 压缩一轮长上下文可能要几分钟；重建索引可达十分钟。
    timeout: 600_000,
    headers: { 'X-Desktop-Token': token, 'Content-Type': 'application/json' },
  })
  client.interceptors.response.use(
    (res) => {
      const body = res.data as { code?: string; message?: string; detail?: string; data?: unknown }
      const tr = i18n.global.t
      if (body && typeof body.code === 'string' && body.code !== '0') {
        const err = new ApiError(body.code, body.message || tr('net.requestFailed'), body.detail)
        // 记一条失败现场：错误码的「下一步」写在文档里，而用户面对的是一条已经消失的提示。
        recordFailure(err.code, err.message, err.detail ?? '', where(res.config.method, res.config.url))
        throw err
      }
      return res
    },
    (err) => {
      // 连不上后端是网络问题（8003），不是「缺包管理器」——
      // 同一个码两种意思会让界面没法分流。
      const tr = i18n.global.t
      const message = err?.code === 'ECONNABORTED' ? tr('net.timeout') : (err?.message ?? tr('net.unreachable'))
      recordFailure('8003', message, String(err?.stack ?? ''), where(err?.config?.method, err?.config?.url))
      throw new ApiError('8003', message)
    },
  )
  return client
}

/** 拼一个「方法 + 路径」的定位串；axios 的 config 可能缺大半，所以每一段都兜底。 */
function where(method: unknown, url: unknown): string {
  const m = typeof method === 'string' ? method.toUpperCase() : ''
  const u = typeof url === 'string' ? url : ''
  return [m, u].filter(Boolean).join(' ') || 'unknown'
}

/** SSE 需要自己拼地址与令牌：EventSource 不带自定义头，令牌走查询参数。 */
export function streamURL(since = 0): string {
  const suffix = since > 0 ? `&since=${since}` : ''
  return `${baseURL}/events?token=${encodeURIComponent(token)}${suffix}`
}

async function get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
  const c = await ensureClient()
  const res = await c.get(path, { params })
  return res.data.data as T
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const c = await ensureClient()
  const res = await c.post(path, body ?? {})
  return res.data.data as T
}

/** 会话参数只在需要时带上：省略即「后端当前绑定的那个」。 */
const s = (sessionFile?: string) => (sessionFile ? { session_file: sessionFile } : {})

export const api = {
  // ---- 引导 / 设置 / 偏好 ----
  bootstrap: () => get<Bootstrap>('/bootstrap'),
  settings: () => get<Record<string, string>>('/settings'),
  saveSettings: (kv: Record<string, string>) => post<Record<string, string>>('/settings/save', kv),
  setWorkspace: (workspace: string) => post<{ workspace: string }>('/settings/workspace', { workspace }),
  workspaces: () => get<WorkspaceList>('/workspaces'),
  forgetWorkspace: (workspace: string) =>
    post<{ workspaces: Bootstrap['workspaces'] }>('/settings/workspace/forget', { workspace_path: workspace }),
  logs: (lines = 500) => get<{ lines: string[] }>('/logs', { lines: String(lines) }),

  prefs: () => get<AppPrefs>('/prefs'),
  setAutoStart: (prefEnabled: boolean) => post<AppPrefs>('/prefs/autostart', { pref_enabled: prefEnabled }),
  setRestoreSession: (prefEnabled: boolean) => post<AppPrefs>('/prefs/restore', { pref_enabled: prefEnabled }),
  setCloseAction: (closeAction: 'tray' | 'quit') => post<AppPrefs>('/prefs/close-action', { close_action: closeAction }),
  openDataDir: () => post<{ ok: boolean }>('/prefs/data-dir'),

  // ---- 引擎 / 应用 / 健康 ----
  engineStatus: () => get<EngineStatus>('/engines'),
  detectEngines: () => post<RuntimeStatus>('/engines/detect'),
  restartEngine: () => post<{ ok: boolean }>('/engines/restart'),
  health: () => get<HealthData>('/health/engines'),
  apps: () => get<{ apps: AppRow[] }>('/apps'),

  // ---- 会话 ----
  // q 非空时后端按标题与正文过滤（大小写不敏感）
  sessions: (workspace?: string, query?: string) =>
    get<SessionList>('/sessions', { workspace, q: query ?? '' }),
  deleteSession: (sessionPath: string) => post<{ ok: boolean }>('/sessions/delete', { session_path: sessionPath }),
  renameSession: (sessionPath: string, sessionTitle: string) =>
    post<{ ok: boolean }>('/sessions/rename', { session_path: sessionPath, session_title: sessionTitle }),

  // ---- 对话 ----
  chatState: (sessionFile?: string) => get<ChatState>('/chat/state', { session: sessionFile }),
  // 三个发送类接口共用一份形状（含附件）：分开写的后果是
  // 「插话能带图、发送不能」这种说不通的不一致。
  send: (sessionFile: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ run_id: string; disposition: string }>('/chat/send', {
      session_file: sessionFile,
      message,
      attachments,
    }),
  stop: (sessionFile: string) => post<{ ok: boolean }>('/chat/stop', s(sessionFile)),
  steer: (sessionFile: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ ok: boolean }>('/chat/steer', { session_file: sessionFile, message, attachments }),
  followUp: (sessionFile: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ ok: boolean }>('/chat/followup', { session_file: sessionFile, message, attachments }),
  clearQueue: (sessionFile: string) =>
    post<{ steering: string[]; follow_up: string[] }>('/chat/clear-queue', s(sessionFile)),
  // 单条移除：后端用「取出全队 → 摘一条 → 其余重投」实现，返回移除后的两条队
  removeQueued: (sessionFile: string, queueKind: 'steering' | 'followup', queueIndex: number) =>
    post<{ steering: string[]; follow_up: string[] }>('/chat/remove-queued', {
      session_file: sessionFile,
      queue_kind: queueKind,
      queue_index: queueIndex,
    }),
  compact: (sessionFile: string, instructions = '') =>
    post<{ ok: boolean }>('/chat/compact', { session_file: sessionFile, instructions }),
  answer: (sessionFile: string, req: UIResponse) =>
    post<{ ok: boolean }>('/chat/answer', { session_file: sessionFile, ...req }),
  newChat: () => post<{ session_file: string }>('/chat/new'),
  cloneChat: () => post<{ session_file: string }>('/chat/clone'),
  exportSessionHTML: (sessionFile: string) =>
    post<SessionExport>('/chat/export-html', s(sessionFile)),
  setModel: (sessionFile: string, providerName: string, modelId: string) =>
    post<{ ok: boolean }>('/chat/model', { session_file: sessionFile, provider_name: providerName, model_id: modelId }),
  setThinking: (sessionFile: string, thinkingLevel: string) =>
    post<{ ok: boolean }>('/chat/thinking', { session_file: sessionFile, thinking_level: thinkingLevel }),
  setAutoCompaction: (sessionFile: string, autoCompaction: boolean) =>
    post<{ ok: boolean }>('/chat/autocompact', { session_file: sessionFile, auto_compaction: autoCompaction }),
  // 消息隐藏：界面上「删除一轮」的两个方向。后端按会话记账，引擎的会话文件不动。
  hideTurn: (sessionFile: string, turnId: string) =>
    post<{ ok: boolean }>('/messages/hide', { session_file: sessionFile, turn_id: turnId }),
  unhideTurn: (sessionFile: string, turnId: string) =>
    post<{ ok: boolean }>('/messages/unhide', { session_file: sessionFile, turn_id: turnId }),

  // ---- 模型与服务商 ----
  models: () => get<{ models: ModelRow[] }>('/models'),
  providers: () => get<ProvidersView>('/providers'),
  upsertProvider: (body: {
    provider_name: string
    base_url: string
    api_protocol: string
    auth_kind: string
    api_key: string
    models?: ModelEntry[]
  }) => post<ProvidersView>('/providers/upsert', body),
  deleteProvider: (providerName: string) =>
    post<ProvidersView>('/providers/delete', { provider_name: providerName }),
  // 向上游问一次模型清单。密钥不给就用已保存的那个。
  probeProvider: (body: ProviderProbeREQ) => post<ProviderProbeRESP>('/providers/models', body),
  deleteProviderModel: (providerName: string, modelId: string) =>
    post<ProvidersView>('/providers/model/delete', { provider_name: providerName, model_id: modelId }),
  // 新会话的起步设置：默认服务商 / 默认模型 / 默认思考档
  modelDefaults: () => get<ModelDefaults>('/models/defaults'),
  saveModelDefaults: (body: ModelDefaultsSave) => post<ModelDefaults>('/models/defaults', body),

  // ---- 技能 / MCP ----
  skills: () => get<SkillListData>('/skills'),
  skillContent: (path: string) => get<{ file_text: string }>('/skills/content', { path }),
  saveSkillContent: (skillPath: string, fileText: string) =>
    post<{ file_text: string }>('/skills/save', { skill_path: skillPath, file_text: fileText }),
  toggleSkill: (skillPath: string, skillEnabled: boolean) =>
    post<{ ok: boolean }>('/skills/toggle', { skill_path: skillPath, skill_enabled: skillEnabled }),
  createSkill: (skillName: string, skillDesc: string, skillBody: string) =>
    post<{ skill_path: string }>('/skills/create', { skill_name: skillName, skill_desc: skillDesc, skill_body: skillBody }),
  // 把一条推荐技能装进用户的技能区；已存在会被拒
  installSkill: (skillDirName: string) =>
    post<{ ok: boolean }>('/skills/install', { skill_dir_name: skillDirName }),
  importSkill: (skillFromPath: string) =>
    post<{ skill_dir_name: string; skill_path: string }>('/skills/import', { skill_from_path: skillFromPath }),
  deleteSkill: (skillPath: string) => post<{ ok: boolean }>('/skills/delete', { skill_path: skillPath }),
  // 内置技能改坏了：从随包原件覆盖回来
  resetSkill: (skillPath: string) => post<{ ok: boolean }>('/skills/reset', { skill_path: skillPath }),

  // 插件生态已被引擎内建能力取代，这里不再暴露插件的安装 / 启停 / 升级入口。

  mcpList: () => get<McpListData>('/mcp'),
  mcpSave: (body: {
    server_name: string
    command_text?: string
    server_url?: string
    transport_kind?: string
    mcp_desc?: string
    work_dir?: string
    env_list?: string[]
    /** 连接超时（秒）；0 表示不配置 */
    mcp_timeout?: number
    mcp_scope?: 'user' | 'project'
  }) => post<McpListData>('/mcp/save', body),
  // 连接测试：stdio 拉真进程、http 发真请求，完成一次 MCP initialize 握手
  mcpProbe: (serverName: string) =>
    post<{ probe_ok: boolean; probe_message: string; probe_tools: number }>('/mcp/probe', { server_name: serverName }),
  mcpToggle: (serverName: string, mcpEnabled: boolean) =>
    post<McpListData>('/mcp/toggle', { server_name: serverName, mcp_enabled: mcpEnabled }),
  mcpRemove: (serverName: string) => post<McpListData>('/mcp/remove', { server_name: serverName }),

  // ---- 工作区 ----
  files: (path: string) => get<FileList>('/files', { path }),
  filesSearch: (query: string, limit = 50) => get<FileSearch>('/files/search', { query, limit: String(limit) }),
  fileRead: (path: string) => get<FileRead>('/files/read', { path }),
  fileSave: (filePath: string, fileText: string) =>
    post<{ ok: boolean }>('/files/save', { file_path: filePath, file_text: fileText }),
  fileMkdir: (filePath: string) => post<{ ok: boolean }>('/files/mkdir', { file_path: filePath }),
  fileDelete: (filePath: string) => post<{ ok: boolean }>('/files/delete', { file_path: filePath }),
  fileRename: (filePath: string, newFilePath: string) =>
    post<{ ok: boolean }>('/files/rename', { file_path: filePath, new_file_path: newFilePath }),
  gitStatus: () => get<GitStatus>('/git/status'),
  gitDiff: (path?: string) => get<GitDiff>('/git/diff', { path }),
  gitHistory: (limit = 50) => get<{ commits: GitCommit[] }>('/git/history', { limit: String(limit) }),
  gitCommitDetail: (hash: string) => get<GitCommitDetail>('/git/commit', { hash }),
  gitFileHistory: (path: string, limit = 40) =>
    get<{ commits: GitCommit[] }>('/git/file-history', { path, limit: String(limit) }),
  gitFileDiff: (hash: string, path: string) => get<GitDiff>('/git/file-diff', { hash, path }),
  gitBranches: () => get<GitBranch[]>('/git/branches'),
  gitStage: (filePaths: string[]) => post<GitStatus>('/git/stage', { file_paths: filePaths }),
  gitUnstage: (filePaths: string[]) => post<GitStatus>('/git/unstage', { file_paths: filePaths }),
  gitDiscard: (filePaths: string[]) => post<GitStatus>('/git/discard', { file_paths: filePaths }),
  gitResolve: (filePaths: string[]) => post<GitStatus>('/git/resolve', { file_paths: filePaths }),
  gitResolveConflict: (filePath: string, resolveSide: 'ours' | 'theirs' | '') =>
    post<GitStatus>('/git/conflict/resolve', { file_path: filePath, resolve_side: resolveSide }),
  gitCommit: (commitMessage: string, isAmend = false) =>
    post<GitCommitResult>('/git/commit', { commit_message: commitMessage, is_amend: isAmend }),
  gitCreateBranch: (branchName: string) => post<GitStatus>('/git/branch', { branch_name: branchName }),
  gitSwitchBranch: (branchName: string) => post<GitStatus>('/git/switch', { branch_name: branchName }),
  gitDeleteBranch: (branchName: string) => post<GitStatus>('/git/branch/delete', { branch_name: branchName }),
  gitMerge: (branchName: string, isRebase = false) => post<GitStatus>('/git/merge', { branch_name: branchName, is_rebase: isRebase }),
  gitState: (stateAction: 'continue' | 'abort') => post<GitStatus>('/git/state', { state_action: stateAction }),
  gitInit: (branchName = '') => post<GitStatus>('/git/init', { branch_name: branchName }),
  gitAddRemote: (remoteName: string, remoteUrl: string) =>
    post<GitStatus>('/git/remote', { remote_name: remoteName, remote_url: remoteUrl }),
  gitPush: (body: {
    remote_name: string
    branch_name: string
    force_mode?: string
    set_upstream?: boolean
    confirm?: string
  }) => post<GitStatus>('/git/push', body),
  gitPull: (remoteName: string, branchName: string, isRebase = false) =>
    post<GitStatus>('/git/pull', { remote_name: remoteName, branch_name: branchName, is_rebase: isRebase }),
  gitFetch: (remoteName = '') => post<GitStatus>('/git/fetch', { remote_name: remoteName }),
  // stash：暂存区之外的「先收起来」，存取列丢四件套
  gitStashList: () => get<{ stashes: GitStash[] }>('/git/stash/list'),
  gitStashPush: (stashMessage = '') => post<GitStatus>('/git/stash/push', { stash_message: stashMessage }),
  gitStashPop: () => post<GitStatus>('/git/stash/pop'),
  gitStashDrop: (stashRef: string, confirm: string) =>
    post<GitStatus>('/git/stash/drop', { stash_ref: stashRef, confirm }),

  // ---- 系统 / 配置 ----
  reveal: (targetPath: string) => post<{ ok: boolean }>('/system/reveal', { target_path: targetPath }),
  configFiles: () => get<{ entries: ConfigFile[] }>('/config/files'),
  configText: (name: string) => get<ConfigText>('/config/text', { name }),

  // ---- 联网 ----
  webStatus: () => get<WebStatusView>('/web/status'),
  webSettings: (webKind: string, webApiKey = '') =>
    post<WebStatusView>('/web/settings', { web_kind: webKind, web_api_key: webApiKey }),
  webSearch: (searchQuery: string, searchLimit = 8) =>
    post<WebSearchView>('/web/search', { search_query: searchQuery, search_limit: searchLimit }),
  webFetch: (fetchUrl: string) =>
    post<WebFetchView>('/web/fetch', { fetch_url: fetchUrl }),

  // ---- 工作区预索引 ----
  indexStatus: () => get<IndexStatusView>('/index/status'),
  indexSearch: (searchQuery: string, searchLimit = 20) =>
    post<IndexSearchView>('/index/search', { search_query: searchQuery, search_limit: searchLimit }),
  indexRebuild: () => post<IndexStatusView>('/index/rebuild'),
}

export type { AskRequest }
