// HTTP 客户端：一个 axios 实例 + 统一错误映射。
// 后端恒回 200，错误在 body.code 里，所以拦截器把 code !== '0' 也变成异常，
// 上层只需要 try/catch，不用每次判断 code。
import axios, { type AxiosInstance } from 'axios'
import { getEndpoint } from './shell'
import { recordFailure } from '@/utils/diag'
import type {
  AgentRow,
  AppPrefs,
  AppRow,
  AskRequest,
  Bootstrap,
  ChatAttachment,
  ChatState,
  ConfigFileList,
  ConfigText,
  EngineKind,
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
  HealthData,
  LoginProvider,
  McpListData,
  ModelEntry,
  ModelRow,
  PluginListData,
  ProviderProbeREQ,
  ProvidersView,
  ProviderView,
  SessionDetail,
  SessionExport,
  SessionRow,
  SkillRow,
  TranscriptPage,
  UIResponse,
  WorkspaceRow,
  HelpDocRESP,
  HelpDocVO,
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
    // 压缩一轮长上下文可能要几分钟；插件安装可达十分钟。
    timeout: 600_000,
    headers: { 'X-Desktop-Token': token, 'Content-Type': 'application/json' },
  })
  client.interceptors.response.use(
    (res) => {
      const body = res.data as { code?: string; message?: string; detail?: string; data?: unknown }
      if (body && typeof body.code === 'string' && body.code !== '0') {
        const err = new ApiError(body.code, body.message || '请求失败', body.detail)
        // 记一条失败现场：错误码的「下一步」写在文档里，而用户面对的是一条已经消失的提示。
        recordFailure(err.code, err.message, err.detail ?? '', where(res.config.method, res.config.url))
        throw err
      }
      return res
    },
    (err) => {
      // 连不上后端是网络问题（8003），不是「缺包管理器」——
      // 同一个码两种意思会让界面没法分流。
      const message = err?.code === 'ECONNABORTED' ? '后端响应超时' : (err?.message ?? '连不上后端')
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

/** 引擎名只在需要时带上：省略即「后端当前选中的那个」。 */
const k = (kind?: EngineKind) => (kind ? { engine: kind } : {})

export const api = {
  bootstrap: () => get<Bootstrap>('/bootstrap'),
  settings: () => get<Record<string, string>>('/settings'),
  saveSettings: (kv: Record<string, string>) => post<Record<string, string>>('/settings/save', kv),
  setWorkspace: (path: string) => post<{ workspace: string }>('/settings/workspace', { path }),
  forgetWorkspace: (path: string) =>
    post<{ workspaces: WorkspaceRow[] }>('/settings/workspace/forget', { path }),
  setEngine: (engine: EngineKind) => post<{ engine: EngineKind }>('/settings/engine', { engine }),
  logs: () => get<{ lines: string[] }>('/logs'),

  // 应用偏好：开机自启、恢复上次会话、数据目录
  prefs: () => get<AppPrefs>('/prefs'),
  setAutoStart: (enabled: boolean) => post<AppPrefs>('/prefs/autostart', { enabled }),
  setRestoreSession: (enabled: boolean) => post<AppPrefs>('/prefs/restore', { enabled }),
  setCloseAction: (action: 'tray' | 'quit') => post<AppPrefs>('/prefs/close-action', { action }),
  rememberSession: (engine: EngineKind, sessionFile: string) =>
    post<AppPrefs>('/prefs/session', { engine_kind: engine, session_file: sessionFile }),
  openDataDir: () => post<null>('/prefs/data-dir'),

  // 引擎配置原文（只读）
  configFiles: (engine?: EngineKind) => get<ConfigFileList>('/config/files', k(engine)),
  configText: (file: string, engine?: EngineKind) => get<ConfigText>('/config/text', { file, ...k(engine) }),

  engineStatus: () => get<EngineStatus>('/engines'),
  detectEngines: () => post<EngineStatus>('/engines/detect'),
  restartEngines: (engine?: EngineKind) => post<{ ok: boolean }>('/engines/restart', k(engine)),
  // 停一个存活实例：入参必须带工作目录，只给引擎名会停错目录里那个
  stopEngine: (engine: EngineKind, workDir: string) =>
    post<{ stopped: boolean }>('/engines/stop', { engine, work_dir: workDir }),
  // 引擎健康度：概览页一屏要用的全部事实
  health: () => get<HealthData>('/health/engines'),

  // 应用：装 / 升级 / 卸载引擎本身（代跑引擎自己的安装器）
  apps: (refresh = false) => get<{ apps: AppRow[] }>('/apps', refresh ? { refresh: '1' } : undefined),
  installApp: (engine: EngineKind) => post<{ apps: AppRow[] }>('/apps/install', { engine }),
  updateApp: (engine: EngineKind) => post<{ apps: AppRow[] }>('/apps/update', { engine }),
  uninstallApp: (engine: EngineKind) => post<{ apps: AppRow[] }>('/apps/uninstall', { engine }),

  sessions: (workspace?: string, engine?: EngineKind) =>
    get<{ sessions: SessionRow[] }>('/sessions', { workspace, ...k(engine) }),
  sessionDetail: (path: string, engine?: EngineKind) =>
    get<SessionDetail>('/sessions/detail', { path, ...k(engine) }),
  deleteSession: (path: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/sessions/delete', { path, ...k(engine) }),
  renameSession: (path: string, name: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/sessions/rename', { path, name, ...k(engine) }),

  chatState: (engine: EngineKind, session?: string) =>
    get<ChatState>('/chat/state', { session, engine }),
  // 三个发送类接口共用一份形状（含附件）：分开写的后果是
  // 「插话能带图、发送不能」这种说不通的不一致。
  send: (engine: EngineKind, session: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ run_id: string }>('/chat/send', { engine, session, message, attachments }),
  stop: (engine: EngineKind, session: string) => post<{ ok: boolean }>('/chat/stop', { engine, session }),
  steer: (engine: EngineKind, session: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ ok: boolean }>('/chat/steer', { engine, session, message, attachments }),
  followUp: (engine: EngineKind, session: string, message: string, attachments: ChatAttachment[] = []) =>
    post<{ ok: boolean }>('/chat/followup', { engine, session, message, attachments }),
  clearQueue: (engine: EngineKind, session: string) =>
    post<{ ok: boolean }>('/chat/clear-queue', { engine, session }),
  removeQueued: (engine: EngineKind, session: string, message: string, queue: string) =>
    post<{ removed: boolean }>('/chat/remove-queued', { engine, session, message, queue }),
  compact: (engine: EngineKind, session: string, instructions = '') =>
    post<{ ok: boolean }>('/chat/compact', { engine, session, instructions }),
  answer: (engine: EngineKind, session: string, request: UIResponse) =>
    post<{ ok: boolean }>('/chat/answer', { engine, session, request }),
  newChat: (engine: EngineKind) => post<{ session_file: string }>('/chat/new', { engine }),
  // 复制当前会话开新分支（仅 Caps.SessionClone 的引擎提供，入口按能力位显隐）
  cloneChat: (engine: EngineKind) => post<{ session_file: string }>('/chat/clone', { engine }),
  // 导出当前会话为单文件 HTML（仅 Caps.ExportHTML 的引擎提供，入口按能力位显隐）
  exportSessionHTML: (session: string, engine?: EngineKind) =>
    post<SessionExport>('/chat/export-html', { session, ...k(engine) }),
  setModel: (engine: EngineKind, session: string, provider: string, model: string) =>
    post<{ ok: boolean }>('/chat/model', { engine, session, provider, model }),
  setThinking: (engine: EngineKind, session: string, level: string) =>
    post<{ ok: boolean }>('/chat/thinking', { engine, session, level }),
  setAutoCompaction: (engine: EngineKind, session: string, enabled: boolean) =>
    post<{ ok: boolean }>('/chat/autocompact', { engine, session, enabled }),
  // 消息隐藏：界面上「删除一轮」的两个方向。hid 是历史投影带下去的轮次标识，
  // 服务端按会话记账，引擎的会话文件与上下文不动。
  hideTurn: (session: string, hid: string) => post<{ ok: boolean }>('/messages/hide', { session, hid }),
  unhideTurn: (session: string, hid: string) => post<{ ok: boolean }>('/messages/unhide', { session, hid }),

  agents: (engine?: EngineKind) => get<{ agents: AgentRow[] }>('/agents', k(engine)),
  agentMessages: (id: string, from: number, engine?: EngineKind) =>
    get<TranscriptPage>('/agents/messages', { id, from: String(from), ...k(engine) }),
  steerAgent: (id: string, message: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/agents/steer', { id, message, ...k(engine) }),
  cancelAgent: (id: string, engine?: EngineKind) =>
    post<{ cancelled: boolean }>('/agents/cancel', { id, ...k(engine) }),
  subscribeAgents: (level: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/agents/subscribe', { level, ...k(engine) }),

  models: (engine?: EngineKind) => get<{ models: ModelRow[] }>('/models', k(engine)),
  providers: (engine?: EngineKind) => get<ProvidersView>('/providers', k(engine)),
  upsertProvider: (name: string, provider: Partial<ProviderView>, engine?: EngineKind) =>
    post<ProvidersView>('/providers/upsert', { name, provider, ...k(engine) }),
  deleteProvider: (name: string, engine?: EngineKind) =>
    post<ProvidersView>('/providers/delete', { name, ...k(engine) }),
  // 向上游问一次模型清单。密钥不给就用已保存的那个。
  fetchProviderModels: (body: ProviderProbeREQ) =>
    post<{ models: ModelEntry[]; config_path: string }>('/providers/models', body),
  // 测一条服务商配置通不通：不通时回 ok=false 与原因原话。
  testProvider: (body: ProviderProbeREQ) =>
    post<{ ok: boolean; model_count: number; message: string }>('/providers/test', body),
  loginProviders: (engine?: EngineKind) =>
    get<{ providers: LoginProvider[] }>('/login/providers', k(engine)),
  startLogin: (provider: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/login/start', { provider, ...k(engine) }),

  skills: (engine?: EngineKind) => get<{ skills: SkillRow[] }>('/skills', k(engine)),
  skillContent: (path: string) => get<{ content: string }>('/skills/content', { path }),
  saveSkillContent: (engine: EngineKind, path: string, text: string) =>
    post<{ content: string }>('/skills/save', { engine_kind: engine, file_path: path, text }),
  toggleSkill: (path: string, enabled: boolean, engine?: EngineKind) =>
    post<{ ok: boolean }>('/skills/toggle', { path, enabled, ...k(engine) }),
  createSkill: (name: string, description: string, body: string, engine?: EngineKind) =>
    post<{ path: string }>('/skills/create', { name, description, body, ...k(engine) }),
  deleteSkill: (path: string, engine?: EngineKind) =>
    post<{ ok: boolean }>('/skills/delete', { path, ...k(engine) }),
  // 矩阵页：把另一个引擎的用户级同名技能复制过来
  skillCopyTo: (path: string, engine: EngineKind) =>
    post<{ path: string }>('/skills/copy', { path, engine_kind: engine }),

  plugins: (engine?: EngineKind) => get<PluginListData>('/plugins', k(engine)),
  installPlugin: (source: string, engine?: EngineKind) =>
    post<{ output: string }>('/plugins/install', { source, ...k(engine) }),
  uninstallPlugin: (source: string, engine?: EngineKind) =>
    post<{ output: string }>('/plugins/uninstall', { source, ...k(engine) }),
  // 启停 / 升级只对有该能力的引擎开放（omp），入口由 Caps 决定显隐
  togglePlugin: (name: string, enabled: boolean, engine?: EngineKind) =>
    post<{ output: string }>('/plugins/toggle', { plugin_name: name, enabled, ...k(engine) }),
  upgradePlugin: (source: string, engine?: EngineKind) =>
    post<{ output: string }>('/plugins/upgrade', { plugin_source: source, ...k(engine) }),

  // MCP 服务器
  mcpList: (engine?: EngineKind, live = false) =>
    get<McpListData>('/mcp', { ...k(engine), ...(live ? { live: '1' } : {}) }),
  mcpSave: (body: {
    engine_kind?: EngineKind
    server_name: string
    command_text?: string
    server_url?: string
    transport_kind?: string
    desc?: string
    env_list?: string[]
    work_dir?: string
    target_project?: boolean
  }) => post<McpListData>('/mcp/save', body),
  mcpToggle: (serverName: string, enabled: boolean, engine?: EngineKind) =>
    post<McpListData>('/mcp/toggle', { server_name: serverName, enabled, ...k(engine) }),
  mcpRemove: (serverName: string, engine?: EngineKind) =>
    post<McpListData>('/mcp/remove', { server_name: serverName, ...k(engine) }),
  // 矩阵页：把另一个引擎里同名服务器的定义复制过来并启停
  mcpCopyTo: (serverName: string, enabled: boolean, engine: EngineKind) =>
    post<McpListData>('/mcp/copy', { server_name: serverName, enabled, engine_kind: engine }),

  // 工作区
  files: (path: string) => get<FileList>('/files', { path }),
  filesSearch: (query: string, limit = 50) => get<FileSearch>('/files/search', { query, limit: String(limit) }),
  fileRead: (path: string) => get<FileRead>('/files/read', { path }),
  gitStatus: (path?: string) => get<GitStatus>('/git/status', { path }),
  gitDiff: (path?: string) => get<GitDiff>('/git/diff', { path }),
  gitHistory: (limit = 50) => get<GitCommit[]>('/git/history', { limit: String(limit) }),
  gitCommitDetail: (hash: string) => get<GitCommitDetail>('/git/commit', { hash }),
  gitFileHistory: (path: string, limit = 40) =>
    get<{ commits: GitCommit[] }>('/git/file-history', { path, limit: String(limit) }),
  gitFileDiff: (hash: string, path: string) =>
    get<GitDiff>('/git/file-diff', { hash, path }),
  gitBranches: () => get<GitBranch[]>('/git/branches'),
  gitStage: (paths: string[]) => post<GitStatus>('/git/stage', { paths }),
  gitUnstage: (paths: string[]) => post<GitStatus>('/git/unstage', { paths }),
  gitDiscard: (paths: string[]) => post<GitStatus>('/git/discard', { paths }),
  gitResolve: (paths: string[]) => post<GitStatus>('/git/resolve', { paths }),
  gitCommit: (message: string, amend = false) => post<GitCommitResult>('/git/commit', { message, amend }),
  gitCreateBranch: (name: string) => post<GitStatus>('/git/branch', { name }),
  gitSwitchBranch: (name: string) => post<GitStatus>('/git/switch', { name }),
  gitDeleteBranch: (name: string) => post<GitStatus>('/git/branch/delete', { name }),
  gitMerge: (name: string, rebase = false) => post<GitStatus>('/git/merge', { name, rebase }),
  gitState: (action: 'continue' | 'abort') => post<GitStatus>('/git/state', { action }),
  gitInit: (branch = '') => post<GitStatus>('/git/init', { branch }),
  gitAddRemote: (name: string, url: string) => post<GitStatus>('/git/remote', { name, url }),
  gitPush: (remote: string, branch: string, force = 'none', setUpstream = false, confirm = '') =>
    post<GitStatus>('/git/push', {
      remote_name: remote,
      branch_name: branch,
      force,
      set_upstream: setUpstream,
      confirm,
    }),
  gitPull: (remote: string, branch: string, rebase = false) =>
    post<GitStatus>('/git/pull', { remote_name: remote, branch_name: branch, rebase }),
  gitFetch: (remote = '') => post<GitStatus>('/git/fetch', { remote_name: remote }),

  reveal: (path: string) => post<{ ok: boolean }>('/system/reveal', { path }),
  openPath: (path: string) => post<{ ok: boolean }>('/system/open', { path }),

  // 内置帮助文档
  docs: () => get<HelpDocVO[]>('/docs'),
  docContent: (name: string) => get<HelpDocRESP>(`/docs/${encodeURIComponent(name)}`),
}

export type { AskRequest }
