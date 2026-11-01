// 前端 API 层：路径与 wb-api 路由一一对应，字段名照 domain serde json 逐字一致。
import { get, getBaseURL, post } from './http'
import type {
  AvailableModelVO,
  BackgroundVO,
  BootVO,
  CommandVO,
  ContextBreakdownRESP,
  FileBytesRESP,
  FileListRESP,
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
  MessageVO,
  ProviderListRESP,
  ProviderVO,
  RevealProviderRESP,
  SendMessageRESP,
  SessionDetailRESP,
  SessionVO,
  SkillContentRESP,
  SkillImportRESP,
  SkillVO,
  StatsRESP,
  TestProviderRESP,
  ToolVO,
  UpsertProviderREQ,
} from '../types/api'

/** 丢掉空值再拼查询串：不带空参数，后端「没给」与「给了空串」就不用分开判 */
function pick(src: Record<string, string>): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(src)) if (v) out[k] = v
  return Object.keys(out).length ? out : undefined
}

export const bootstrap = () => get<BootVO>('/bootstrap')

/** 首屏预热聚合：设置页各面板的清单一次拉齐 */
export interface WarmupRESP {
  providers: ProviderVO[]
  skills: SkillVO[]
  tools: ToolVO[]
  features: import('../types/api').FeatureVO[]
}
export const warmup = () => get<WarmupRESP>('/warmup')

/** 路径约定：变量参数一律落在路径末尾（`/{resource}/{action}/:id`） */
export const sessions = {
  list: () => get<SessionVO[]>('/sessions'),
  detail: (id: string) => get<SessionDetailRESP>(`/sessions/${id}`),
  rename: (id: string, title: string) => post<boolean>(`/sessions/rename/${id}`, { session_name: title }),
  remove: (id: string) => post<boolean>(`/sessions/delete/${id}`),
  setModel: (id: string, provider: string, model: string) =>
    post<boolean>(`/sessions/model/${id}`, { session_provider: provider, session_model: model }),
  setThinking: (id: string, level: string) =>
    post<boolean>(`/sessions/thinking/${id}`, { session_thinking_level: level }),
  /** 导出成 HTML；path 空 = 存数据目录 exports */
  exportHTML: (id: string, path = '') => post<string>(`/sessions/export/${id}`, { path }),
  /** 从某条用户消息之后重开：旧分支保留，返回新详情 */
  branch: (id: string, entryId: string) =>
    post<SessionDetailRESP>(`/sessions/branch/${id}`, { branch_entry_id: entryId }),
  /** 切回某个旧分支的叶子 */
  switchLeaf: (id: string, entryId: string) =>
    post<SessionDetailRESP>(`/sessions/leaf/${id}`, { leaf_entry_id: entryId }),
}

export const chat = {
  /** session_id 为空 = 新对话：后端会现场创建并回传 session_id */
  send: (body: { session_id?: string; content: string }) =>
    post<SendMessageRESP>('/chat/send', {
      chat_session_id: body.session_id,
      chat_text: body.content,
      chat_attachments: [],
    }),
  stop: (session_id: string) => post<boolean>('/chat/stop', { chat_session_id: session_id }),
  steer: (session_id: string, content: string) =>
    post<boolean>('/chat/steer', { chat_session_id: session_id, chat_text: content }),
  followUp: (session_id: string, content: string) =>
    post<boolean>('/chat/followup', { chat_session_id: session_id, chat_text: content }),
  /** 叫停正在进行的自动重试 */
  cancelRetry: (session_id: string) => post<boolean>('/chat/retry-cancel', { chat_session_id: session_id }),
  /** 手动压缩上下文 */
  compact: (session_id: string) => post<boolean>('/chat/compact', { chat_session_id: session_id }),
  /** 回答引擎抛回来的问题 */
  answer: (body: { id: string; value?: string; confirmed?: boolean; cancelled?: boolean }) =>
    post<boolean>('/chat/answer', {
      answer_ask_id: body.id,
      answer_value: body.value,
      answer_confirmed: body.confirmed,
      answer_cancelled: body.cancelled,
    }),
}

