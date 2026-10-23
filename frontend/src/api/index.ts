// 前端 API 层：路径与 backend/server/routes.go 一一对应，字段名照后端 json tag。
import { get, getBaseURL, post } from './http'
import type {
  AttachmentREQ,
  AvailableModelVO,
  BackgroundVO,
  BootstrapVO,
  CommandVO,
  EngineLatestVO,
  EngineStatusVO,
  ExtensionListRESP,
  FileListVO,
  GitBranchVO,
  GitCommitDetailVO,
  GitCommitRESP,
  GitCommitVO,
  GitDiffVO,
  GitMessageRESP,
  GitPullREQ,
  GitPushREQ,
  GitStatusVO,
  HelpDocRESP,
  HelpDocVO,
  MCPListRESP,
  PiEnvVO,
  PISettingsVO,
  ProviderListRESP,
  ProviderVO,
  SendMessageRESP,
  SessionDetailVO,
  SessionVO,
  SkillVO,
  StatsRESP,
  ToolListRESP,
  UpsertProviderREQ,
  WorkspacesRESP,
} from '../types/api'

/** 丢掉空值再拼查询串：不带空参数，后端「没给」与「给了空串」就不用分开判 */
function pick(src: Record<string, string>): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(src)) if (v) out[k] = v
  return Object.keys(out).length ? out : undefined
}

export const bootstrap = () => get<BootstrapVO>('/bootstrap')

/** 首屏预热聚合：设置页各面板的清单一次拉齐（字段与 domain.WarmupRESP 一致） */
export interface WarmupRESP {
  skills: SkillVO[]
  providers: ProviderListRESP | null
  tools: ToolListRESP | null
  extensions: ExtensionListRESP | null
  mcp: MCPListRESP | null
}
export const warmup = () => get<WarmupRESP>('/warmup')

/** 路径约定：变量参数一律落在路径末尾（`/{resource}/{action}/:id`） */
export const sessions = {
  /** workspace 为空表示「全部工作目录」 */
  list: (workspace = '') => get<SessionVO[]>('/sessions', pick({ workspace })),
  detail: (id: string) => get<SessionDetailVO>(`/sessions/${id}`),
  rename: (id: string, title: string) => post<boolean>(`/sessions/rename/${id}`, { title }),
  remove: (id: string) => post<boolean>(`/sessions/delete/${id}`),
  setModel: (id: string, provider: string, model: string) =>
    post<boolean>(`/sessions/model/${id}`, { provider, model }),
  setThinking: (id: string, level: string) => post<boolean>(`/sessions/thinking/${id}`, { level }),
  /** 导出成 HTML：path 由保存对话框给出，导出实现在 PI 里 */
  exportHTML: (id: string, path: string) => post<string>(`/sessions/export/${id}`, { path }),
}

export const chat = {
  /** session_id 为空 = 新对话：后端会现场创建并回传 session_id */
  send: (body: { session_id?: string; workspace?: string; content: string; attachments?: AttachmentREQ[] }) =>
    post<SendMessageRESP>('/chat/send', body),
  stop: (session_id: string) => post<boolean>('/chat/stop', { session_id }),
  steer: (session_id: string, content: string) => post<boolean>('/chat/steer', { session_id, content }),
  followUp: (session_id: string, content: string) =>
    post<boolean>('/chat/followup', { session_id, content }),
  /** 叫停正在进行的自动重试 */
  cancelRetry: (session_id: string) => post<boolean>('/chat/retry-cancel', { session_id }),
  /** 手动压缩上下文（PI 的 /compact） */
  compact: (session_id: string) => post<boolean>('/chat/compact', { session_id }),
  /** 回答引擎抛回来的问题 */
  answer: (body: { id: string; value?: string; confirmed?: boolean; cancelled?: boolean }) =>
    post<boolean>('/chat/answer', body),
}

