//! 服务商与模型：pi 的 models.json / auth.json 读写、连通性探测与模型清单。
import fs from 'node:fs'
import path from 'node:path'
import { paths } from '../context'
import { readJson, writeJson } from '../pkg/fsutil'
import { appError, wrapError } from '../pkg/apperror'
import { CODE_PROVIDER_BAD, CODE_PROVIDER_FETCH, CODE_PROVIDER_TEST, CODE_PROVIDER_LOGIN } from '../domain/errors'
import type {
  LoginProviderVO,
  ModelEntryVO,
  ModelRowVO,
  ProviderProbeREQ,
  ProviderProbeRESP,
  ProviderTestRESP,
  ProviderViewVO,
  ProvidersViewVO,
} from '../domain/types_data'

/** models.json 的服务商条目。 */
interface ProviderEntry {
  name?: string
  baseUrl: string
  api: string
  models: ModelEntryVO[]
  [key: string]: unknown
}

interface ModelsFile {
  providers: Record<string, ProviderEntry>
}

interface AuthFile {
  [provider: string]: { type: string; key: string; [key: string]: unknown }
}

function modelsPath(): string {
  return path.join(paths().wb_pi_agent_dir, 'models.json')
}

function authPath(): string {
  return path.join(paths().wb_pi_agent_dir, 'auth.json')
}

function loadModels(): ModelsFile {
  const raw = readJson<ModelsFile>(modelsPath())
  return { providers: raw?.providers ?? {} }
}

function saveModels(data: ModelsFile): void {
  writeJson(modelsPath(), data)
}

function loadAuth(): AuthFile {
  return readJson<AuthFile>(authPath()) ?? {}
}

function saveAuth(data: AuthFile): void {
  writeJson(authPath(), data)
  try {
    fs.chmodSync(authPath(), 0o600)
  } catch {
    /* Windows 上 chmod 意义有限，失败忽略 */
  }
}

/** pi 内置服务商清单（baseUrl 供表单预填）。 */
const BUILTIN_PROVIDERS: { id: string; label: string; base_url: string; api: string }[] = [
  { id: 'anthropic', label: 'Anthropic', base_url: 'https://api.anthropic.com', api: 'anthropic-messages' },
  { id: 'openai', label: 'OpenAI', base_url: 'https://api.openai.com/v1', api: 'openai-completions' },
  { id: 'google', label: 'Google', base_url: 'https://generativelanguage.googleapis.com', api: 'google-generative-ai' },
  { id: 'openrouter', label: 'OpenRouter', base_url: 'https://openrouter.ai/api/v1', api: 'openai-completions' },
  { id: 'deepseek', label: 'DeepSeek', base_url: 'https://api.deepseek.com/v1', api: 'openai-completions' },
  { id: 'groq', label: 'Groq', base_url: 'https://api.groq.com/openai/v1', api: 'openai-completions' },
  { id: 'xai', label: 'xAI', base_url: 'https://api.x.ai/v1', api: 'openai-completions' },
]

/** 服务商总览：自定义 + 内置 + 凭据状态合并。 */
export function providersView(): ProvidersViewVO {
  const models = loadModels()
  const auth = loadAuth()
  const rows: ProviderViewVO[] = []
  for (const [name, entry] of Object.entries(models.providers)) {
    rows.push({
      engine_kind: 'pi',
      provider_name: name,
      base_url: entry.baseUrl ?? '',
      api: entry.api ?? 'openai-completions',
      auth: 'api_key',
      api_key: auth[name] ? '********' : '',
      has_key: Boolean(auth[name]),
      models: entry.models ?? [],
    })
  }
  for (const b of BUILTIN_PROVIDERS) {
    if (rows.some((r) => r.provider_name === b.id)) continue
    rows.push({
      engine_kind: 'pi',
      provider_name: b.id,
      base_url: b.base_url,
      api: b.api,
      auth: 'api_key',
      api_key: auth[b.id] ? '********' : '',
      has_key: Boolean(auth[b.id]),
      models: [],
    })
  }
  rows.sort((a, b) => a.provider_name.localeCompare(b.provider_name))
  return { providers: rows, config_path: modelsPath() }
}

/** 新增或更新服务商：写 models.json + 可选写 key。 */
export function providerUpsert(
  name: string,
  patch: Partial<ProviderViewVO>,
): ProvidersViewVO {
  const key = name.trim()
  if (!key) throw appError(CODE_PROVIDER_BAD, '服务商名不能为空')
  const data = loadModels()
  const prev = data.providers[key]
  const models = patch.models ?? prev?.models ?? []
  const base_url = patch.base_url?.trim() || prev?.baseUrl
  if (!base_url) throw appError(CODE_PROVIDER_BAD, 'Base URL 不能为空')
  data.providers[key] = {
    name: patch.provider_name?.trim() || prev?.name || key,
    baseUrl: base_url,
    api: patch.api || prev?.api || 'openai-completions',
    models,
  }
  saveModels(data)
  if (patch.api_key?.trim() && patch.api_key !== '********') {
    const auth = loadAuth()
    auth[key] = { type: 'api_key', key: patch.api_key.trim() }
    saveAuth(auth)
  }
  return providersView()
}

