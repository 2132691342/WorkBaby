import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { streamChat, type StreamHandle } from '@/api/stream'
import { mergeLoadedMessages, resolveRunAssistant, toUiMessages } from '@/chat/models/merge'
import { streamingBlocksToMessageBlocks, type StreamingBlock } from '@/chat/models/streamingBlocks'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type {
  ApprovalPending,
  PendingApproval,
  Artifact,
  ArtifactPayload,
  AvailableModel,
  BackgroundTask,
  ChatStats,
  ChatStreamEvent,
  CircuitState,
  CompactREQ,
  CompactResultRESP,
  ContextUsageRESP,
  EffectiveParams,
  FileChange,
  Message as ApiMessage,
  Page,
  Session,
  SessionGoal,
  SkillHit,
  SlashCommand,
  TodoStateRESP
} from '@/types/api'
import type { UiNode } from '@/components/genui/GenUiRenderer.vue'
import {
  applyStreamUpdate,
  type ApprovalRequest,
  type StreamEventUpdate,
  type ToolCallInfo
} from './chat/ChatStreamDecoder'
import { StreamEventBatcher } from './chat/StreamEventBatcher'

// ToolCallInfo / ApprovalRequest 类型从 decoder 模块 re-export（保持外部 import 兼容）。
export type { ToolCallInfo, ApprovalRequest }

/** 前端权限语义级别（后端 SessionMode 映射：restricted→restricted、confirm→default、auto→auto_edit、full→yolo）。 */
export type PermissionLevel = 'restricted' | 'confirm' | 'auto' | 'full'

/** 权限语义 ↔ 后端 SessionMode 双向映射（仅此一处，杜绝组件层散落字符串）。 */
export const PERMISSION_MODE_MAP: Record<PermissionLevel, string> = {
  restricted: 'restricted',
  confirm: 'default',
  auto: 'auto_edit',
  full: 'yolo'
}

export function permissionModeToBackend(level: PermissionLevel): string {
  return PERMISSION_MODE_MAP[level] ?? 'default'
}

export function backendModeToPermission(mode: string | null | undefined): PermissionLevel {
  switch (mode) {
    case 'restricted':
      return 'restricted'
    case 'auto_edit':
      return 'auto'
    case 'yolo':
      return 'full'
    default:
      return 'confirm'
  }
}

/**
 * Chat store：会话列表与当前会话、消息、流式状态、发送与取消、审批与上下文占用。
 * 不变式：currentID 一经设置，messages 恒为该会话最新消息；streaming 为真时以流式状态为准。
 */
