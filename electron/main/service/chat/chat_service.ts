//! 聊天编排：一个工作区一个 pi RPC 子进程；send/stop/steer/切换会话/状态组装。
import { logError, logInfo } from '../../pkg/log'
import { appError } from '../../pkg/apperror'
import { CODE_ENGINE_BUSY, CODE_SESSION_BIND } from '../../domain/errors'
import { currentWorkspace } from '../settings_service'
import { saveSettings } from '../../repo/settings_repo'
import { broadcast } from '../../sse/hub'
import { PiRpc, type PiRecord } from './rpc_client'
import { ChatStream } from './chat_frames'
import { buildHistory } from '../session_service'
import { toModelInfo } from '../../domain/types_chat'
import { PI_CAPS } from '../../domain/types_core'
import type {
  AskRequestVO,
  ChatAttachmentVO,
  ChatStateVO,
  QueuedMessagesVO,
  SlashCommandVO,
  UIResponseVO,
} from '../../domain/types_chat'

interface PiStateData {
  model?: unknown
  thinkingLevel?: string
  isStreaming?: boolean
  isCompacting?: boolean
  sessionFile?: string
  sessionId?: string
  sessionName?: string
  autoCompactionEnabled?: boolean
  messageCount?: number
}

interface PiStatsData {
  sessionId?: string
  userMessages?: number
  assistantMessages?: number
  toolCalls?: number
  totalMessages?: number
  tokens?: { input?: number; output?: number; cacheRead?: number; cacheWrite?: number; total?: number }
  cost?: number
  contextUsage?: { tokens?: number | null; contextWindow?: number; percent?: number | null }
}

let rpc: PiRpc | null = null
let rpc_ws = ''
let session_file = ''
let thinking_level = 'medium'
let levels: string[] = ['off', 'minimal', 'low', 'medium', 'high']
let commands: SlashCommandVO[] = []
let auto_compaction = true
let bind_err: string | undefined
let queued: QueuedMessagesVO = { steering: [], followUp: [], liveSteered: 0 }
const stream = new ChatStream()

/** 确保子进程在跑且绑定当前工作区。 */
async function ensureRpc(): Promise<PiRpc> {
  const ws = currentWorkspace()
  if (!ws) throw appError(CODE_SESSION_BIND, '请先选择工作区')
  if (rpc?.isAlive() && rpc_ws === ws) return rpc
  if (rpc) {
    rpc.stop()
    rpc = null
  }
  session_file = ''
  commands = []
  const client = new PiRpc(ws, {
    on_event: (record) => onPiEvent(record),
    on_ui_request: (record) => onUiRequest(record),
    on_exit: () => {
      rpc = null
      broadcast('engine:exit', { kind: 'pi', engine_kind: 'pi' })
      logInfo('pi 子进程退出')
    },
  })
  client.start()
  rpc = client
  rpc_ws = ws
  try {
    const state = (await client.request({ type: 'get_state' }, 60_000)) as PiStateData
    session_file = state.sessionFile ?? ''
    thinking_level = state.thinkingLevel ?? 'medium'
    auto_compaction = state.autoCompactionEnabled ?? true
    bind_err = undefined
    broadcast('engine:ready', { kind: 'pi', engine_kind: 'pi' })
    void refreshCommands(client)
    void refreshLevels(client)
  } catch (err) {
    bind_err = err instanceof Error ? err.message : String(err)
    logError(`pi 绑定失败：${bind_err}`)
  }
  return client
}

/** 子进程起好后的元数据刷新。 */
async function refreshCommands(client: PiRpc): Promise<void> {
  try {
    const data = (await client.request({ type: 'get_commands' }, 60_000)) as {
      commands?: { name?: string; source?: string; description?: string }[]
    }
    commands = (data.commands ?? [])
      .filter((c) => c.name)
      .map((c) => ({ name: String(c.name), source: String(c.source ?? 'prompt'), description: c.description }))
    broadcast('engine:commands', { commands, engine_kind: 'pi' })
  } catch {
    /* 拿不到就不展示 */
  }
}

async function refreshLevels(client: PiRpc): Promise<void> {
  try {
    const data = (await client.request({ type: 'get_available_thinking_levels' }, 60_000)) as {
      levels?: string[]
    }
    levels = data.levels ?? levels
  } catch {
    /* 保持默认 */
  }
}

/** pi 事件 → 翻译器。 */
function onPiEvent(record: PiRecord): void {
  const type = String(record.type ?? '')
  if (type === 'queue_update') {
    queued = {
      steering: asStrings(record.steering),
      followUp: asStrings(record.followUp),
      liveSteered: 0,
    }
  }
  stream.onEvent(record)
}

