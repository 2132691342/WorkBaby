import type {
  Artifact,
  ArtifactPayload,
  BackgroundTask,
  ChatStats,
  ChatStreamEvent,
  FileChange,
  PendingApproval,
  SessionGoal,
  SkillHit,
  TodoStateRESP
} from '@/types/api'
import type { UiNode } from '@/components/genui/GenUiRenderer.vue'
import { applyBlockUpdate } from '@/chat/models/streamingBlocks'

/**
 * 流式聊天事件解码器（纯函数）：输入 SSE 领域事件 → 输出 {@link StreamEventUpdate} 描述对象，store 负责 apply。
 * 事件映射：{@code content/thinking → appendContent/appendThinking}、{@code tool_call → addTool}、{@code tool_result → updateTool}、{@code artifact → setArtifacts}、
 * {@code tool_approval_request → setApproval}、{@code error → setError}、{@code stopped → setStopReason}（中性终态）。
 */
export interface ToolCallInfo {
  id: string
  name: string
  args?: string
  result?: string
  /** stopped = 用户主动停止 / run 终止时仍未返回的工具，区别于真实失败。 */
  state?: 'running' | 'success' | 'error' | 'stopped'
  started_at?: number
  duration_ms?: number
  /** 子 Agent 来源标签（delegate 转发的子 run 工具事件）；空 = 主 Agent。 */
  agent?: string
  /** 工具自述的一行动作描述（"正在编辑 app.ts"）；空则退回工具名展示。 */
  activity?: string
}

export interface ApprovalRequest {
  id: string
  command: string
  reason: string
  /**
   * 风险等级：
   * - `needs_approval`：危险但可恢复，可选「本会话允许」后免审
   * - `irreversible`：执行后无法撤销（rm -rf / format / dd if= …），每次都要确认
   * - `input_required`：模型补充输入请求（request_input 工具），卡片切问答形态
   */
  risk?: 'needs_approval' | 'irreversible' | 'input_required'
  /** 后端声明可否「本会话允许」（不可逆操作恒 false）。 */
  canRemember?: boolean
}

/** decoder 输出：store 拿到后按需 apply 到 ref state。 */
export interface StreamEventUpdate {
  appendContent?: string
  appendThinking?: string
  /** 流式块增量（按事件到达顺序）：工具与正文的唯一真相源。 */
  blockAppend?: { kind: 'thinking' | 'text'; text: string }
  blockToolCall?: { id: string; name: string; arguments?: string; agent?: string; activity?: string }
  blockToolResult?: { id: string; name: string; content?: string; error?: string; duration_ms?: number; refused?: boolean; data?: Record<string, unknown> | null }
  blockSkill?: { name: string; source?: string; description?: string; tools?: string[]; injected_chars?: number }
  blockArtifact?: { name: string; data: Record<string, unknown> }
  blockGenUi?: UiNode
  setStats?: ChatStats
  /** 当前轮次（chat:turn-start，1 起）：长任务中让用户看到「第 N 轮」在推进。 */
  setTurn?: number
  /** 最近一次检查点位点轮次（chat:checkpoint）：续跑提示据此说明从哪一轮接着做。 */
  setCheckpoint?: number
  /** 本轮命中的 Skill（chat:skill）；一轮至多一次，覆盖式写入。 */
  setSkillHit?: SkillHit
  setArtifacts?: ArtifactPayload
  setGenUi?: UiNode
  setApproval?: ApprovalRequest
  /**
   * 清 pendingApproval（chat:approval-decided → approval_decided）。
   * 仅 id 匹配的才清，避免误清并发新审批请求。
   */
  clearApproval?: { id: string; decision: string }
  setError?: string
  /** 建流瞬时错误自动重试提示（限流/5xx）；正文恢复后自动清除。 */
  setRetry?: { attempt: number; delay_ms: number }
  /** 流终止原因（cancelled 等）。中性终态，区别于 error。 */
  setStopReason?: string
  // ===== P2 扩展事件 =====
  /** todo 工具每次动作后推送的快照（覆盖式）。 */
  setTodo?: TodoStateRESP
  /** file_write 写入后推送的单条变更（最新在前；按 run 范围前端可选择性过滤）。 */
  pushFileChange?: FileChange
  /** 工件登记事件（file_write 旁路自动 upsert）。 */
  pushArtifact?: Artifact
  /** 自动上下文压缩（chat:compressed）：告知用户历史已被折叠，不是内容丢了。
   *  summary 是压缩器产出的交接纪要（auto 六段摘要；micro 为空），分隔线展开可见。 */
  setCompressed?: { removed_messages: number; filter_key?: string; recovery_refs?: string[]; summary?: string }
  /** 上下文按预算裁剪（chat:context-trimmed）：system 段被丢，回答质量下降需可解释。 */
  setContextTrimmed?: { dropped_segments: string[]; budget_runes: number }
  /** 越界告警（chat:warn）：副作用落到了 .workbaby/ 之外，强制 toast 提示用户清理。 */
  setWarn?: { kind: string; message: string; rel_path: string; path: string }
  /** 会话目标状态推送（chat:goal）；null = 目标已清除。 */
  setGoal?: SessionGoal | null
  /** 建 BackgroundTask 条目（subagent_start：子 Agent 委派开跑即登记）。 */
  upsertTask?: BackgroundTask
  /** 子 Agent 生命周期（subagent_start/done/error）：按 id 合并状态，不整体替换。 */
  updateTask?: { id: string; agent?: string; state?: BackgroundTask['state']; error?: string }
  /**
   * 断线重放窗口已失效（chat:gap）→ 请求 store 立即拉取权威快照。
   * 由 store 的 batcher 消费（异步动作），applyStreamUpdate 忽略。
   */
  requestSnapshot?: boolean
}