export const providers = {
  list: () => get<ProviderListRESP>('/providers'),
  upsert: (body: UpsertProviderREQ) => post<{ resp_provider_id: string }>('/providers/upsert', body),
  remove: (id: string) => post<boolean>('/providers/delete', { req_provider_id: id }),
  test: (id: string) => post<TestProviderRESP>('/providers/test', { req_provider_id: id }),
  /** 显式查看已保存的密钥（用户点「显示」时才调用） */
  reveal: (id: string) => post<RevealProviderRESP>('/providers/reveal', { req_provider_id: id }),
  setDefault: (provider: string, model: string) =>
    post<boolean>('/providers/default', { req_provider: provider, req_model: model }),
  /** 拉上游模型清单：编辑已存在的服务时可以只给 id 复用已存密钥 */
  fetchModels: (body: { api?: string; base_url?: string; api_key?: string; id?: string }) =>
    post<{ resp_models: string[] }>('/providers/models/fetch', body),
  /** 全部可用模型（跨 provider 平铺） */
  available: () => get<AvailableModelVO[]>('/models/available'),
}

export const skills = {
  list: () => get<SkillVO[]>('/skills'),
  create: (body: { name: string; description: string; body: string }) =>
    post<boolean>('/skills/create', {
      req_skill_name: body.name,
      req_skill_description: body.description,
      req_skill_body: body.body,
    }),
  import: (paths: string[]) => post<SkillImportRESP>('/skills/import', { req_paths: paths }),
  /** 导入 zip 技能包：data 是 zip 原始字节的 base64 */
  importZip: (filename: string, data: string) =>
    post<SkillImportRESP>('/skills/import-zip', { req_skill_filename: filename, req_skill_data: data }),
  remove: (id: string) => post<boolean>(`/skills/delete/${id}`),
  toggle: (id: string, enabled: boolean) => post<boolean>(`/skills/toggle/${id}`, { skill_enabled: enabled }),
  content: (id: string) => get<SkillContentRESP>(`/skills/content/${id}`),
}

/** 工作区文件浏览：path 为相对工作区的目录，空串即根目录 */
export const files = {
  list: (path = '') => get<FileListRESP>('/files', { path }),
  /** 单文件字节（文档预览用；后端限 32MB） */
  bytes: (path: string) => get<FileBytesRESP>('/files/bytes', { path }),
}

/** 斜杠命令清单：当前是启用技能的显式调用入口 */
export const commands = {
  list: () => get<CommandVO[]>('/commands'),
}

export const tools = {
  list: () => get<ToolVO[]>('/tools'),
  toggle: (name: string, enabled: boolean) =>
    post<boolean>('/tools/toggle', { req_tool_name: name, req_enabled: enabled }),
}

export const mcp = {
  list: () => get<MCPListRESP>('/mcp'),
  /** 保存配置原文：后端先校验再落盘，解析不过一个字符都不写 */
  save: (scope: string, raw: string) => post<boolean>('/mcp/save', { mcp_scope: scope, mcp_raw: raw }),
  remove: (name: string, scope: string) => post<boolean>('/mcp/remove', { mcp_name: name, mcp_scope: scope }),
  login: (name: string) => post<boolean>('/mcp/login', { mcp_name: name, mcp_scope: 'global' }),
  logout: (name: string) => post<boolean>('/mcp/logout', { mcp_name: name, mcp_scope: 'global' }),
  reload: () => post<boolean>('/mcp/reload'),
}

export const settings = {
  all: () => get<Record<string, string>>('/settings'),
  set: (key: string, value: string) => post<boolean>('/settings', { req_values: [{ setting_key: key, setting_value: value }] }),
  setMany: (pairs: { setting_key: string; setting_value: string }[]) => post<boolean>('/settings', { req_values: pairs }),
  background: () => get<BackgroundVO>('/settings/background'),
  setBackground: (path: string, opacity: number, blur: number) =>
    post<BackgroundVO>('/settings/background', {
      req_background_path: path,
      req_background_opacity: opacity,
      req_background_blur: blur,
    }),
}

/** 背景图本体：前端拿不到数据目录里的文件路径，只能走这个 URL */
export const backgroundUrl = () => `${getBaseURL()}/assets/background`

/** 上下文占用分解（context_view 数据源） */
export const contextBreakdown = (session_id = '') =>
  get<ContextBreakdownRESP>('/context/breakdown', pick({ session_id }))