/** UI 请求：对话框转提问板，其余转通知。 */
function onUiRequest(record: PiRecord): void {
  const method = String(record.method ?? '')
  const id = String(record.id ?? '')
  const dialog = ['select', 'confirm', 'input', 'editor'].includes(method)
  if (dialog && id) {
    const ask: AskRequestVO = {
      id,
      method: method as AskRequestVO['method'],
      title: record.title != null ? String(record.title) : undefined,
      message: record.message != null ? String(record.message) : undefined,
      placeholder: record.placeholder != null ? String(record.placeholder) : undefined,
      prefill: record.prefill != null ? String(record.prefill) : undefined,
      options: Array.isArray(record.options) ? record.options.map(String) : undefined,
      timeout: typeof record.timeout === 'number' ? record.timeout : undefined,
    }
    broadcast('chat:ask', { ...ask, engine_kind: 'pi' })
    return
  }
  stream.onUiRequest(record)
}

function asStrings(v: unknown): string[] {
  return Array.isArray(v) ? v.map(String) : []
}

/** 聊天状态总装：pi 实况 + 会话文件回放。 */
export async function chatState(session?: string): Promise<ChatStateVO> {
  let client: PiRpc | null = null
  try {
    client = await ensureRpc()
  } catch (err) {
    return emptyState(err instanceof Error ? err.message : String(err))
  }
  try {
    if (session && session !== session_file) {
      await client.request({ type: 'switch_session', sessionPath: session }, 120_000)
      const state = (await client.request({ type: 'get_state' }, 60_000)) as PiStateData
      session_file = state.sessionFile ?? session
      remember(session_file)
    }
    const state = (await client.request({ type: 'get_state' }, 60_000)) as PiStateData
    session_file = state.sessionFile ?? session_file
    thinking_level = state.thinkingLevel ?? thinking_level
    auto_compaction = state.autoCompactionEnabled ?? auto_compaction
    const stats = (await client.request({ type: 'get_session_stats' }, 60_000).catch(() => null)) as
      | PiStatsData
      | null
    let history: ChatStateVO['history'] = []
    if (session_file) {
      try {
        history = buildHistory(session_file)
      } catch {
        history = []
      }
    }
    return {
      kind: 'pi',
      session: session_file,
      session_file,
      work_dir: rpc_ws,
      engine_running: client.isAlive(),
      streaming: stream.streaming || state.isStreaming === true,
      model: toModelInfo(state.model),
      thinking_level,
      levels,
      context: stats?.contextUsage
        ? {
            tokens: stats.contextUsage.tokens ?? null,
            contextWindow: stats.contextUsage.contextWindow ?? 0,
            percent: stats.contextUsage.percent ?? null,
          }
        : undefined,
      auto_compaction,
      queued,
      stats: stats
        ? {
            sessionId: stats.sessionId ?? '',
            userMessages: stats.userMessages ?? 0,
            assistantMessages: stats.assistantMessages ?? 0,
            toolCalls: stats.toolCalls ?? 0,
            totalMessages: stats.totalMessages ?? 0,
            tokens: {
              input: stats.tokens?.input ?? 0,
              output: stats.tokens?.output ?? 0,
              cacheRead: stats.tokens?.cacheRead ?? 0,
              cacheWrite: stats.tokens?.cacheWrite ?? 0,
              total: stats.tokens?.total ?? 0,
            },
            cost: stats.cost ?? 0,
            contextUsage: stats.contextUsage
              ? {
                  tokens: stats.contextUsage.tokens ?? null,
                  contextWindow: stats.contextUsage.contextWindow ?? 0,
                  percent: stats.contextUsage.percent ?? null,
                }
              : undefined,
          }
        : undefined,
      messages: [],
      history,
      commands,
      profile: '',
      caps: PI_CAPS,
      bind_err,
    }
  } catch (err) {
    return emptyState(err instanceof Error ? err.message : String(err))
  }
}

function remember(file: string): void {
  if (!file) return
  saveSettings({ 'prefs.last_session_file': file, 'prefs.last_engine_kind': 'pi' })
}

function emptyState(err: string): ChatStateVO {
  return {
    kind: 'pi',
    session: '',
    session_file: '',
    work_dir: '',
    engine_running: false,
    streaming: false,
    thinking_level: 'medium',
    levels,
    auto_compaction: true,
    messages: [],
    history: [],
    commands,
    profile: '',
    caps: PI_CAPS,
    bind_err: err,
  }
}

/** 把附件折进消息文本：图片走 images 字段，文件走路径引用。 */
function composeMessage(message: string, attachments: ChatAttachmentVO[]): { text: string; images: Record<string, string>[] } {
  let text = message
  const images: Record<string, string>[] = []
  for (const att of attachments ?? []) {
    if (att.kind === 'image' && att.image_base64) {
      images.push({ type: 'image', data: att.image_base64, mimeType: att.image_mime ?? 'image/png' })
    } else if (att.kind === 'file' && att.file_path) {
      text += `\n\n[附件] ${att.file_path}`
    }
  }
  return { text, images }
}

/** 发消息：登记运行 → 发 prompt；被拒则回 error 帧。 */
export async function chatSend(
  _engine: string,
  session: string,
  message: string,
  attachments: ChatAttachmentVO[],
): Promise<{ run_id: string }> {
  const client = await ensureRpc()
  if (session && session !== session_file) {
    await client.request({ type: 'switch_session', sessionPath: session }, 120_000)
    session_file = session
  }
  const { text, images } = composeMessage(message, attachments)
  const run_id = stream.begin(message)
  try {
    await client.request({
      type: 'prompt',
      message: text,
      ...(images.length ? { images } : {}),
    })
    remember(session_file)
    return { run_id }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    stream.streaming = false
    broadcast('chat:error', { run_id, message: msg, engine_kind: 'pi' })
    throw err
  }
}