/** 解码单个事件为 store update；type 不识别 / data 字段缺失返回 null。 */
export function decodeStreamEvent(event: ChatStreamEvent, now: number = Date.now()): StreamEventUpdate | null {
  const data = event.data
  switch (event.type) {
    case 'content':
      return typeof data === 'string' ? { appendContent: data, blockAppend: { kind: 'text', text: data } } : null
    case 'thinking':
      return typeof data === 'string' ? { appendThinking: data, blockAppend: { kind: 'thinking', text: data } } : null
    case 'stats':
      return data && typeof data === 'object' ? { setStats: data as ChatStats } : null
    case 'turn_start': {
      if (!data || typeof data !== 'object') return null
      const d = data as { turn?: number }
      const turn = Number(d.turn ?? 0)
      return turn > 0 ? { setTurn: turn } : null
    }
    case 'checkpoint': {
      if (!data || typeof data !== 'object') return null
      const d = data as { turn?: number }
      const turn = Number(d.turn ?? 0)
      return turn > 0 ? { setCheckpoint: turn } : null
    }
    case 'tool_call': {
      if (!data || typeof data !== 'object') return null
      const d = data as { id: string; name: string; arguments?: string; agent?: string; activity?: string }
      return {
        blockToolCall: { id: d.id, name: d.name, arguments: d.arguments, agent: d.agent, activity: d.activity }
      }
    }
    case 'tool_result': {
      if (!data || typeof data !== 'object') return null
      const d = data as { id: string; name: string; output: string; state: string; agent?: string; duration_ms?: number; refused?: boolean; data?: Record<string, unknown> }
      const update: StreamEventUpdate = {
        blockToolResult: {
          id: d.id,
          name: d.name,
          content: d.output,
          error: d.state !== 'success' && !d.refused ? d.output : undefined,
          duration_ms: d.duration_ms,
          refused: d.refused === true,
          data: d.data ?? null
        }
      }
      // 修复：gen_ui 工具结果同步解析 UiTree（避免 store 二次解析）
      if (d.name === 'gen_ui' && d.output) {
        try {
          const tree = JSON.parse(d.output) as { root: UiNode }
          update.setGenUi = tree.root
          update.blockGenUi = tree.root
        } catch {
          // 解析失败时忽略 setGenUi，保留 updateTool 让用户在 tool_result 中查看原文
        }
      }
      // 通用 artifact 块：FileInfo 类工具结果附带给前端 artifact 渲染
      if (d.name && d.output) {
        try {
          const parsed = JSON.parse(d.output) as { artifact?: boolean; data?: Record<string, unknown>; name?: string }
          if (parsed?.artifact) {
            update.blockArtifact = { name: parsed.name ?? d.name, data: parsed.data ?? {} }
          }
        } catch {
          /* 非 JSON 输出忽略 */
        }
      }
      return update
    }
    case 'skill': {
      if (!data || typeof data !== 'object') return null
      const d = data as Partial<SkillHit>
      if (!d.name) return null
      return {
        setSkillHit: { ...(d as SkillHit), tools: d.tools ?? [] },
        blockSkill: {
          name: d.name,
          source: d.source,
          description: d.description,
          tools: d.tools ?? [],
          injected_chars: d.injected_chars
        }
      }
    }
    case 'todo': {
          if (!data || typeof data !== 'object') return null
          const d = data as { state?: TodoStateRESP }
          return d.state ? { setTodo: d.state } : null
        }
        case 'goal': {
          if (!data || typeof data !== 'object') return null
          const d = data as { goal?: SessionGoal | null }
          return { setGoal: d.goal ?? null }
        }
        case 'file_change': {
          if (!data || typeof data !== 'object') return null
          const d = data as { change?: FileChange }
          return d.change ? { pushFileChange: d.change } : null
        }
        case 'session_artifact': {
          if (!data || typeof data !== 'object') return null
          const d = data as { artifact?: Artifact }
          return d.artifact ? { pushArtifact: d.artifact } : null
        }
        case 'compressed': {
          if (!data || typeof data !== 'object') return null
          const d = data as { removed_messages?: number; filter_key?: string; recovery_refs?: string[]; summary?: string }
          return {
            setCompressed: {
              removed_messages: d.removed_messages ?? 0,
              filter_key: d.filter_key ?? '',
              recovery_refs: Array.isArray(d.recovery_refs) ? d.recovery_refs : [],
              summary: typeof d.summary === 'string' ? d.summary : ''
            }
          }
        }
        case 'context_trimmed': {
          if (!data || typeof data !== 'object') return null
          const d = data as { dropped_segments?: string[]; budget_runes?: number }
          const dropped = Array.isArray(d.dropped_segments) ? d.dropped_segments : []
          if (dropped.length === 0) return null
          return { setContextTrimmed: { dropped_segments: dropped, budget_runes: Number(d.budget_runes ?? 0) } }
        }
        case 'warn': {
          if (!data || typeof data !== 'object') return null
          const d = data as { kind?: string; message?: string; rel_path?: string; path?: string }
          return {
            setWarn: {
              kind: d.kind ?? '',
              message: d.message ?? '工作区越界写入',
              rel_path: d.rel_path ?? '',
              path: d.path ?? ''
            }
          }
        }
        case 'subagent_start': {
          if (!data || typeof data !== 'object') return null
          const d = data as { sub_run_id?: string; agent?: string }
          if (!d.sub_run_id) return null
          return {
            upsertTask: {
              id: d.sub_run_id, session_id: '', agent: d.agent ?? 'subagent', prompt: '',
              state: 'running', run_id: d.sub_run_id, result: '', error: '',
              created_at: now, started_at: now, finished_at: 0
            }
          }
        }
        case 'subagent_done': {
          if (!data || typeof data !== 'object') return null
          const d = data as { sub_run_id?: string; agent?: string }
          if (!d.sub_run_id) return null
          return { updateTask: { id: d.sub_run_id, agent: d.agent, state: 'completed' } }
        }
        case 'subagent_error': {
          if (!data || typeof data !== 'object') return null
          const d = data as { sub_run_id?: string; agent?: string; message?: string }
          if (!d.sub_run_id) return null
          return { updateTask: { id: d.sub_run_id, agent: d.agent, state: 'failed', error: d.message ?? '' } }
        }
        case 'tool_approval_request': {
      if (!data || typeof data !== 'object') return null
      const d = data as { id: string; command: string; reason: string; risk?: string; can_remember?: boolean }
      return {
        setApproval: {
          id: d.id,
          command: d.command,
          reason: d.reason,
          risk: d.risk === 'irreversible' ? 'irreversible' : d.risk === 'input_required' ? 'input_required' : 'needs_approval',
          canRemember: d.can_remember === true
        }
      }
    }
    case 'approval_decided': {
      // 反向事件：清 pendingApproval。id 不匹配则忽略（避免误清并发新请求）
      if (!data || typeof data !== 'object') return null
      const d = data as { id: string; decision: string }
      return { clearApproval: { id: d.id, decision: d.decision } }
    }
    case 'error':
      return { setError: extractError(data) }
    case 'retry': {
      if (!data || typeof data !== 'object') return null
      const d = data as { attempt?: number; delay_ms?: number }
      return { setRetry: { attempt: d.attempt ?? 1, delay_ms: d.delay_ms ?? 0 } }
    }
    case 'gap':
      // 断线重连后重放窗口已被覆盖（RunEventLog 环形缓冲滚掉）：
      // 增量补不齐 → 立即拉权威快照做最终一致
      return { requestSnapshot: true }
    case 'stopped': {
      // 用户主动停止 → 中性终态。后端 ChatStreamService.onCancelled 发出。
      const reason = data && typeof data === 'object' && 'reason' in data
        ? String((data as { reason: unknown }).reason)
        : 'cancelled'
      return { setStopReason: reason }
    }
    default:
      return null
  }
}