/** 装载预设（工具/技能开关的命名集合） */
export interface LoadoutPresetVO {
  loadout_name: string
  loadout_tools: string[]
  loadout_skills: string[]
}
export const loadout = {
  list: () => get<{ resp_presets: LoadoutPresetVO[] }>('/loadout'),
  save: (name: string) => post<boolean>('/loadout/save', { loadout_name: name }),
  apply: (name: string) => post<boolean>('/loadout/apply', { loadout_name: name }),
  remove: (name: string) => post<boolean>('/loadout/delete', { loadout_name: name }),
}

/** 内省快照：工具/技能/命令/功能/系统提示词 */
export interface InspectSnapshotRESP {
  resp_tools: unknown[]
  resp_skills: unknown[]
  resp_commands: unknown[]
  resp_system_prompt: string
  resp_features: unknown[]
  resp_session: unknown | null
}
export const inspect = () => get<InspectSnapshotRESP>('/inspect')

/** 子代理角色预设（task 工具可选的角色与职责）。 */
export interface SubagentRoleVO {
  role_code: string
  role_name: string
  role_prompt: string
  role_tools: string[]
  role_max_turns: number
}

/** 子代理角色清单。 */
export const subagentRoles = () =>
  get<{ resp_roles: SubagentRoleVO[] }>('/subagents/roles')

/** 活跃子代理（正在跑的那些）；空 parentSessionId = 全部 */
export const agents = {
  /** 活跃清单 */
  list: (parentSessionId = '') =>
    get<{ resp_agents: AgentRunVO[] }>('/subagents', pick({ session_id: parentSessionId })),
  /** 一个子代理已累积的过程正文 */
  transcript: (id: string) => get<{ resp_transcript: string }>(`/subagents/transcript/${id}`),
}

/** 活跃子代理行（SubagentRunVO）。 */
export interface AgentRunVO {
  run_session_id: string
  run_parent_session_id: string
  run_role_code: string
  run_task: string
  run_started_at: number
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

  stage: (paths: string[], session_id = '') => post<GitStatusVO>('/git/stage', { git_paths: paths, git_session_id: session_id }),
  unstage: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/unstage', { git_paths: paths, git_session_id: session_id }),
  /** 丢弃工作区改动；必须显式给文件，后端不提供「全部丢弃」 */
  discard: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/discard', { git_paths: paths, git_session_id: session_id }),
  /** 把冲突文件标记为已解决（等同 add，但不代为提交） */
  resolve: (paths: string[], session_id = '') =>
    post<GitStatusVO>('/git/resolve', { git_paths: paths, git_session_id: session_id }),

  commit_: (message: string, session_id = '', amend = false) =>
    post<GitCommitRESP>('/git/commit', { git_message: message, git_amend: amend, git_session_id: session_id }),

  createBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/branch', { git_name: name, git_session_id: session_id }),
  switchBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/switch', { git_name: name, git_session_id: session_id }),
  deleteBranch: (name: string, session_id = '') =>
    post<GitStatusVO>('/git/branch/delete', { git_name: name, git_session_id: session_id }),
  /** 合并或变基；冲突时返回带 state 的状态而不是错误 */
  merge: (name: string, rebase: boolean, session_id = '') =>
    post<GitStatusVO>('/git/merge', { git_name: name, git_rebase: rebase, git_session_id: session_id }),
  /** 继续或中止未完成的合并 / 变基 */
  state: (action: 'continue' | 'abort', session_id = '') =>
    post<GitStatusVO>('/git/state', { git_action: action, git_session_id: session_id }),

  pull: (req: GitPullREQ, session_id = '') => post<GitStatusVO>('/git/pull', { ...req, git_session_id: session_id }),
  /** 取回远端最新状态；不改工作区也不改本地分支 */
  fetch: (remote = '', session_id = '') => post<GitStatusVO>('/git/fetch', { git_remote: remote, git_session_id: session_id }),
  push: (req: GitPushREQ = {}, session_id = '') =>
    post<GitStatusVO>('/git/push', { ...req, git_session_id: session_id }),

  init: (branch: string, session_id = '') => post<GitStatusVO>('/git/init', { git_branch: branch, git_session_id: session_id }),
  addRemote: (name: string, url: string, session_id = '') =>
    post<GitStatusVO>('/git/remote', { git_name: name, git_url: url, git_session_id: session_id }),

  draftMessage: (paths: string[], session_id = '') =>
    post<GitMessageRESP>('/git/message', { git_paths: paths, git_session_id: session_id }),
}

/** 消息回放形态（详情接口直接给数组，前端分页交给容器） */
export type { MessageVO }
