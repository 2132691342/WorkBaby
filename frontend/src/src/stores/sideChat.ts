import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { streamChat, type StreamHandle } from '@/api/stream'
import { resolveRunAssistant, toUiMessages } from '@/chat/models/merge'
import { streamingBlocksToMessageBlocks, type StreamingBlock } from '@/chat/models/streamingBlocks'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type {
  Artifact,
  ArtifactPayload,
  BackgroundTask,
  ChatStats,
  ChatStreamEvent,
  FileChange,
  Message as ApiMessage,
  PendingApproval,
  Session,
  SkillHit,
  TodoStateRESP
} from '@/types/api'
import type { UiNode } from '@/components/genui/GenUiRenderer.vue'
import { applyStreamUpdate } from '@/stores/chat/ChatStreamDecoder'
import { StreamEventBatcher } from '@/stores/chat/StreamEventBatcher'

/**
 * 辅助对话 store：右栏与主任务并行的独立小会话。
 *
 * <p>辅助会话是一个功能完整的会话（可调工具、走权限确认），后端以 kind='side' + parent_id
 * 标识且每轮 run 自动前置主会话的有界历史（service/side_conversation.go），追问不用重复交代背景。
 * 本 store 是主 chat store 的精简镜像：不承接目标模式 / steering / 检查点续跑，只保留
 * 消息 + 流式渲染 + 审批闭环。主会话切换时由面板调用 open() 重置。
 */

type ApiMessageT = ApiMessage

