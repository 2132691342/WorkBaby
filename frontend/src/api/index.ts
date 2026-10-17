// 前端 API 层：路径与 backend/server/routes.go 一一对应，字段名照后端 json tag。
import { get, getBaseURL, post } from './http'
import type {
  ApprovalVO,
  ArchEntrypointVO,
  ArchGraph,
  ArchiveVO,
  AttachmentREQ,
  BackgroundVO,
  BaselineVO,
  BootstrapVO,
  CompareVO,
  EvalVO,
  FileListVO,
  GitDiffVO,
  GitStatusVO,
  HelpDocRESP,
  HelpDocVO,
  HistoryEntry,
  HistoryScanRESP,
  KnowledgeDocVO,
  MCPServerVO,
  MCPConfigRESP,
  MCPTemplateVO,
  MCPToolVO,
  PiEnvVO,
  ListMemoriesRESP,
  MemoryVO,
  ModelCapability,
  ModelConfigVO,
  ProviderVO,
  RuntimeInfo,
  ReindexJobVO,
  SaveMCPConfigREQ,
  SearchHitVO,
  SendMessageRESP,
  SessionDetailVO,
  SessionVO,
  SkillVO,
  StatsRESP,
  RunVO,
  RunDetailVO,
  CheckpointVO,
  ToolVO,
  UpsertModelConfigREQ,
} from '../types/api'

/** 丢掉空值再拼查询串：不带空参数，后端「没给」与「给了空串」就不用分开判 */
function pick(src: Record<string, string>): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(src)) if (v) out[k] = v
  return Object.keys(out).length ? out : undefined
}

export const bootstrap = () => get<BootstrapVO>('/bootstrap')

/** 路径约定：变量参数一律落在路径末尾（`/{resource}/{action}/:id`） */
export const sessions = {
  list: (offset = 0) => get<SessionVO[]>('/sessions', offset > 0 ? { offset } : undefined),
  create: (body: { title?: string; workspace?: string; provider_id?: string; model?: string }) =>
    post<SessionVO>('/sessions', body),
  detail: (id: string) => get<SessionDetailVO>(`/sessions/${id}`),
  rename: (id: string, title: string) => post<boolean>(`/sessions/rename/${id}`, { title }),
  remove: (id: string) => post<boolean>(`/sessions/delete/${id}`),
  setModel: (id: string, provider_id: string, model: string) =>
    post<boolean>(`/sessions/model/${id}`, { provider_id, model }),
  setPermission: (id: string, permission: string) =>
    post<boolean>(`/sessions/permission/${id}`, { permission }),
  setWorkspace: (id: string, workspace: string) =>
    post<boolean>(`/sessions/workspace/${id}`, { workspace }),
  setThinking: (id: string, level: string) =>
    post<boolean>(`/sessions/thinking/${id}`, { level }),
  /** 从某条历史分叉成新会话：PI 没有「改 leaf」的协议面，所以是分叉而不是回退 */
  fork: (id: string, entry_id: string) => post<SessionVO>(`/sessions/fork/${id}`, { entry_id }),
}

export const chat = {
  send: (session_id: string, content: string, attachments?: AttachmentREQ[]) =>
    post<SendMessageRESP>('/chat/send', { session_id, content, attachments }),
  stop: (session_id: string) => post<boolean>('/chat/stop', { session_id }),
  steer: (session_id: string, content: string) =>
    post<boolean>('/chat/steer', { session_id, content }),
  followUp: (session_id: string, content: string) =>
    post<boolean>('/chat/followup', { session_id, content }),
}

export const approvals = {
  pending: (session_id: string) => get<ApprovalVO[]>(`/approvals?session_id=${session_id}`),
  /** allow / deny 共用一个端点：动作走请求体，id 才能落在路径末尾。
   *  session_id 必须带上：决策要回给发出请求的那个 PI 进程。 */
  decide: (id: string, session_id: string, action: 'allow' | 'deny', scope = 'once') =>
    post<boolean>(`/approvals/decide/${id}`, { session_id, action, scope }),
}