/** 从事件 data 安全取错误消息字符串（与原 store 实现一致）。 */
function extractError(data: unknown): string {
  if (data && typeof data === 'object' && 'message' in data) {
    return String((data as { message: unknown }).message)
  }
  return String(data)
}

/**
 * 应用单个 update 到 store 状态（store 内部 use）。
 * 这里做成 pure 让 chat.ts 简化；副作用（修改 ref）封装在此函数里。
 */
export function applyStreamUpdate(
  update: StreamEventUpdate,
  state: {
    streamingContent: { value: string }
    streamingThinking: { value: string }
    /** 流式累积块序列（按事件到达顺序）：工具与正文的唯一真相源。 */
    streamingBlocks?: { value: import('@/chat/models/streamingBlocks').StreamingBlock[] }
    streamingStats: { value: ChatStats | null }
    /** 当前轮次（1 起）；调用方按需传入。 */
    streamingTurn?: { value: number }
    /** 最近检查点位点（轮次）；调用方按需传入。 */
    lastCheckpointTurn?: { value: number | null }
    /** 本轮命中的 Skill（可选，调用方按需传入）。 */
    streamingSkill?: { value: SkillHit | null }
    streamingArtifacts: { value: ArtifactPayload | null }
    streamingGenUi: { value: UiNode | null }
    /** 未决审批队列（唯一真相源；多审批并发时按到达顺序排队）。 */
    pendingApprovals: { value: PendingApproval[] }
    error: { value: string | null }
    /** 建流自动重试提示（可选）。 */
    streamingRetry?: { value: { attempt: number; delay_ms: number } | null }
    /** 流终止原因（用户主动停止时由 stopped 事件写入）。 */
    stopReason: { value: string | null }
    // ===== P2 扩展字段（可选，调用方按需传入） =====
    todoState?: { value: TodoStateRESP | null }
    fileChanges?: { value: FileChange[] }
    artifacts?: { value: Artifact[] }
    tasks?: { value: BackgroundTask[] }
  },
  now: number = Date.now()
): void {
  if (update.appendContent !== undefined) {
    state.streamingContent.value += update.appendContent
    // 正文恢复 → 重试提示完成使命
    if (state.streamingRetry && state.streamingRetry.value) state.streamingRetry.value = null
  }
  if (update.appendThinking !== undefined) {
    state.streamingThinking.value += update.appendThinking
  }
  // ===== streamingBlocks 增量应用（单一真相源；MessageBlocksRenderer 直接消费） =====
  if (state.streamingBlocks) {
    let blocks = state.streamingBlocks.value
    if (update.blockAppend) {
      blocks = applyBlockUpdate(blocks, { kind: update.blockAppend.kind, text: update.blockAppend.text })
    }
    if (update.blockToolCall) {
      const tc = update.blockToolCall
      blocks = applyBlockUpdate(blocks, {
        kind: 'tool_call',
        id: tc.id,
        name: tc.name,
        arguments: tc.arguments,
        agent: tc.agent,
        activity: tc.activity,
        now
      })
    }
    if (update.blockToolResult) {
      const tr = update.blockToolResult
      blocks = applyBlockUpdate(blocks, {
        kind: 'tool_result',
        toolCallId: tr.id,
        name: tr.name,
        content: tr.content,
        error: tr.error,
        durationMs: tr.duration_ms,
        refused: tr.refused,
        data: tr.data ?? null
      })
    }
    if (update.blockSkill) {
      blocks = applyBlockUpdate(blocks, {
        kind: 'skill',
        id: `skill-${state.streamingBlocks.value.length}`,
        payload: update.blockSkill as Record<string, unknown>
      })
    }
    if (update.blockArtifact) {
      blocks = applyBlockUpdate(blocks, {
        kind: 'artifact',
        id: `artifact-${state.streamingBlocks.value.length}`,
        payload: update.blockArtifact as Record<string, unknown>
      })
    }
    if (update.blockGenUi) {
      blocks = applyBlockUpdate(blocks, {
        kind: 'genui',
        id: `genui-${state.streamingBlocks.value.length}`,
        payload: { root: update.blockGenUi }
      })
    }
    if (blocks !== state.streamingBlocks.value) state.streamingBlocks.value = blocks
  }
  if (update.setStats) {
    state.streamingStats.value = update.setStats
  }
  if (update.setTurn !== undefined && state.streamingTurn) {
    state.streamingTurn.value = update.setTurn
  }
  if (update.setCheckpoint !== undefined && state.lastCheckpointTurn) {
    state.lastCheckpointTurn.value = update.setCheckpoint
  }
  if (update.setSkillHit && state.streamingSkill) {
    state.streamingSkill.value = update.setSkillHit
  }
  if (update.setArtifacts) {
    state.streamingArtifacts.value = update.setArtifacts
  }
  if (update.setGenUi !== undefined) {
    state.streamingGenUi.value = update.setGenUi
  }
  if (update.setApproval) {
    // 入队而非覆盖：同轮多个工具需确认时，覆盖会让先到的请求永久看不到、也无法决策
    const a = update.setApproval
    if (!state.pendingApprovals.value.some((p) => p.id === a.id)) {
      state.pendingApprovals.value.push({
        id: a.id,
        command: a.command,
        reason: a.reason,
        risk: a.risk ?? 'needs_approval',
        can_remember: a.canRemember
      })
    }
  }
  if (update.clearApproval) {
    // 仅按 id 移除：避免在「A 决策事件」到达时误清「B 新审批请求」
    const clearID = update.clearApproval.id
    state.pendingApprovals.value = state.pendingApprovals.value.filter((p) => p.id !== clearID)
  }
  if (update.setError !== undefined) {
    state.error.value = update.setError
  }
  if (update.setRetry !== undefined && state.streamingRetry) {
    state.streamingRetry.value = update.setRetry
  }
  if (update.setStopReason !== undefined) {
    state.stopReason.value = update.setStopReason
  }
  if (update.setTodo && state.todoState) {
    state.todoState.value = update.setTodo
  }
  if (update.pushFileChange && state.fileChanges) {
    const list = state.fileChanges.value
    const i = list.findIndex((c) => c.id === update.pushFileChange!.id)
    if (i >= 0) list[i] = update.pushFileChange!
    else list.unshift(update.pushFileChange!)
  }
  if (update.pushArtifact && state.artifacts) {
    const list = state.artifacts.value
    const i = list.findIndex((a) => a.id === update.pushArtifact!.id)
    if (i >= 0) list[i] = update.pushArtifact!
    else list.unshift(update.pushArtifact!)
  }
  if (update.upsertTask && state.tasks) {
    const list = state.tasks.value
    const i = list.findIndex((t) => t.id === update.upsertTask!.id)
    if (i >= 0) list[i] = update.upsertTask!
    else list.unshift(update.upsertTask!)
  }
  if (update.updateTask && state.tasks) {
    const t = state.tasks.value.find((x) => x.id === update.updateTask!.id)
    if (t) {
      if (update.updateTask.agent) t.agent = update.updateTask.agent
      if (update.updateTask.state) t.state = update.updateTask.state
      if (update.updateTask.error !== undefined) t.error = update.updateTask.error
      if (update.updateTask.state === 'completed' || update.updateTask.state === 'failed') {
        t.finished_at = now
      }
    }
  }
}