export const useSideChatStore = defineStore('sideChat', () => {
  const toast = useToast()

  /** 辅助面板所属的主会话（null = 尚未绑定任何主会话）。 */
  const mainSessionID = ref<string | null>(null)
  /** 已开启的辅助会话（null = 该主会话还没有辅助会话 → 面板显示启动页）。 */
  const sideSession = ref<Session | null>(null)
  const loading = ref(false)
  const messages = ref<ApiMessageT[]>([])
  /** 输入框草稿（划选引用 / 外部入口可预填）。 */
  const draft = ref('')

  // ===== 流式状态（与主链路同形，渲染组件共用） =====
  const streaming = ref(false)
  const streamingContent = ref('')
  const streamingThinking = ref('')
  const streamingBlocks = ref<StreamingBlock[]>([])
  const streamingStats = ref<ChatStats | null>(null)
  const streamingTurn = ref(0)
  const streamingSkill = ref<SkillHit | null>(null)
  const streamingArtifacts = ref<ArtifactPayload | null>(null)
  const streamingGenUi = ref<UiNode | null>(null)
  const streamingRetry = ref<{ attempt: number; delay_ms: number } | null>(null)
  const pendingApprovals = ref<PendingApproval[]>([])
  const error = ref<string | null>(null)
  const stopReason = ref<string | null>(null)

  let loadSeq = 0
  let activeHandle: StreamHandle | null = null

  const batcher = new StreamEventBatcher((updates) => {
    for (const update of updates) {
      if (update.requestSnapshot && sideSession.value) {
        void loadMessages(sideSession.value.id, { replace: true })
        continue
      }
      // 压缩/裁剪/越界写等提示：面板内用轻量 toast，不重复主链路的分隔线逻辑
      if (update.setCompressed || update.setContextTrimmed || update.setWarn) continue
      // 目标模式与辅助对话互斥（后端也不继承）：忽略目标事件
      if (update.setGoal !== undefined) continue
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
      if (update.setStopReason !== undefined) {
        streaming.value = false
      }
    }
  })
  // 占位 ref：applyStreamUpdate 的入参契约要求齐全；面板不展示这些维度，仅承接事件
  const todoState = ref<TodoStateRESP | null>(null)
  const fileChanges = ref<FileChange[]>([])
  const artifacts = ref<Artifact[]>([])
  const tasks = ref<BackgroundTask[]>([])
  const lastCheckpointTurn = ref<number | null>(null)

  function handleEvent(event: ChatStreamEvent): void {
    batcher.push(event)
  }

  /** 打开主会话的辅助面板：有辅助会话则加载其消息，否则停在启动页（不创建）。 */
  async function open(mainID: string | null): Promise<void> {
    if (mainID === mainSessionID.value && (sideSession.value || messages.value.length === 0)) {
      return
    }
    reset()
    mainSessionID.value = mainID
    if (!mainID) return
    try {
      const side = await apiGet<Session | null>(`/api/v1/chat/sessions/${mainID}/side`)
      if (side) {
        sideSession.value = side
        await loadMessages(side.id)
      }
    } catch {
      // 拉取失败视作无辅助会话：面板回落启动页
    }
  }

  /** 确保辅助会话存在（不存在即创建）并加载消息；返回会话 ID。 */
  async function ensure(mainID: string): Promise<string> {
    if (sideSession.value && mainSessionID.value === mainID) return sideSession.value.id
    reset()
    mainSessionID.value = mainID
    const side = await apiPost<Session>(`/api/v1/chat/sessions/${mainID}/side`)
    sideSession.value = side
    await loadMessages(side.id)
    return side.id
  }

  async function loadMessages(id: string, opts?: { replace?: boolean }): Promise<void> {
    const seq = ++loadSeq
    loading.value = true
    try {
      const resp = await apiGet<{ items: ApiMessageT[]; total: number; next_seq: number }>(
        `/api/v1/chat/sessions/${id}/messages?limit=100`
      )
      if (seq !== loadSeq) return
      const loaded = toUiMessages(Array.isArray(resp?.items) ? resp.items : [])
      messages.value = opts?.replace ? loaded : loaded
    } finally {
      if (seq === loadSeq) loading.value = false
    }
  }

  async function send(text: string): Promise<void> {
    const body = text.trim()
    const mainID = mainSessionID.value
    if (!body || !mainID) return
    const sessionID = await ensure(mainID)
    if (streaming.value) return
    draft.value = ''

    messages.value.push({
      id: `local-side-${Date.now()}`,
      session_id: sessionID,
      role: 'user',
      content: body,
      status: 'completed',
      model: null,
      created_at: Date.now(),
      updated_at: Date.now()
    } as unknown as ApiMessageT)

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
    pendingApprovals.value = []
    streamingTurn.value = 0

    try {
      const handle = streamChat(
        { message: body, session_id: sessionID },
        (event: ChatStreamEvent) => handleEvent(event),
        { onReconnect: () => void loadMessages(sessionID, { replace: true }) }
      )
      activeHandle = handle
      await handle.promise
      await loadMessages(sessionID, { replace: true })
      finalize(sessionID)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      try {
        await loadMessages(sessionID, { replace: true })
      } catch {
        // swallow
      }
      finalize(sessionID)
      toast.error(t('chat.streamFailed'), error.value ?? undefined)
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
      streamingRetry.value = null
    }
  }

  /** 收尾：权威快照缺正文/过程块时用本地流式累积兜底（与主链路 resolveRunAssistant 同口径）。 */
  function finalize(sessionID: string): void {
    const content = streamingContent.value.trim()
    const hasPayload = content.length > 0 || streamingBlocks.value.length > 0
    if (!hasPayload && !error.value) return
    const verdict = resolveRunAssistant(messages.value, content)
    if (verdict === 'dedupe') return
    if (verdict !== 'missing') {
      const m = verdict.message
      m.content = content || error.value || t('chat.streamFailedPlaceholder')
      m.status = 'completed'
      m.updated_at = Date.now()
      if ((!m.blocks || m.blocks.length === 0) && streamingBlocks.value.length > 0) {
        m.blocks = streamingBlocksToMessageBlocks(streamingBlocks.value, m.id)
      }
      return
    }
    if (!content && !error.value) return
    messages.value.push({
      id: `local-side-a-${Date.now()}`,
      session_id: sessionID,
      role: 'assistant',
      content: content || error.value || t('chat.streamFailedPlaceholder'),
      status: 'completed',
      model: null,
      created_at: Date.now(),
      updated_at: Date.now()
    } as unknown as ApiMessageT)
  }

  async function cancel(): Promise<void> {
    const id = sideSession.value?.id
    if (id) {
      try {
        await apiPost(`/api/v1/chat/sessions/${id}/cancel`)
      } catch {
        // swallow
      }
    }
    activeHandle?.cancel()
    streaming.value = false
  }

  /** 审批闭环（辅助会话的工具同样受执行模式约束）。 */
  async function decideApproval(id: string, approved: boolean): Promise<void> {
    try {
      await apiPost(`/api/v1/chat/approval/${id}/decide`, { approved })
      pendingApprovals.value = pendingApprovals.value.filter((x) => x.id !== id)
    } catch (e) {
      toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
    }
  }

  /** 面板关闭/主会话切换：终止在飞请求，落盘的会话与消息留在后端。 */
  function reset(): void {
    activeHandle?.cancel()
    activeHandle = null
    sideSession.value = null
    messages.value = []
    draft.value = ''
    streaming.value = false
    streamingContent.value = ''
    streamingThinking.value = ''
    streamingBlocks.value = []
    pendingApprovals.value = []
    error.value = null
    stopReason.value = null
  }

  return {
    mainSessionID,
    sideSession,
    loading,
    messages,
    draft,
    streaming,
    streamingContent,
    streamingThinking,
    streamingBlocks,
    streamingStats,
    streamingTurn,
    streamingRetry,
    pendingApprovals,
    error,
    stopReason,
    open,
    ensure,
    send,
    cancel,
    decideApproval,
    reset
  }
})