export const useChatStore = defineStore('chat', () => {
  const sessions = ref<Session[]>([])
  const currentID = ref<string | null>(null)
  const messages = ref<ApiMessage[]>([])
  const models = ref<AvailableModel[]>([])
  /** 所有模型的熔断状态（Map<id, CircuitState>）。 */
  const circuitStates = ref<Map<string, CircuitState>>(new Map())
  const selectedModelID = ref<string | null>(null)

  const loadingSessions = ref(false)
  const streaming = ref(false)
  const streamingContent = ref('')
  const streamingThinking = ref('')
  /**
   * 流式累积的块序列（按事件到达顺序），**单一真相源**。
   *
   * <p>正文、思考、工具调用与结果全部按到达顺序进这一个数组，渲染层直接消费。
   * 曾并存一份只装工具的 streamingTools，是靠事件先后顺序拼出的第二份状态——
   * 两份状态在「工具与正文穿插」时必然对不齐，已移除。
   */
  const streamingBlocks = ref<StreamingBlock[]>([])
  const streamingStats = ref<ChatStats | null>(null)
  /** 当前轮次（chat:turn-start，1 起）：长任务里「第 N 轮」是还在推进的关键信号。 */
  const streamingTurn = ref(0)
  /** 最近检查点位点（chat:checkpoint）：中断/崩溃后续跑提示据此说明从哪一轮接着做。 */
  const lastCheckpointTurn = ref<number | null>(null)
  /** 本轮命中的 Skill（chat:skill）；流式气泡把它渲染到执行过程时间线首行。 */
  const streamingSkill = ref<SkillHit | null>(null)
  const streamingArtifacts = ref<ArtifactPayload | null>(null)
  const streamingGenUi = ref<UiNode | null>(null)
  /** 建流瞬时错误自动重试提示（chat:retry；正文恢复自动清除）。 */
  const streamingRetry = ref<{ attempt: number; delay_ms: number } | null>(null)
  const error = ref<string | null>(null)
  // ===== P2 扩展状态 =====
  /** 会话计划状态（todo 工具共享；流式增量由 chat:todo 事件推送）。 */
  const todoState = ref<TodoStateRESP | null>(null)
  /** 会话目标（目标模式）：流式增量由 chat:goal 推送，切会话时权威拉取。 */
  const goal = ref<SessionGoal | null>(null)
  /** 会话文件变更列表（流式增量由 chat:file-change 事件推送）。 */
  const fileChanges = ref<FileChange[]>([])
  /** 会话工件列表（流式增量由 chat:artifact 事件推送）。 */
  const artifacts = ref<Artifact[]>([])
  /** 后台任务列表（流式增量由 task:* 事件推送）。 */
  const tasks = ref<BackgroundTask[]>([])
  /** 斜杠命令元数据（启动拉一次，命令面板消费）。 */
  const commands = ref<SlashCommand[]>([])
  /**
   * 未决审批列表（唯一真相源）：流式事件与启动恢复共用。
   * 一次 run 里可能有多个并发审批（同轮多个需确认工具），必须按队列承载。
   */
  const pendingApprovals = ref<PendingApproval[]>([])
  /** 流终止原因（stopped 事件写入；用户主动停止 → 'cancelled'，中性终态不弹错误）。 */
  const stopReason = ref<string | null>(null)
  /** 本轮 run 起始时刻；终止时算出耗时，让横幅能说「你在 12s 后停止」。 */
  const runStartedAt = ref<number | null>(null)
  /** 本轮 run 的终止耗时（仅在收到 stopped 时冻结）。 */
  const stopElapsedMs = ref<number | null>(null)
  /** 上下文自动压缩通知（chat:compressed）：时间线分隔线「N 条历史已折叠」+ 可展开纪要；下次发送或切会话清除。 */
  const compressionNotice = ref<{ removed: number; summary: string } | null>(null)
  /** 本轮流是否由用户主动取消。 */
  const userCancelled = ref(false)
  /** 当前展示的审批（队列首项，只读）：单值 UI 与既有调用签名保持兼容。 */
  const pendingApproval = computed<ApprovalRequest | null>(() => {
    const first = pendingApprovals.value[0]
    return first ? toApprovalRequest(first) : null
  })
  // ===== M2 会话体验增强状态 =====
  /** 当前会话上下文占用快照；切会话或流式 stats 更新后刷新。 */
  const contextUsage = ref<ContextUsageRESP | null>(null)
  /** 当前会话实际生效的运行参数（温度/思考/窗口/压缩阈值）；输入框与设置共用。 */
  const effectiveParams = ref<EffectiveParams | null>(null)
  /** 跨会话检索命中结果。 */
  const searchResults = ref<Session[]>([])
  const searchQuery = ref('')

  /** 当前进行中的流式句柄（用于停止按钮 cancel）。 */
  let activeHandle: StreamHandle | null = null

  async function loadSessions(): Promise<void> {
    loadingSessions.value = true
    try {
      const page = await apiGet<Page<Session>>('/api/v1/chat/sessions?page=1&page_size=100')
      sessions.value = page.items
    } finally {
      loadingSessions.value = false
    }
  }

  /** 拉取序号：并发/重连时只有最新一次拉取可写入，过期响应直接丢弃。 */
  let loadSeq = 0

  async function loadMessages(id: string, opts?: { replace?: boolean }): Promise<void> {
    const seq = ++loadSeq
    // 后端返回分页结构 { items, total, next_seq }（MessageListRESP），
    // 这里先取 items 再与本地乐观消息合并，避免把整包对象当数组喂给 v-for
    const resp = await apiGet<{ items: ApiMessage[]; total: number; next_seq: number }>(`/api/v1/chat/sessions/${id}/messages?limit=200`)
    // 已有更新的拉取发起 → 本次结果过期，丢弃（防止慢响应把新快照覆盖回旧快照）
    if (seq !== loadSeq) return
    const loaded = toUiMessages(Array.isArray(resp?.items) ? resp.items : [])
    // replace：流式收尾以权威快照为准——乐观占位与权威内容的前缀匹配在多轮 ReAct /
    // <think> 场景下不可靠，merge 残留会让同一回复出现两条
    messages.value = opts?.replace ? loaded : mergeLoadedMessages(messages.value, loaded)
  }

  /** 本地乐观消息 id（randomUUID 防同毫秒碰撞；非安全上下文降级时间戳+随机段）。 */
  function genLocalID(): string {
    try {
      if (typeof crypto !== 'undefined' && crypto.randomUUID) return `local-${crypto.randomUUID()}`
    } catch {
      // ignore：非安全上下文可能无 randomUUID
    }
    return `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  }



  /** 加载可切换的模型列表（provider + 模型两级聚合）。 */
  async function loadModels(): Promise<void> {
    // 并行拉模型列表 + 熔断状态；熔断接口偶发失败不阻塞主流程
    let m: AvailableModel[] = []
    let ok = false
    try {
      const r = await apiGet<AvailableModel[]>('/api/v1/ai-provider/available')
      m = Array.isArray(r) ? r : []
      ok = true
    } catch (e) {
      // 模型列表失败：记错误但不阻塞 UI（用户可手动重试）；
      // 保留上一次成功的列表，避免「点新会话后模型下拉忽然空了」
      error.value = e instanceof Error ? e.message : String(e)
    }
    let c: CircuitState[] = []
    try {
      const r = await apiGet<CircuitState[]>('/api/v1/ai-provider/circuit-status')
      c = Array.isArray(r) ? r : []
    } catch {
      c = []
    }
    // 只有成功响应才覆盖 models；请求失败时沿用已有列表（含已选模型的可用性判断）
    if (ok) {
      models.value = m
    } else {
      m = models.value // 回退到旧列表，供下方 selectedModelID 同步使用
    }
    const map = new Map<string, CircuitState>()
    for (const s of c) map.set(s.id, s)
    circuitStates.value = map
    // 模型列表就绪后，把选择器同步到当前会话绑定的 provider（若仍指向已不存在的模型则回落）
    const cur = sessions.value.find((x) => x.id === currentID.value)
    const bound = cur?.provider_id
    if (bound && m.some((mm) => mm.id === bound)) {
      if (selectedModelID.value !== bound) selectedModelID.value = bound
    } else if (selectedModelID.value && !m.some((mm) => mm.id === selectedModelID.value)) {
      selectedModelID.value = bound && m.some((mm) => mm.id === bound) ? bound : null
    }
  }

  /** 清空指定 provider 的熔断。返回 boolean 表示 provider 是否存在。 */
  async function resetCircuit(id: string): Promise<boolean> {
    try {
      await apiPost(`/api/v1/ai-provider/${id}/reset-circuit`)
      await loadModels()
      return true
    } catch {
      return false
    }
  }

  /**
   * 选择本次会话使用的模型（null = Auto，跟随会话绑定/默认）。
   *
   * <p>有会话时切换模型 → 持久化 session.provider_id/model（后端 /chat/sessions/:id/model），
   * 并重新拉取 effective params —— 使输入框的思考强度/采样温度、顶栏模型徽标立即跟随所选模型配置
   * 同步更新会话的 model，避免参数 chip 展示与运行模型不一致。
   * 无会话时仅记录，创建会话时携带。
   */
  async function selectModel(id: string | null): Promise<void> {
    const prev = selectedModelID.value
    selectedModelID.value = id
    const sid = currentID.value
    if (!id || !sid) return
    const m = models.value.find((x) => x.id === id)
    if (!m) return
    try {
      const updated = await apiPost<Session>(`/api/v1/chat/sessions/${sid}/model`, {
        provider_id: m.id,
        model: m.model
      })
      const i = sessions.value.findIndex((s) => s.id === sid)
      if (i >= 0 && updated) sessions.value[i] = updated
      await loadEffectiveParams(sid)
    } catch (e) {
      // 失败回滚到原选择，避免 UI 显示与实际运行模型不一致
      selectedModelID.value = prev
      useToast().error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
    }
  }

  async function selectSession(id: string): Promise<void> {
    currentID.value = id
    compressionNotice.value = null
    await loadMessages(id)
    // 模型选择器跟随会话绑定的 provider：切到历史会话时，顶部徽标与参数展示要与实际运行模型一致
    const s = sessions.value.find((x) => x.id === id)
    if (s?.provider_id && models.value.some((m) => m.id === s.provider_id)) {
      selectedModelID.value = s.provider_id
    }
    // 切会话时同步加载本会话的侧栏数据（todo / 变更 / 工件 / 上下文占用），前端无需额外手动触发
    await Promise.allSettled([
      loadEffectiveParams(id),
      loadTodoState(id),
      loadGoal(id),
      loadFileChanges(id),
      loadArtifacts(id),
      loadContextUsage(id),
      // 工作区级命令随会话变化，切会话必须重拉（懒加载在「同一个绑定工作区」的会话间无副作用）
      loadCommands(id),
      // 页面刷新后恢复本会话的未决审批（此前该方法从未被调用，刷新即丢卡片）
      loadPendingApprovals()
    ])
    // 终态回灌：stop_reason 落在消息上，会话快照里取最后一条 assistant 的值——
    // 刷新/切走再切回时「继续」入口仍然可用；completed 或无值时清空（error 同步清，
    // 错误横幅属于上一条消息的语境，跨会话残留会误导）。
    const lastAssistant = [...messages.value].reverse().find((m) => m.role === 'assistant')
    const sr = lastAssistant?.stop_reason ?? null
    stopReason.value = sr && sr !== 'completed' ? sr : null
    error.value = null
  }

  /** 加载斜杠命令元数据（命令面板的数据源）。
   * sessionID 非空时后端一并返回该会话工作区下的命令文件（<ws>/.workbaby/commands），
   * 因此切会话后要重拉一次：工作区级命令只在绑定该工作区的会话里可用。
   * 前端只在本会话内消费的 ClientOnly 命令走本地分支。 */
  async function loadCommands(sessionID?: string | null): Promise<void> {
    try {
      const q = sessionID ? `?session_id=${encodeURIComponent(sessionID)}` : ''
      const resp = await apiGet<{ items: SlashCommand[] }>(`/api/v1/chat/commands${q}`)
      commands.value = resp.items ?? []
    } catch {
      // 后端暂无：留空数组（前端走本地 fallback）
      commands.value = []
    }
  }

  /** 勾选/取消计划项：以服务端返回的快照为准（避免本地状态与模型侧分叉）。 */
  async function toggleTodo(itemID: string): Promise<void> {
    const sid = currentID.value
    if (!sid || !itemID) return
    try {
      todoState.value = await apiPost<TodoStateRESP>(`/api/v1/chat/sessions/${sid}/todos/${itemID}/toggle`)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 拉取会话计划快照（页面刷新后由 EventLog 之外的权威数据兜底）。 */
  async function loadTodoState(sessionID: string): Promise<void> {
    try {
      const s = await apiGet<TodoStateRESP>(`/api/v1/chat/sessions/${sessionID}/todos`)
      todoState.value = s
    } catch {
      todoState.value = null
    }
  }

  /** 拉取会话目标（切会话/刷新后的权威数据源）。 */
  async function loadGoal(sessionID: string): Promise<void> {
    try {
      const r = await apiGet<{ session_id: string; goal: SessionGoal | null }>(
        `/api/v1/chat/sessions/${sessionID}/goal`
      )
      goal.value = r.goal ?? null
    } catch {
      goal.value = null
    }
  }

  /** 目标操作：set/replace/pause/resume/clear；后端落库并广播 chat:goal。 */
  async function updateGoal(sessionID: string, action: string, text?: string): Promise<SessionGoal | null> {
    const r = await apiPost<{ session_id: string; goal: SessionGoal | null }>(
      `/api/v1/chat/sessions/${sessionID}/goal`,
      { action, text: text ?? '' }
    )
    goal.value = r.goal ?? null
    return goal.value
  }

  /** 拉取会话文件变更列表（侧栏变更面板数据源）。 */
  async function loadFileChanges(sessionID: string, limit = 100): Promise<void> {
    try {
      const resp = await apiGet<{ items: FileChange[] }>(`/api/v1/chat/sessions/${sessionID}/changes?limit=${limit}`)
      fileChanges.value = resp.items ?? []
    } catch {
      fileChanges.value = []
    }
  }

  /** 拉取单条变更详情（diff + before/after）。 */
  async function loadFileChangeDetail(id: string): Promise<import('@/types/api').FileChangeDetail | null> {
    try {
      return await apiGet<import('@/types/api').FileChangeDetail>(`/api/v1/chat/changes/${id}`)
    } catch {
      return null
    }
  }

  /** 回滚单条变更；成功后由后端 chat:file-change 事件推送 rolled_back=true。 */
  async function rollbackFileChange(id: string): Promise<boolean> {
    try {
      await apiPost(`/api/v1/chat/changes/${id}/rollback`)
      // 乐观本地标记：事件回传可能延迟，先把列表里的 rolled_back 标 true
      const it = fileChanges.value.find((c) => c.id === id)
      if (it) it.rolled_back = true
      return true
    } catch {
      return false
    }
  }

  /** 拉取会话工件列表。 */
  async function loadArtifacts(sessionID: string, limit = 100): Promise<void> {
    try {
      const resp = await apiGet<{ items: Artifact[] }>(`/api/v1/chat/sessions/${sessionID}/artifacts?limit=${limit}`)
      artifacts.value = resp.items ?? []
    } catch {
      artifacts.value = []
    }
  }

  /** 删除工件登记（不删磁盘文件）。 */
  async function deleteArtifact(id: string): Promise<boolean> {
    try {
      await apiPost(`/api/v1/chat/artifacts/${id}/delete`)
      artifacts.value = artifacts.value.filter((a) => a.id !== id)
      return true
    } catch {
      return false
    }
  }

  /** 未决审批（队列项）→ 展示用审批请求。 */
  function toApprovalRequest(p: PendingApproval): ApprovalRequest {
    return {
      id: p.id,
      command: p.command,
      reason: p.reason,
      risk: p.risk === 'irreversible' ? 'irreversible' : p.risk === 'input_required' ? 'input_required' : 'needs_approval',
      canRemember: p.can_remember === true
    }
  }

  /** 启动期一次性拉取未决审批（页面刷新后恢复）。 */
  async function loadPendingApprovals(): Promise<void> {
    try {
      const list = await apiGet<ApprovalPending[]>('/api/v1/chat/approvals/pending')
      pendingApprovals.value = Array.isArray(list) ? list : []
    } catch {
      pendingApprovals.value = []
    }
  }

  /** 触发 /compact（确定性压缩历史上下文；）。 */
  async function compactSession(
    sessionID: string,
    req: CompactREQ = {}
  ): Promise<CompactResultRESP | null> {
    try {
      return await apiPost<CompactResultRESP>(`/api/v1/chat/sessions/${sessionID}/compact`, req)
    } catch {
      return null
    }
  }

  /** 跨会话快速检索：scope 为空 = title + content。 */
  async function searchSessions(
    query: string,
    scope: 'all' | 'title' | 'content' = 'all',
    limit = 20
  ): Promise<Session[]> {
    const q = query.trim()
    if (!q) {
      searchResults.value = []
      searchQuery.value = ''
      return []
    }
    const url = `/api/v1/chat/sessions/search?q=${encodeURIComponent(q)}&scope=${scope}&limit=${limit}`
    try {
      const resp = await apiGet<Page<Session>>(url)
      const items = resp?.items ?? []
      searchResults.value = items
      searchQuery.value = q
      return items
    } catch {
      return []
    }
  }

  /** 拉取会话上下文占用快照。 */
  async function loadContextUsage(sessionID: string): Promise<ContextUsageRESP | null> {
    try {
      const resp = await apiGet<ContextUsageRESP>(`/api/v1/chat/sessions/${sessionID}/usage/context`)
      contextUsage.value = resp
      return resp
    } catch {
      return null
    }
  }

  // ===== 流式中随 chat:stats 实时刷新上下文占用 =====
  // 之前只在整轮结束后才 loadContextUsage，右下角与顶栏的 ContextRing 整轮纹丝不动。
  // 现在：stats 事件（每轮 LLM 调用结束）先用 input_tokens 本地补丁总量（实测口径，
  // prompt = system + tools + history 的真实值），再以 3s 节流拉权威快照校准分段明细。
  const CTX_REFRESH_MS = 3000
  let ctxLastRefresh = 0
  let ctxTrailingTimer: ReturnType<typeof setTimeout> | null = null

  /** 用本轮实测 input_tokens 就地补丁上下文占用总量（分段明细等权威快照再校准）。 */
  function patchContextUsageFromStats(input: number): void {
    const cu = contextUsage.value
    if (!cu || input <= 0 || cu.context_window <= 0) return
    cu.used_tokens = input
    cu.free_tokens = Math.max(0, cu.context_window - input)
    cu.used_ratio = Math.min(1000, Math.round((input * 1000) / cu.context_window))
    cu.estimated = false
  }

  watch(streamingStats, (s) => {
    const sid = currentID.value
    if (!sid || !s) return
    patchContextUsageFromStats(s.input_tokens ?? 0)
    const now = Date.now()
    if (now - ctxLastRefresh >= CTX_REFRESH_MS) {
      ctxLastRefresh = now
      void loadContextUsage(sid)
      return
    }
    if (ctxTrailingTimer !== null) return
    ctxTrailingTimer = setTimeout(
      () => {
        ctxTrailingTimer = null
        ctxLastRefresh = Date.now()
        if (currentID.value) void loadContextUsage(currentID.value)
      },
      CTX_REFRESH_MS - (now - ctxLastRefresh)
    )
  })

  /**
   * 新建会话并把用户选择的模型（id）一并写进 session。
   * 前端 store 只知模型名（model 字段），不知道 provider_id；
   * 后端 CreateSession 会按 model 字段精确匹配 ai_providers 表回查 provider，
   * 缺省回退到第一个 enabled Provider——避免会话创建后立即 5003。
   * workspacePath 非空时创建即绑定工作区（一次动作完成，不会因「先建会话再选目录」丢失选择）。
   */
  async function createSession(modelID?: string | null, workspacePath?: string | null): Promise<Session> {
    let modelName: string | undefined
    if (modelID) {
      const m = models.value.find((x) => x.id === modelID)
      if (m) modelName = m.model
    }
    const session = await apiPost<Session>('/api/v1/chat/sessions', {
      name: null,
      model: modelName ?? null,
      workspace_path: workspacePath ?? null
    })
    sessions.value = [session, ...sessions.value]
    currentID.value = session.id
    messages.value = []
    // 新会话创建即拉一次有效参数，让输入框思考强度/温度 chip 立即展示该模型配置
    await loadEffectiveParams(session.id)
    // 刷新模型列表并把 selectedModelID 同步到新会话绑定的 provider：
    // 保证「点新会话后」模型下拉仍有可用模型、顶部徽标与实际运行模型一致。
    await loadModels()
    // 新会话可能绑定了另一个工作区：命令面板要跟着换（含该工作区的 .workbaby/commands）
    await loadCommands(session.id)
    return session
  }

  async function deleteSession(id: string): Promise<void> {
    await apiPost(`/api/v1/chat/sessions/${id}/delete`)
    sessions.value = sessions.value.filter((s) => s.id !== id)
    if (currentID.value === id) {
      const next = sessions.value[0]
      currentID.value = next?.id ?? null
      messages.value = []
      if (next) await loadMessages(next.id)
    }
  }

  /** 置顶/取消置顶：本地即时更新（后端同序返回权威行）。 */
  async function pinSession(id: string, pinned: boolean): Promise<void> {
    const s = await apiPost<Session>(`/api/v1/chat/sessions/${id}/pin`, { pinned })
    sessions.value = sessions.value.map((x) => (x.id === id ? s : x))
  }

  /** 归档/取消归档：归档自动取消置顶；当前会话被归档时切到列表第一个可见会话。 */
  async function archiveSession(id: string, archived: boolean): Promise<void> {
    const s = await apiPost<Session>(`/api/v1/chat/sessions/${id}/archive`, { archived })
    sessions.value = sessions.value.map((x) => (x.id === id ? s : x))
    if (archived && currentID.value === id) {
      const next = sessions.value.find((x) => x.id !== id && x.status !== 'archived')
      if (next) await selectSession(next.id)
    }
  }

  /** 批量删除会话。 */
  async function deleteSessions(ids: string[]): Promise<{ ok: number; failed: string[] }> {
    const failed: string[] = []
    let ok = 0
    for (const id of ids) {
      try {
        await apiPost(`/api/v1/chat/sessions/${id}/delete`)
        sessions.value = sessions.value.filter((s) => s.id !== id)
        if (currentID.value === id) {
          currentID.value = sessions.value[0]?.id ?? null
          messages.value = []
        }
        ok += 1
      } catch {
        failed.push(id)
      }
    }
    if (currentID.value && !sessions.value.some((s) => s.id === currentID.value)) {
      await loadSessions()
      const next = sessions.value[0]
      currentID.value = next?.id ?? null
      if (next) await loadMessages(next.id)
    }
    return { ok, failed }
  }

  /** 清空当前会话所有消息。 */
  async function clearMessages(): Promise<void> {
    if (!currentID.value) return
    await apiPost(`/api/v1/chat/sessions/${currentID.value}/clear`)
    messages.value = []
  }

  /** 从指定消息截断（含该消息及其之后所有消息）。 */
  async function truncateMessages(messageID: string): Promise<void> {
    if (!currentID.value) return
    await apiPost(`/api/v1/chat/messages/${currentID.value}/truncate`, { message_id: messageID })
    await loadMessages(currentID.value)
  }

  /** 重命名会话（inline 双击编辑）。 */
  async function renameSession(id: string, name: string): Promise<void> {
    await apiPost(`/api/v1/chat/sessions/${id}/rename`, { name })
    const s = sessions.value.find((x) => x.id === id)
    if (s) s.name = name
  }

  /** 更新会话绑定的工作区（外部目录绝对路径；null = 解绑回默认）。 */
  async function updateSessionWorkspace(id: string, workspacePath: string | null): Promise<void> {
    const updated = await apiPost<Session>(
      `/api/v1/chat/sessions/${id}/workspace`,
      { workspace_path: workspacePath ?? '' }
    )
    const i = sessions.value.findIndex((s) => s.id === id)
    if (i >= 0) sessions.value[i] = updated
  }

  /** 本地先渲染一条用户气泡（服务端落库后再由 loadMessages 对齐）。 */
  function pushLocalUserMessage(sessionID: string, text: string): void {
    messages.value.push({
      id: genLocalID(),
      session_id: sessionID,
      role: 'user',
      content: text,
      status: 'completed',
      model: null,
      created_at: Date.now(),
      updated_at: Date.now()
    } as unknown as ApiMessage)
  }

  /** 请求级采样参数。 */
  interface SendParams {
    temperature?: number | null
    thinkingEffort?: string | null
  }

  async function sendMessage(text: string, fileIds: string[] = [], params: SendParams = {}): Promise<void> {
    // /new 命令的本地令牌：新建会话（切到空会话），绝不作为消息发给 LLM
    if (text === '__wb_new_session__') {
      const session = await createSession(selectedModelID.value ?? null)
      if (session) useToast().success(t('chat.newSessionCreated'))
      return
    }
    compressionNotice.value = null
    let sessionID = currentID.value
    if (!sessionID) {
      // 新会话时把当前选中的模型 ID 一并传入（避免创建后因缺 provider/model 立即 5003）
      const session = await createSession(selectedModelID.value)
      sessionID = session.id
    }

    // 本轮正在生成：走注入缝（steering / follow-up）——消息并入当前 run，下一轮带上，不打断执行
    if (streaming.value) {
      pushLocalUserMessage(sessionID, text)
      try {
        await apiPost(`/api/v1/chat/sessions/${sessionID}/steer`, { content: text })
        useToast().info(t('chat.steerQueued'))
      } catch (e) {
        useToast().error(t('chat.steerFailed'), e instanceof Error ? e.message : String(e))
      }
      return
    }

    messages.value.push({
      id: genLocalID(),
      session_id: sessionID,
      role: 'user',
      content: text,
      status: 'completed',
      model: null,
      created_at: Date.now(),
      updated_at: Date.now()
    } as unknown as ApiMessage)

    streaming.value = true
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingBlocks.value = []
    streamingStats.value = null
    streamingSkill.value = null
    streamingArtifacts.value = null
    streamingGenUi.value = null
    streamingRetry.value = null
    error.value = null
    stopReason.value = null
    stopElapsedMs.value = null
    runStartedAt.value = Date.now()
    userCancelled.value = false
    pendingApprovals.value = []
    streamingTurn.value = 0
    lastCheckpointTurn.value = null
    // 注：本轮新增的 file_changes / artifacts 由流式事件增量累积；
    // 这里不清空——变更面板承载「本会话全部变更历史」，按 run 分组展示。

    try {
      const handle = streamChat(
        { message: text, session_id: sessionID, model_id: selectedModelID.value ?? undefined, file_ids: fileIds, temperature: params.temperature, thinking_effort: params.thinkingEffort },
        (event: ChatStreamEvent) => {
          handleEvent(event)
        },
        // M2 可靠性：手动重连成功后立即重拉权威快照（弥补重放窗口覆盖不到的缺失）
        { onReconnect: () => void safeLoadMessages(sessionID) }
      )
      activeHandle = handle
      await handle.promise
      await safeLoadMessages(sessionID, { replace: true })
      finalizeStreamingMessage(sessionID)
    } catch (e) {
      if (!userCancelled.value) {
        error.value = e instanceof Error ? e.message : String(e)
      }
      try {
        await safeLoadMessages(sessionID, { replace: true })
      } catch {
        // swallow
      }
      finalizeStreamingMessage(sessionID)
      if (!userCancelled.value) {
        useToast().error(t('chat.streamFailed'), error.value ?? undefined)
      }
    } finally {
      batcher.flush()
      activeHandle = null
      streaming.value = false
      streamingContent.value = ''
      streamingThinking.value = ''
      streamingBlocks.value = []
      streamingStats.value = null
      streamingSkill.value = null
      streamingArtifacts.value = null
      streamingGenUi.value = null
      // 注：本轮新增的 file_changes / artifacts 已由 batcher 流式累积；不重置——保留本会话全部变更历史
      await loadSessions()
      // 刷新上下文占用快照
      if (sessionID) await loadContextUsage(sessionID)
    }
  }

  /** 从检查点续跑：中断/崩溃后的 failed+interrupted 消息可一键续跑。 */
  async function resumeRun(runID: string): Promise<void> {
    const sessionID = currentID.value
    if (!sessionID) throw new Error(t('chat.noSession'))
    streaming.value = true
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingBlocks.value = []
    streamingStats.value = null
    streamingSkill.value = null
    streamingArtifacts.value = null
    streamingGenUi.value = null
    streamingRetry.value = null
    error.value = null
    stopReason.value = null
    stopElapsedMs.value = null
    runStartedAt.value = Date.now()
    userCancelled.value = false
    pendingApprovals.value = []
    streamingTurn.value = 0
    lastCheckpointTurn.value = null
    try {
      const handle = streamChat(
        { message: '', session_id: sessionID, resume_run_id: runID },
        (event: ChatStreamEvent) => handleEvent(event),
        { onReconnect: () => void safeLoadMessages(sessionID) }
      )
      activeHandle = handle
      await handle.promise
      await safeLoadMessages(sessionID, { replace: true })
      finalizeStreamingMessage(sessionID)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      await safeLoadMessages(sessionID, { replace: true })
      finalizeStreamingMessage(sessionID)
      useToast().error(t('chat.streamFailed'), error.value ?? undefined)
    } finally {
      batcher.flush()
      activeHandle = null
      streaming.value = false
      streamingContent.value = ''
      streamingThinking.value = ''
      streamingBlocks.value = []
      streamingStats.value = null
      streamingSkill.value = null
      streamingArtifacts.value = null
      streamingGenUi.value = null
      await loadSessions()
      if (sessionID) await loadContextUsage(sessionID)
    }
  }

  /** 拉取当前会话生效参数（三层合并 + 来源标注）；失败静默（chip 显示兜底文案）。 */
  async function loadEffectiveParams(id: string): Promise<void> {
    try {
      effectiveParams.value = await apiGet<EffectiveParams>(`/api/v1/chat/sessions/${id}/params`)
    } catch {
      effectiveParams.value = null
    }
  }

  /** 切换会话工具权限模式：后端持久化到会话，run 时生效。 */
  async function setPermissionLevel(id: string, level: PermissionLevel): Promise<void> {
    const updated = await apiPost<Session>(`/api/v1/chat/sessions/${id}/permission`, {
      mode: permissionModeToBackend(level)
    })
    const i = sessions.value.findIndex((s) => s.id === id)
    if (i >= 0 && updated) sessions.value[i] = updated
  }

  async function safeLoadMessages(id: string, opts?: { replace?: boolean }): Promise<void> {
    try {
      await loadMessages(id, opts)
    } catch {
      // swallow
    }
  }

  /** 停止当前流式生成。 */
  async function cancelStream(): Promise<void> {
    userCancelled.value = true
    // 停止的那一刻就把还在转圈的工具落成 stopped 终态：
    // 等 run 终止事件再收尾的话，用户会看到转圈继续转很久，分不清「停了没」。
    settleRunningTools()
    const sessionID = currentID.value
    if (sessionID) {
      try {
        await apiPost(`/api/v1/chat/sessions/${sessionID}/cancel`)
      } catch {
        // swallow
      }
    }
    activeHandle?.cancel()
  }

  /** 把仍在 running 的工具块置为 stopped（保留已产出内容，不显示为失败）。 */
  function settleRunningTools(): void {
    if (streamingBlocks.value.length === 0) return
    streamingBlocks.value = streamingBlocks.value.map((b) =>
      b.kind === 'tool_call' && b.state === 'running' ? { ...b, state: 'stopped' as const } : b
    )
  }

  /** 关闭终止原因横幅（用户手动关闭；下轮发送时还会自动复位）。 */
  function dismissStopReason(): void {
    stopReason.value = null
  }

  /** 关闭错误横幅（用户手动关闭；重试走视图层的重新生成入口）。 */
  function dismissError(): void {
    error.value = null
  }

  /**
   * 决策回填：成功才清卡片，失败保留卡片供重试。
   *
   * <p>旧实现在 await 之前就清空 pendingApproval，一次网络/状态异常即让卡片消失，
   * 而后端仍在等待（表现为 run 卡住 + 后续回复出现「请求不存在或已过期」）。
   */
  async function settleApproval(
    a: ApprovalRequest,
    call: () => Promise<unknown>
  ): Promise<void> {
    try {
      await call()
      // 成功才出队；失败保留卡片供重试（此前失败会重新赋值单值 ref，多审批下会挤掉队友）
      pendingApprovals.value = pendingApprovals.value.filter((p) => p.id !== a.id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 允许一次：只放行本次，同命令下次仍会询问。 */
  async function approveApproval(): Promise<void> {
    const a = pendingApproval.value
    if (!a) return
    await settleApproval(a, () =>
      apiPost(`/api/v1/chat/approval/${a.id}/decide`, { approved: true })
    )
  }

  /** 本会话允许：本次放行并记住该命令（仅 needs_approval 可选）。 */
  async function approveApprovalForSession(): Promise<void> {
    const a = pendingApproval.value
    if (!a) return
    await settleApproval(a, () =>
      apiPost(`/api/v1/chat/approval/${a.id}/decide`, { approved: true, scope: 'session' })
    )
  }

  /** 回复补充输入请求（request_input 工具，risk=input_required）。 */
  async function answerApproval(text: string): Promise<void> {
    const a = pendingApproval.value
    if (!a) return
    await settleApproval(a, () =>
      apiPost(`/api/v1/chat/approval/${a.id}/answer`, { answer: text })
    )
  }

  /** 拒绝待审批的危险命令。 */
  async function denyApproval(): Promise<void> {
    const a = pendingApproval.value
    if (!a) return
    await settleApproval(a, () =>
      apiPost(`/api/v1/chat/approval/${a.id}/decide`, { approved: false })
    )
  }

  /**
   * 全部批准：一次确认放行队列中所有「可恢复」审批。
   *
   * 不可逆（irreversible）与补充输入（input_required）不参与批量——前者必须逐条确认
   * （永不免审），后者需要具体内容而非「批准」。
   */
  async function approveAllPending(): Promise<void> {
    const targets = pendingApprovals.value.filter((p) => p.risk === 'needs_approval')
    for (const p of targets) {
      try {
        await apiPost(`/api/v1/chat/approval/${p.id}/decide`, { approved: true })
        pendingApprovals.value = pendingApprovals.value.filter((x) => x.id !== p.id)
      } catch (e) {
        error.value = e instanceof Error ? e.message : String(e)
      }
    }
  }

  /**
   * 跳过：审批按拒绝处理，补充输入按「未回复」处理。
   * 两类请求共用后端 /skip（/decide 只认审批通道）。
   */
  async function skipApproval(): Promise<void> {
    const a = pendingApproval.value
    if (!a) return
    await settleApproval(a, () => apiPost(`/api/v1/chat/approval/${a.id}/skip`))
  }

  const batcher = new StreamEventBatcher((updates: StreamEventUpdate[]) => {
    for (const update of updates) {
      // chat:gap → 重放窗口失效，立即拉权威快照
      if (update.requestSnapshot && currentID.value) {
        void safeLoadMessages(currentID.value)
        continue
      }
      // chat:compressed → 自动压缩发生了但没有别的视觉信号，必须显式提示，
      // 否则用户只会发现「前面的聊天不见了」；toast 之外再落一条时间线分隔线（带交接纪要）
      if (update.setCompressed) {
        useToast().info(t('chat.autoCompressed', update.setCompressed.removed_messages))
        compressionNotice.value = {
          removed: update.setCompressed.removed_messages,
          summary: update.setCompressed.summary ?? ''
        }
        continue
      }
      // chat:context-trimmed → system 段被预算裁掉（如 Skill 正文）：回答质量下降必须可解释，
      // 否则用户只会觉得「模型变笨了」。
      if (update.setContextTrimmed) {
        useToast().warning(t('chat.contextTrimmed', update.setContextTrimmed.dropped_segments.join('、')))
        continue
      }
      // chat:warn → 工作区越界写入：AI 把文件落到了 .workbaby/ 之外，
      // 必须以红色 toast + 文件路径强制提示用户清理；这是污染既有目录的硬伤，
      // 不能仅靠 message block 静默展示。
      if (update.setWarn) {
        // 反幻觉核验：模型声称产出文件但本轮零工具调用——比普通错误更伤信任，红色强提示
        if (update.setWarn.kind === 'unbacked_claim') {
          useToast().error(t('chat.unbackedClaimTitle'), update.setWarn.message)
          continue
        }
        const title = update.setWarn.rel_path
          ? t('chat.sandboxViolation', update.setWarn.rel_path)
          : t('chat.sandboxViolationNoPath')
        useToast().error(title, update.setWarn.message)
        continue
      }
      // chat:goal → 目标状态卡（set/pause/auto-continue 校验后共用；null = 已清除）
      if (update.setGoal !== undefined) {
        const prevStatus = goal.value?.status
        goal.value = update.setGoal
        // 达成即时反馈：卡片随 done 消失，达成依据用 toast 收尾
        if (update.setGoal?.status === 'done' && prevStatus !== 'done') {
          useToast().success(
            t('chat.goal.doneTitle'),
            update.setGoal.done_because || update.setGoal.text
          )
        }
        continue
      }
      applyStreamUpdate(update, {
        streamingContent,
        streamingThinking,
        streamingBlocks,
        streamingStats,
        streamingTurn,
        lastCheckpointTurn,
        streamingSkill,
        stopReason,
        streamingArtifacts,
        streamingGenUi,
        pendingApprovals,
        error,
        streamingRetry,
        todoState,
        fileChanges,
        artifacts,
        tasks
      })
      // 终态即收尾：立即关闭流式气泡，消除「流式气泡 + 权威消息」重影窗口。
      // 权威快照由 sendMessage / resumeRun 在 handle.promise 结束后统一拉一次——
      // 这里再发一次请求会与前者并发且返回顺序不定，过期响应由 loadSeq 守卫丢弃。
      if (update.setStopReason !== undefined) {
        streaming.value = false
        // run 已终止：还挂着 running 的工具不会再有结果回来，落成明确终态
        settleRunningTools()
        // 冻结本轮耗时：终止原因横幅据此把系统状态翻译成用户做过的事
        //（「你在 12s 后停止」比裸「已停止」有信息量得多）
        stopElapsedMs.value = runStartedAt.value != null ? Date.now() - runStartedAt.value : null
      }
    }
  })

  function handleEvent(event: ChatStreamEvent): void {
    batcher.push(event)
  }

  /** 错误文案压成单行短句：正文区只承担「发生了什么」，长诊断交给横幅与日志。 */
  function shortErrorText(): string {
    const e = (error.value ?? '').trim()
    if (!e) return ''
    const oneLine = e.replace(/\s+/g, ' ')
    return oneLine.length > 160 ? `${oneLine.slice(0, 160)}…` : oneLine
  }

  function finalizeStreamingMessage(sessionID: string): void {
    const content = streamingContent.value.trim()
    const errText = shortErrorText()
    const hasPayload =
      content.length > 0 ||
      streamingBlocks.value.length > 0 ||
      streamingArtifacts.value !== null ||
      streamingGenUi.value !== null
    if (!hasPayload && !error.value) return

    // 权威快照已替换 messages，本 run 回复的落位由 resolveRunAssistant 判定：
    // dedupe=权威已含（绝不本地再 push） / fill=空占位补正文 / missing=本地兜底
    const verdict = resolveRunAssistant(messages.value, content)
    if (verdict === 'dedupe') return
    if (verdict !== 'missing') {
      const m = verdict.message
      m.content = content || errText || t('chat.streamFailedPlaceholder')
      m.status = 'completed'
      m.updated_at = Date.now()
      // 兜底：权威快照若尚未带上过程块，用本地流式累积还原（chat:done 先于落库完成的窗口期）。
      // 快照完整时不覆盖——权威数据优先，本地只是这段时间的补位。
      if ((!m.blocks || m.blocks.length === 0) && streamingBlocks.value.length > 0) {
        m.blocks = streamingBlocksToMessageBlocks(streamingBlocks.value, m.id)
      }
      return
    }
    // missing：仅当确有内容/错误可展示时才本地兜底，避免塞无意义的占位气泡
    if (!content && !errText) return
    const msg = {
      id: genLocalID(),
      session_id: sessionID,
      role: 'assistant',
      content: content || errText || t('chat.streamFailedPlaceholder'),
      status: 'completed',
      model: null,
      created_at: Date.now(),
      updated_at: Date.now()
    } as unknown as ApiMessage
    const s = streamingStats.value
    if (s) {
      msg.input_tokens = s.input_tokens ?? null
      msg.output_tokens = s.output_tokens ?? null
      msg.cache_read_tokens = s.cache_read_tokens ?? null
      msg.total_tokens = s.total_tokens ?? null
      msg.latency_ms = s.latency_ms ?? null
      msg.cost = s.cost ?? null
    }
    messages.value.push(msg)
    if (!content && !error.value) {
      // eslint-disable-next-line no-console
      console.warn('[chat] finalizeStreamingMessage inserted empty placeholder; session=', sessionID)
    }
  }

  /** 删除单条消息。 */
  async function deleteMessage(messageID: string): Promise<void> {
    if (!currentID.value) return
    await apiPost(`/api/v1/chat/messages/${currentID.value}/delete/${messageID}`)
    messages.value = messages.value.filter((m) => m.id !== messageID)
  }

  /** 从指定消息截断后重新发送。 */
  async function resendFrom(messageID: string, content: string): Promise<void> {
    await truncateMessages(messageID)
    await sendMessage(content)
  }

  /** 会话分叉。 */
  async function forkFrom(messageID: string): Promise<string> {
    if (!currentID.value) throw new Error(t('chat.noSession'))
    const fork = await apiPost<Session>(`/api/v1/chat/messages/${currentID.value}/fork`, {
      message_id: messageID
    })
    sessions.value = [fork, ...sessions.value]
    await selectSession(fork.id)
    return fork.id
  }

  return {
    sessions,
    currentID,
    messages,
    models,
    selectedModelID,
    circuitStates,
    loadingSessions,
    streaming,
    streamingContent,
    streamingThinking,
    streamingBlocks,
    streamingStats,
    streamingTurn,
    lastCheckpointTurn,
    streamingSkill,
    streamingArtifacts,
    streamingGenUi,
    streamingRetry,
    compressionNotice,
    error,
    stopReason,
    stopElapsedMs,
    pendingApproval,
    // ===== P2 扩展 =====
    todoState,
    goal,
    fileChanges,
    artifacts,
    tasks,
    commands,
    pendingApprovals,
    approveAllPending,
    effectiveParams,
    loadSessions,
    loadMessages,
    loadModels,
    loadCommands,
    loadTodoState,
    loadGoal,
    updateGoal,
    toggleTodo,
    loadFileChanges,
    loadFileChangeDetail,
    rollbackFileChange,
    loadArtifacts,
    deleteArtifact,
    loadPendingApprovals,
    compactSession,
    searchSessions,
    loadContextUsage,
    contextUsage,
    searchResults,
    searchQuery,
    selectModel,
    resetCircuit,
    selectSession,
    createSession,
    deleteSession,
    pinSession,
    archiveSession,
    deleteSessions,
    clearMessages,
    truncateMessages,
    renameSession,
    sendMessage,
    resumeRun,
    cancelStream,
    dismissStopReason,
    dismissError,
    deleteMessage,
    resendFrom,
    forkFrom,
    approveApproval,
    approveApprovalForSession,
    answerApproval,
    denyApproval,
    skipApproval,
    updateSessionWorkspace,
    loadEffectiveParams,
    setPermissionLevel
  }
})
