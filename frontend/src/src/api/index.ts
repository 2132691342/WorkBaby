import { get, post } from './http'
import type {
  ApprovalVO,
  AttachmentREQ,
  BootstrapVO,
  KnowledgeDocVO,
  ModelCapability,
  ModelConfigVO,
  ProviderVO,
  RuntimeInfo,
  SearchHitVO,
  SendMessageRESP,
  SessionDetailVO,
  SessionVO,
  SkillVO,
  StatsRESP,
  ToolVO,
  UpsertModelConfigREQ,
} from '../types/api'

export const bootstrap = () => get<BootstrapVO>('/bootstrap')

export const sessions = {
  list: (offset = 0) => get<SessionVO[]>('/sessions', offset > 0 ? { offset } : undefined),
  create: (body: { title?: string; workspace?: string; provider_id?: string; model?: string }) =>
    post<SessionVO>('/sessions', body),
  detail: (id: string) => get<SessionDetailVO>(`/sessions/${id}`),
  rename: (id: string, title: string) => post<boolean>(`/sessions/${id}/rename`, { title }),
  remove: (id: string) => post<boolean>(`/sessions/${id}/delete`),
  setModel: (id: string, provider_id: string, model: string) =>
    post<boolean>(`/sessions/${id}/model`, { provider_id, model }),
  setPermission: (id: string, permission: string) =>
    post<boolean>(`/sessions/${id}/permission`, { permission }),
  setWorkspace: (id: string, workspace: string) =>
    post<boolean>(`/sessions/${id}/workspace`, { workspace }),
  branch: (id: string, entry_id: string) => post<boolean>(`/sessions/${id}/branch`, { entry_id }),
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
  approve: (id: string, scope = 'once') => post<boolean>(`/approvals/${id}/approve`, { scope }),
  deny: (id: string) => post<boolean>(`/approvals/${id}/deny`, {}),
}

export const providers = {
  list: () => get<ProviderVO[]>('/providers'),
  upsert: (body: Record<string, unknown>) => post<ProviderVO>('/providers', body),
  update: (id: string, body: Record<string, unknown>) => post<ProviderVO>(`/providers/${id}/update`, body),
  remove: (id: string) => post<boolean>(`/providers/${id}/delete`),
  test: (id: string) => post<{ ok: boolean; model: string; detail: string }>(`/providers/${id}/test`, { id }),
  /** 显式查看已保存的密钥（用户点「显示」时才调用） */
  reveal: (id: string) => post<{ api_key: string }>(`/providers/${id}/reveal`),
  setDefault: (id: string) => post<boolean>(`/providers/${id}/default`),
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
  remove: (id: string) => post<boolean>(`/skills/${id}/delete`),
  toggle: (id: string, enabled: boolean) => post<boolean>(`/skills/${id}/toggle`, { enabled }),
  content: (id: string) => get<{ content: string }>(`/skills/${id}/content`),
}

export const knowledge = {
  docs: () => get<KnowledgeDocVO[]>('/knowledge/docs'),
  add: (paths: string[]) => post<{ added: number }>('/knowledge/docs/add', { paths }),
  remove: (id: string) => post<boolean>(`/knowledge/docs/${id}/delete`),
  reindex: () => post<{ reindexed: number }>('/knowledge/reindex'),
  search: (query: string, limit = 5) =>
    post<{ hits: SearchHitVO[] }>('/knowledge/search', { query, limit }),
}

export const settings = {
  all: () => get<Record<string, string>>('/settings'),
  set: (key: string, value: string) => post<boolean>('/settings', { key, value }),
}

export const tools = {
  list: () => get<ToolVO[]>('/tools'),
  toggle: (name: string, enabled: boolean) => post<boolean>(`/tools/${name}/toggle`, { enabled }),
}

export const models = {
  capability: (model: string, provider_id?: string) =>
    get<ModelCapability>(
      `/models/capability?model=${encodeURIComponent(model)}${provider_id ? `&provider_id=${provider_id}` : ''}`,
    ),
  /** 批量能力：换模型下拉一次列几十上百个模型，逐个查会打出 N+1 */
  capabilities: (provider_id: string, models: string[]) =>
    post<ModelCapability[]>('/models/capabilities', { provider_id, models }),
  config: (model: string, provider_id?: string) =>
    get<ModelConfigVO>(
      `/models/config?model=${encodeURIComponent(model)}${provider_id ? `&provider_id=${provider_id}` : ''}`,
    ),
  configs: (provider_id?: string) =>
    get<ModelConfigVO[]>(`/models/configs${provider_id ? `?provider_id=${provider_id}` : ''}`),
  saveConfig: (body: UpsertModelConfigREQ) => post<ModelConfigVO>('/models/config', body),
}

export const runtime = {
  status: () => get<RuntimeInfo>('/runtime'),
  /** 重新探测：后端先清探测缓存再跑一遍，运行期放入归档后点这里生效 */
  redetect: () => post<RuntimeInfo>('/runtime/redetect'),
}

export const stats = (days: number) => get<StatsRESP>('/stats', { days })
