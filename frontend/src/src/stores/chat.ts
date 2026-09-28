import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type {
  ApprovalData,
  ApprovalVO,
  AttachmentREQ,
  CompressedData,
  ContextData,
  DeltaData,
  DoneData,
  StartData,
  ToolEndData,
  ToolStartData,
} from '../types/api'

// 当前正在跑的工具调用：流式期间只做「展示」，chat:done 后用权威快照覆盖。
export interface ToolRun {
  tool_call_id: string
  tool: string
  label: string
  args: Record<string, unknown>
  running: boolean
  ok: boolean
  title: string
  output: string
  duration_ms: number
}

export const useChatStore = defineStore('chat', () => {
  const running = ref(false)
  const thinking = ref('')
  const streaming = ref('')
  const runs = ref<ToolRun[]>([])
  const approvals = ref<ApprovalVO[]>([])
  const lastError = ref('')
  const notice = ref('')
  // 上下文水位：ratio 为 0-100 整数，0 表示这个模型没告诉窗口多大
  const contextUsed = ref(0)
  const contextWindow = ref(0)
  const contextRatio = ref(0)

  function reset() {
    running.value = false
    thinking.value = ''
    streaming.value = ''
    runs.value = []
    approvals.value = []
    lastError.value = ''
    notice.value = ''
    contextUsed.value = 0
    contextWindow.value = 0
    contextRatio.value = 0
  }

  async function send(sessionId: string, content: string, attachments?: AttachmentREQ[]) {
    lastError.value = ''
    await api.chat.send(sessionId, content, attachments)
    running.value = true
  }

  async function stop(sessionId: string) {
    await api.chat.stop(sessionId)
  }

  async function steer(sessionId: string, content: string) {
    await api.chat.steer(sessionId, content)
  }

  async function decide(approvalId: string, approved: boolean, scope = 'once') {
    if (approved) await api.approvals.approve(approvalId, scope)
    else await api.approvals.deny(approvalId)
    approvals.value = approvals.value.filter((a) => a.id !== approvalId)
  }

  // ---- 事件入口（SSE → store） ----
  function onStart(_data: StartData) {
    running.value = true
    thinking.value = ''
    streaming.value = ''
    runs.value = []
    lastError.value = ''
  }

  function onDelta(data: DeltaData) {
    if (data.kind === 'thinking') thinking.value += data.delta
    else streaming.value += data.delta
  }

  function onToolStart(data: ToolStartData) {
    runs.value.push({
      tool_call_id: data.tool_call_id,
      tool: data.tool,
      label: data.label || data.tool,
      args: data.args,
      running: true,
      ok: true,
      title: '',
      output: '',
      duration_ms: 0,
    })
  }

  function onToolEnd(data: ToolEndData) {
    const run = runs.value.find((r) => r.tool_call_id === data.tool_call_id)
    if (!run) return
    run.running = false
    run.ok = data.ok
    run.title = data.title
    run.output = data.output
    run.duration_ms = data.duration_ms
  }

  function onApproval(data: ApprovalData) {
    approvals.value.push({
      id: data.approval_id,
      session_id: '',
      tool_call_id: data.tool_call_id,
      tool: data.tool,
      label: data.label,
      args: data.args,
      risk: data.risk,
      reason: data.reason,
      status: 'pending',
      created_at: 0,
      decided_at: 0,
    })
  }

  function onDone(_data: DoneData) {
    running.value = false
  }

  function onError(data: { code: number; message: string }) {
    running.value = false
    lastError.value = data.message
  }

  // 上下文被裁剪：只说一句发生了什么，不弹窗不打断
  function onCompressed(data: CompressedData) {
    notice.value = `对话太长了，已整理较早的内容（${data.tokens_before} → ${data.tokens_after}）`
  }

  function onContext(data: ContextData) {
    contextUsed.value = data.used
    contextWindow.value = data.window
    contextRatio.value = Math.max(0, Math.min(100, Math.round(data.ratio || 0)))
  }

  function notify(text: string) {
    notice.value = text
  }

  function onGap() {
    notice.value = '连接出现过中断，正在对账刷新'
  }

  return {
    running,
    thinking,
    streaming,
    runs,
    approvals,
    lastError,
    notice,
    contextUsed,
    contextWindow,
    contextRatio,
    reset,
    send,
    stop,
    steer,
    decide,
    notify,
    onStart,
    onDelta,
    onToolStart,
    onToolEnd,
    onApproval,
    onDone,
    onError,
    onCompressed,
    onContext,
    onGap,
  }
})