export const providers = {
  list: () => get<ProviderVO[]>('/providers'),
  upsert: (body: Record<string, unknown>) => post<ProviderVO>('/providers', body),
  update: (id: string, body: Record<string, unknown>) => post<ProviderVO>(`/providers/update/${id}`, body),
  remove: (id: string) => post<boolean>(`/providers/delete/${id}`),
  test: (id: string) => post<{ ok: boolean; model: string; detail: string }>(`/providers/test/${id}`, { id }),
  /** 显式查看已保存的密钥（用户点「显示」时才调用） */
  reveal: (id: string) => post<{ api_key: string }>(`/providers/reveal/${id}`),
  setDefault: (id: string) => post<boolean>(`/providers/default/${id}`),
  models: (provider_id: string) => get<string[]>(`/providers/models?provider_id=${provider_id}`),
  /** 新增服务还没保存时也能拉：直接把连接信息发给后端 */
  fetchModels: (body: { api: string; base_url: string; api_key?: string }) =>
    post<string[]>('/providers/models/fetch', body),
}

export const skills = {
  list: () => get<SkillVO[]>('/skills'),
  create: (body: { name: string; description: string; body: string }) =>
    post<SkillVO>('/skills', body),
  import: (paths: string[]) =>
    post<{ imported: number; skipped: string[] }>('/skills/import', { paths }),
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

export const knowledge = {
  docs: () => get<KnowledgeDocVO[]>('/knowledge/docs'),
  add: (paths: string[]) => post<{ added: number }>('/knowledge/docs/add', { paths }),
  remove: (id: string) => post<boolean>(`/knowledge/docs/delete/${id}`),
  /** 启动后台重建：大库要跑几十秒，同步等一个请求既占连接也没法停 */
  reindex: () => post<{ started: boolean }>('/knowledge/reindex'),
  reindexStatus: () => get<ReindexJobVO>('/knowledge/reindex'),
  cancelReindex: () => post<boolean>('/knowledge/reindex/cancel'),
  search: (query: string, limit = 5) =>
    post<{ hits: SearchHitVO[] }>('/knowledge/search', { query, limit }),
}

export const memories = {
  list: () => get<ListMemoriesRESP>('/memories'),
  add: (content: string) => post<MemoryVO>('/memories', { content }),
  /** content 留空表示只改开关 */
  update: (id: string, patch: { content?: string; enabled?: boolean }) =>
    post<MemoryVO>(`/memories/update/${id}`, patch),
  remove: (id: string) => post<boolean>(`/memories/delete/${id}`),
}

export const settings = {
  all: () => get<Record<string, string>>('/settings'),
  set: (key: string, value: string) => post<boolean>('/settings', { key, value }),
  background: () => get<BackgroundVO>('/settings/background'),
  setBackground: (path: string, opacity: number, blur: number) =>
    post<BackgroundVO>('/settings/background', { path, opacity, blur }),
}

/** 背景图本体：前端拿不到数据目录里的文件路径，只能走这个 URL */
export const backgroundUrl = () => `${getBaseURL()}/assets/background`

export const tools = {
  list: () => get<ToolVO[]>('/tools'),
  toggle: (name: string, enabled: boolean) => post<boolean>(`/tools/toggle/${name}`, { enabled }),
}

export const models = {
  capability: (model: string, provider_id?: string) =>
    get<ModelCapability>(
      `/models/capability?model=${encodeURIComponent(model)}${provider_id ? `&provider_id=${provider_id}` : ''}`,
    ),
  /** 批量能力：换模型下拉一次列几十上百个模型，逐个查会打出 N+1 */
  capabilities: (provider_id: string, models: string[]) =>
    post<ModelCapability[]>('/models/capabilities', { provider_id, models }),
  /** 一次取回某服务下全部模型的覆写：展开参数面板时用它，避免逐个 models/config 打成 N+1 */
  configs: (provider_id?: string) =>
    get<ModelConfigVO[]>(`/models/configs${provider_id ? `?provider_id=${provider_id}` : ''}`),
  saveConfig: (body: UpsertModelConfigREQ) => post<ModelConfigVO>('/models/config', body),
}

export const runtime = {
  status: () => get<RuntimeInfo>('/runtime'),
  /** 重新探测：后端先清探测缓存再跑一遍，运行期放入归档后点这里生效 */
  redetect: () => post<RuntimeInfo>('/runtime/redetect'),
}

/** 环境引导：闸门快照与 PI 的安装更新（specs/26） */
export const env = {
  status: () => get<PiEnvVO>('/env'),
  /** 安装 PI：进度走 chat:setup 事件流式回传，接口立刻返回 */
  install: () => post<boolean>('/env/install'),
  update: () => post<boolean>('/env/update'),
  recheck: () => post<PiEnvVO>('/env/recheck'),
}

export const stats = (days: number) => get<StatsRESP>('/stats', { days })

/** 内置帮助文档（只读，随应用打包） */
export const docs = {
  list: () => get<HelpDocVO[]>('/docs'),
  content: (name: string) => get<HelpDocRESP>(`/docs/${name}`),
}

/** 工作区版本状态：只读，path 为空表示整个工作区。
 *  带 session_id 时看该会话的工作目录——会话级目录才是这次改动真正发生的地方 */
export const git = {
  status: (session_id = '') => get<GitStatusVO>('/git/status', pick({ session_id })),
  diff: (path = '', session_id = '') => get<GitDiffVO>('/git/diff', pick({ path, session_id })),
}

/* ================= 指挥台 API（specs/16-21） ================= */

export const mcp = {
  /** 读回配置原文、解析结果与解析错误（界面左侧编辑、右侧对照） */
  config: () => get<MCPConfigRESP>('/mcp/config'),
  /** 保存原文：后端先校验再落盘，成功即重载；解析不过不写文件 */
  save: (body: SaveMCPConfigREQ) => post<MCPConfigRESP>('/mcp/config', body),
  /** 内置模板三档（blank / stdio / example），一次给全 */
  templates: () => get<MCPTemplateVO[]>('/mcp/template'),
  /** 按当前文件重新拉起全部服务 */
  reload: () => post<MCPConfigRESP>('/mcp/reload'),
  /** 配置里的服务条目（含探测状态与工具数） */
  servers: () => get<MCPServerVO[]>('/mcp/servers'),
  /** 即时探测：连一次拉工具清单 */
  probe: (name: string) => post<MCPToolVO[]>('/mcp/servers/probe', { name }),
  tools: () => get<MCPToolVO[]>('/mcp/tools'),
}

/** 运行观测：这一轮干了什么、花了多少、从哪能回去（specs/23）。 */
export const runs = {
  list: (session_id = '', limit = 50) =>
    get<RunVO[]>('/runs', pick({ session_id, limit: String(limit) })),
  detail: (id: string) => get<RunDetailVO>(`/runs/${id}`),
}

export const checkpoints = {
  list: (session_id: string) => get<CheckpointVO[]>('/checkpoints', { session_id }),
  /** 回到某轮：把会话当前位点移回去，不删任何条目 */
  restore: (session_id: string, id: string) =>
    post<null>(`/checkpoints/restore/${id}`, { session_id }),
  /** 从某轮分叉出一条新会话：原会话完整保留 */
  fork: (session_id: string, id: string) =>
    post<SessionVO>(`/checkpoints/fork/${id}`, { session_id }),
}

/** 评估与回归：一次对话打几分，与基线比差在哪。 */
export const evals = {
  list: (scope?: string, scopeId?: string) =>
    get<EvalVO[]>('/evals', pick({ scope: scope || '', scope_id: scopeId || '' })),
  register: (body: { run_id: string; score: number; note?: string }) =>
    post<EvalVO>('/evals', body),
  baselines: () => get<BaselineVO[]>('/evals/baselines'),
  saveBaseline: (evalId: string, scope: string) =>
    post<BaselineVO>('/evals/baselines', { eval_id: evalId, scope }),
  removeBaseline: (id: string) => post<null>(`/evals/baselines/delete/${id}`),
  compare: (baselineId: string, runId: string) =>
    get<CompareVO>('/evals/compare', pick({ baseline_id: baselineId, run_id: runId })),
}

/** 归档：按保留期回收过期的隐藏会话（只丢逐字对话，留结论 / 时间线 / 用量账） */
export const archive = {
  now: () => post<ArchiveVO>('/archive'),
}
export const history = {
  scan: () => get<HistoryScanRESP>('/history/entries'),
  read: (id: string) => get<HistoryEntry>('/history/entry', { id }),
  folders: () => get<Record<string, string>>('/history/folders'),
  addFolder: (source: string, path: string) => post<null>('/history/folders', { source, path }),
  removeFolder: (source: string) => post<null>('/history/folders/delete', { source }),
  continueChat: (entryId: string, next = '') =>
    post<SessionVO>('/history/continue', { entry_id: entryId, next }),
}

export const archmap = {
  analyze: (path = '') => post<ArchGraph>('/archmap/analyze', { path }),
  entrypoints: (path = '') => get<ArchEntrypointVO[]>('/archmap/entrypoints', pick({ path })),
}