export const providers = {
  list: () => get<ProviderListRESP>('/providers'),
  upsert: (body: UpsertProviderREQ) => post<ProviderVO>('/providers/upsert', body),
  remove: (id: string) => post<boolean>('/providers/delete', { id }),
  test: (id: string) => post<{ ok: boolean; model: string; message: string }>('/providers/test', { id }),
  /** 显式查看已保存的密钥（用户点「显示」时才调用） */
  reveal: (id: string) => post<{ api_key: string }>('/providers/reveal', { id }),
  setDefault: (provider: string, model: string) =>
    post<boolean>('/providers/default', { provider, model }),
  /** 拉上游模型清单：编辑已存在的服务时可以只给 id 复用已存密钥 */
  fetchModels: (body: { api?: string; base_url?: string; api_key?: string; id?: string }) =>
    post<{ models: string[] }>('/providers/models/fetch', body),
  /** PI 眼下的可用模型（含内置服务商与登录态） */
  available: (workspace = '') => get<AvailableModelVO[]>('/models/available', pick({ workspace })),
}

export const skills = {
  list: () => get<SkillVO[]>('/skills'),
  create: (body: { name: string; description: string; body: string }) => post<SkillVO>('/skills/create', body),
  import: (paths: string[]) => post<{ imported: number; skipped: string[] }>('/skills/import', { paths }),
  /** 导入 zip 技能包：data 是 zip 原始字节的 base64 */
  importZip: (filename: string, data: string) =>
    post<{ imported: number; skipped: string[] }>('/skills/import-zip', { filename, data }),
  remove: (id: string) => post<boolean>(`/skills/delete/${id}`),
  toggle: (id: string, enabled: boolean) => post<boolean>(`/skills/toggle/${id}`, { enabled }),
  content: (id: string) => get<{ content: string }>(`/skills/content/${id}`),
}

/** 工作区文件浏览：path 为相对工作区的目录，空串即根目录 */
export const files = {
  list: (path = '') => get<FileListVO>('/files', { path }),
}

/** 插件（PI 扩展与包）：目录即真相源，settings.json 的 extensions / packages 是启用清单 */
export const extensions = {
  list: () => get<ExtensionListRESP>('/extensions'),
  toggle: (path: string, enabled: boolean) => post<boolean>('/extensions/toggle', { path, enabled }),
  import: (paths: string[]) =>
    post<{ imported: number; skipped: string[] }>('/extensions/import', { paths }),
  remove: (path: string) => post<boolean>('/extensions/delete', { path }),
  /** 装包 / 卸包：进度走 chat:setup 事件流式回传，接口立刻返回 */
  install: (source: string) => post<boolean>('/extensions/install', { source }),
  uninstall: (source: string) => post<boolean>('/extensions/remove', { source }),
  reload: () => post<boolean>('/extensions/reload'),
}

/** 斜杠命令清单：没有会话时后端直接给空数组（没有进程就没有命令） */
export const commands = {
  list: (session_id = '') => get<CommandVO[]>('/commands', pick({ session_id })),
}

export const tools = {
  list: () => get<ToolListRESP>('/tools'),
  toggle: (name: string, enabled: boolean) => post<boolean>('/tools/toggle', { name, enabled }),
}

export const mcp = {
  list: (workspace = '') => get<MCPListRESP>('/mcp', pick({ workspace })),
  /** 保存配置原文：后端先校验再落盘，解析不过一个字符都不写 */
  save: (scope: string, raw: string) => post<boolean>('/mcp/save', { scope, raw }),
  remove: (name: string, scope: string) => post<boolean>('/mcp/remove', { name, scope }),
  login: (name: string) => post<boolean>('/mcp/login', { name }),
  logout: (name: string) => post<boolean>('/mcp/logout', { name }),
  reload: () => post<boolean>('/mcp/reload'),
}

export const settings = {
  all: () => get<Record<string, string>>('/settings'),
  set: (key: string, value: string) => post<boolean>('/settings', { key, value }),
  /** PI 侧配置快照：默认模型、思考档、压缩参数与全部文件位置 */
  pi: () => get<PISettingsVO>('/settings/pi'),
  setThinking: (level: string) => post<boolean>('/settings/pi/thinking', { level }),
  setCompaction: (reserve: number, keep: number) =>
    post<boolean>('/settings/pi/compaction', { reserve, keep }),
  workspaces: () => get<WorkspacesRESP>('/settings/workspaces'),
  background: () => get<BackgroundVO>('/settings/background'),
  setBackground: (path: string, opacity: number, blur: number) =>
    post<BackgroundVO>('/settings/background', { path, opacity, blur }),
}

/** 背景图本体：前端拿不到数据目录里的文件路径，只能走这个 URL */
export const backgroundUrl = () => `${getBaseURL()}/assets/background`