/** 删除服务商：models.json 条目与凭据一并清。 */
export function providerDelete(name: string): ProvidersViewVO {
  const data = loadModels()
  delete data.providers[name]
  saveModels(data)
  const auth = loadAuth()
  delete auth[name]
  saveAuth(auth)
  return providersView()
}

/** 模型清单：自定义服务商的模型平铺。 */
export function modelsList(): { models: ModelRowVO[] } {
  const data = loadModels()
  const rows: ModelRowVO[] = []
  for (const [provider, entry] of Object.entries(data.providers)) {
    for (const m of entry.models ?? []) {
      rows.push({
        kind: 'pi',
        provider,
        id: m.model_id,
        name: m.model_name ?? m.model_id,
        api: entry.api,
        context_window: m.context_window ?? 128_000,
        max_tokens: m.max_tokens ?? 8192,
        reasoning: m.reasoning ?? false,
        input: m.input,
      })
    }
  }
  return { models: rows }
}

/** 取生效的 API Key：请求里给了就用，没给看凭据文件。 */
function effectiveKey(req: ProviderProbeREQ): string {
  if (req.api_key?.trim() && req.api_key !== '********') return req.api_key.trim()
  if (req.provider_name) {
    const auth = loadAuth()
    return auth[req.provider_name]?.key ?? ''
  }
  return ''
}

/** 按协议探测模型清单。 */
export async function providerProbe(req: ProviderProbeREQ): Promise<ProviderProbeRESP> {
  const base = req.base_url?.trim().replace(/\/$/, '')
  const api = req.api || 'openai-completions'
  if (!base) throw appError(CODE_PROVIDER_BAD, 'Base URL 不能为空')
  const key = effectiveKey(req)
  let entries: ModelEntryVO[] = []
  try {
    if (api === 'anthropic-messages') {
      entries = await probeAnthropic(base, key)
    } else if (api === 'google-generative-ai') {
      entries = await probeGoogle(base, key)
    } else {
      entries = await probeOpenAI(base, key)
    }
  } catch (err) {
    throw wrapError(CODE_PROVIDER_FETCH, '模型清单拉取失败', err)
  }
  return { models: entries, config_path: modelsPath() }
}

/** 连通性测试：拉一次清单并数个数。 */
export async function providerTest(req: ProviderProbeREQ): Promise<ProviderTestRESP> {
  try {
    const r = await providerProbe(req)
    return { ok: true, model_count: r.models.length, message: `连通正常，发现 ${r.models.length} 个模型` }
  } catch (err) {
    const raw = err instanceof Error ? err.message : String(err)
    return { ok: false, model_count: 0, message: raw }
  }
}

async function fetchJson(url: string, headers: Record<string, string>): Promise<unknown> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 20_000)
  try {
    const res = await fetch(url, { headers, signal: controller.signal })
    const text = await res.text()
    if (!res.ok) throw new Error(`HTTP ${res.status}：${text.slice(0, 300)}`)
    return JSON.parse(text) as unknown
  } finally {
    clearTimeout(timer)
  }
}

async function probeOpenAI(base: string, key: string): Promise<ModelEntryVO[]> {
  const data = (await fetchJson(`${base}/models`, {
    Authorization: `Bearer ${key}`,
  })) as { data?: { id?: string }[] }
  return (data.data ?? [])
    .filter((m) => typeof m.id === 'string')
    .map((m) => ({ model_id: m.id as string, api: 'openai-completions' }))
}

async function probeAnthropic(base: string, key: string): Promise<ModelEntryVO[]> {
  const data = (await fetchJson(`${base}/v1/models`, {
    'x-api-key': key,
    'anthropic-version': '2023-06-01',
  })) as { data?: { id?: string; display_name?: string }[] }
  return (data.data ?? [])
    .filter((m) => typeof m.id === 'string')
    .map((m) => ({
      model_id: m.id as string,
      model_name: m.display_name,
      api: 'anthropic-messages',
    }))
}

async function probeGoogle(base: string, key: string): Promise<ModelEntryVO[]> {
  const data = (await fetchJson(`${base}/v1beta/models?key=${encodeURIComponent(key)}`, {})) as {
    models?: { name?: string; displayName?: string }[]
  }
  return (data.models ?? [])
    .filter((m) => typeof m.name === 'string')
    .map((m) => ({
      model_id: (m.name as string).replace(/^models\//, ''),
      model_name: m.displayName,
      api: 'google-generative-ai',
    }))
}

/** 登录服务商：pi 的 OAuth 走 TUI，本壳只走 API Key。 */
export function loginProviders(): { providers: LoginProviderVO[] } {
  return { providers: [] }
}

/** 发起登录：本壳不支持 OAuth 流，直接提示走模型页填 Key。 */
export function loginStart(_provider: string): { ok: boolean } {
  throw appError(CODE_PROVIDER_LOGIN, '请在本页直接填写 API Key，不走登录流')
}

/** 有凭据的服务商数（健康面板用）。 */
export function authedProviderCount(): number {
  return Object.keys(loadAuth()).length
}