/** 停止当前运行。 */
export async function chatStop(): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  await client.request({ type: 'abort' })
  return { ok: true }
}

/** 插话（steer）。 */
export async function chatSteer(message: string, attachments: ChatAttachmentVO[]): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  const { text, images } = composeMessage(message, attachments)
  await client.request({ type: 'steer', message: text, ...(images.length ? { images } : {}) })
  return { ok: true }
}

/** 追加排队消息（followUp）。 */
export async function chatFollowUp(message: string, attachments: ChatAttachmentVO[]): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  const { text, images } = composeMessage(message, attachments)
  await client.request({ type: 'follow_up', message: text, ...(images.length ? { images } : {}) })
  return { ok: true }
}

/** 清空队列。 */
export async function chatClearQueue(): Promise<QueuedMessagesVO> {
  const client = await ensureRpc()
  const data = (await client.request({ type: 'clear_queue' })) as { steering?: string[]; followUp?: string[] }
  queued = { steering: data.steering ?? [], followUp: data.followUp ?? [], liveSteered: 0 }
  return queued
}

/** 单条移除：pi 只支持整队清空，不单删。 */
export async function chatRemoveQueued(): Promise<{ removed: boolean }> {
  throw appError(CODE_ENGINE_BUSY, 'pi 不支持单条移除，请整队清空')
}

/** 手动压缩。 */
export async function chatCompact(instructions: string): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  const wire: Record<string, unknown> = { type: 'compact' }
  if (instructions.trim()) wire.customInstructions = instructions.trim()
  await client.request(wire, 10 * 60_000)
  return { ok: true }
}

/** 回答内核提问。 */
export async function chatAnswer(_session: string, request: UIResponseVO): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  const wire: Record<string, unknown> = { type: 'extension_ui_response', id: request.id }
  if (request.answers?.length) {
    const ans = request.answers[0]
    const value = ans.selectedOptions.length > 0 ? ans.selectedOptions.join(', ') : (ans.customInput ?? '')
    if (value) wire.value = value
    else wire.cancelled = true
  } else if (request.value != null) {
    wire.value = request.value
  } else if (request.confirmed != null) {
    wire.confirmed = request.confirmed
  } else {
    wire.cancelled = true
  }
  client.send(wire)
  return { ok: true }
}

/** 新会话。 */
export async function chatNew(): Promise<{ session_file: string }> {
  const client = await ensureRpc()
  await client.request({ type: 'new_session' }, 60_000)
  const state = (await client.request({ type: 'get_state' }, 60_000)) as PiStateData
  session_file = state.sessionFile ?? ''
  remember(session_file)
  broadcast('session:info', { engine_kind: 'pi' })
  return { session_file }
}

/** 克隆会话。 */
export async function chatClone(): Promise<{ session_file: string }> {
  const client = await ensureRpc()
  await client.request({ type: 'clone' }, 10 * 60_000)
  const state = (await client.request({ type: 'get_state' }, 60_000)) as PiStateData
  session_file = state.sessionFile ?? ''
  remember(session_file)
  return { session_file }
}

/** 切模型。 */
export async function chatSetModel(provider: string, model: string): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  await client.request({ type: 'set_model', provider, modelId: model })
  return { ok: true }
}

/** 切思考等级。 */
export async function chatSetThinking(level: string): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  await client.request({ type: 'set_thinking_level', level })
  thinking_level = level
  return { ok: true }
}

/** 自动压缩开关。 */
export async function chatSetAutoCompaction(enabled: boolean): Promise<{ ok: boolean }> {
  const client = await ensureRpc()
  await client.request({ type: 'set_auto_compaction', enabled })
  auto_compaction = enabled
  return { ok: true }
}

/** 引擎重启：杀子进程，下次用到再拉。 */
export function chatRestart(): void {
  rpc?.stop()
  rpc = null
  rpc_ws = ''
  stream.streaming = false
  broadcast('engine:status', { engine_kind: 'pi' })
}

/** 工作区切换时调用：绑定失效即重启。 */
export function chatOnWorkspaceChanged(): void {
  const ws = currentWorkspace()
  if (rpc_ws && ws !== rpc_ws) chatRestart()
}

/** 运行实例信息（引擎面板用）。 */
export function chatRunInfo(): {
  pid: number
  session_id: string
  running: boolean
  protocol: string
  started_at: number
  call_count: number
  busy: boolean
} {
  return {
    pid: rpc?.pid ?? 0,
    session_id: session_file,
    running: Boolean(rpc?.isAlive()),
    protocol: 'pi-rpc/1',
    started_at: rpc?.started_at ?? 0,
    call_count: rpc?.call_count ?? 0,
    busy: stream.streaming,
  }
}