/** 环境引导：闸门快照与 PI 的安装更新 */
export const env = {
  status: () => get<PiEnvVO>('/env'),
  install: () => post<boolean>('/env/install'),
  update: () => post<boolean>('/env/update'),
  recheck: () => post<PiEnvVO>('/env/recheck'),
  /** 重新探测命令执行环境并写回 PI（装了 Git 之后点这个） */
  fixShell: () => post<PiEnvVO>('/env/shell'),
  /** 生成终端里的 pi 入口并挂到用户 PATH */
  enableTerminal: () => post<PiEnvVO>('/env/terminal'),
}

/** 引擎：PI 与内置运行时的状态、版本对账与升级 */
export const engine = {
  /** 本地快照：不联网 */
  status: () => get<EngineStatusVO>('/engine/status'),
  /** 联网查最新版本（PI + npm 来源的插件包） */
  latest: () => get<EngineLatestVO>('/engine/latest'),
  /** 升级 PI；进度走 chat:setup 事件流 */
  update: () => post<boolean>('/engine/update'),
  /** 更新插件包；source 为空表示全部 */
  updatePackages: (source = '') => post<boolean>('/engine/packages/update', { source }),
}

export const stats = (span: string) => get<StatsRESP>('/stats', { span })

/** 内置帮助文档（只读，随应用打包） */
export const docs = {
  list: () => get<HelpDocVO[]>('/docs'),
  content: (name: string) => get<HelpDocRESP>(`/docs/${name}`),
}

/** 工作区版本状态：带 session_id 时看该会话的工作目录 */
export const git = {
  status: (session_id = '') => get<GitStatusVO>('/git/status', pick({ session_id })),
  diff: (path = '', session_id = '') => get<GitDiffVO>('/git/diff', pick({ path, session_id })),
  history: (session_id = '', limit = 50) =>
    get<GitCommitVO[]>('/git/history', pick({ session_id, limit: String(limit) })),
  commit: (hash: string, session_id = '') =>
    get<GitCommitDetailVO>('/git/commit', pick({ hash, session_id })),
  fileHistory: (path: string, session_id = '', limit = 50) =>
    get<GitCommitVO[]>('/git/file-history', pick({ path, session_id, limit: String(limit) })),
  branches: (session_id = '') => get<GitBranchVO[]>('/git/branches', pick({ session_id })),

  stage: (paths: string[], session_id = '') => post<GitStatusVO>('/git/stage', { paths, session_id }),
  unstage: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/unstage', { paths, session_id }),
  /** 丢弃工作区改动；必须显式给文件，后端不提供「全部丢弃」 */
  discard: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/discard', { paths, session_id }),
  /** 把冲突文件标记为已解决（等同 add，但不代为提交） */
  resolve: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/resolve', { paths, session_id }),

  commit_: (message: string, session_id = '', amend = false) =>
    post<GitCommitRESP>('/git/commit', { message, session_id, amend }),

  createBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/branch', { name, session_id }),
  switchBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/switch', { name, session_id }),
  deleteBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/branch/delete', { name, session_id }),
  /** 合并或变基；冲突时返回带 state 的状态而不是错误 */
  merge: (name: string, rebase: boolean, session_id = '') =>
    post<GitStatusVO>('/git/merge', { name, rebase, session_id }),
  /** 继续或中止未完成的合并 / 变基 */
  state: (action: 'continue' | 'abort', session_id = '') =>
    post<GitStatusVO>('/git/state', { action, session_id }),

  pull: (req: GitPullREQ, session_id = '') => post<GitStatusVO>('/git/pull', { ...req, session_id }),
  /** 取回远端最新状态；不改工作区也不改本地分支 */
  fetch: (remote = '', session_id = '') => post<GitStatusVO>('/git/fetch', { remote, session_id }),
  push: (req: GitPushREQ = {}, session_id = '') =>
    post<GitStatusVO>('/git/push', { ...req, session_id }),

  init: (branch: string, session_id = '') => post<GitStatusVO>('/git/init', { branch, session_id }),
  addRemote: (name: string, url: string, session_id = '') =>
    post<GitStatusVO>('/git/remote', { name, url, session_id }),

  draftMessage: (paths: string[], session_id = '') =>
    post<GitMessageRESP>('/git/message', { paths, session_id }),
}
