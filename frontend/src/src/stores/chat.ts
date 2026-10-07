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
import { useToastStore } from './toast'

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
  // 上下文水位：ratio 为 0-100 整数；windowKnown=false 表示这个模型的窗口是估算值
  const contextUsed = ref(0)
  const contextWindow = ref(0)
  const contextRatio = ref(0)
  const contextKnown = ref(false)

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
    contextKnown.value = false
  }

  // 思考内容可能被用户关掉：关掉后不累积，也不出现在历史渲染里。
  let showThinking = true

  function setShowThinking(on: boolean) {
    showThinking = on
    if (!on) thinking.value = ''
  }

  function isThinkingVisible() {
    return showThinking
  }

  // 返回后端分配的用户条目 id，供调用方立刻回显这条消息。
  async function send(sessionId: string, content: string, attachments?: AttachmentREQ[]) {
    lastError.value = ''
    const resp = await api.chat.send(sessionId, content, attachments)
    running.value = true
    return resp
  }

  // 停止：后端广播 chat:stopped 才是权威信号，这里只负责把请求发出去。
  // 提前把 running 置 false 会和后端实际状态脱节——内核可能还没真正停下。
  async function stop(sessionId: string) {
    await api.chat.stop(sessionId)
  }

  async function steer(sessionId: string, content: string) {
    await api.chat.steer(sessionId, content)
  }

  async function decide(approvalId: string, approved: boolean, scope = 'once') {
    try {
      if (approved) await api.approvals.approve(approvalId, scope)
      else await api.approvals.deny(approvalId)
      approvals.value = approvals.value.filter((a) => a.id !== approvalId)
    } catch (e) {
      // 决策没生效就不能移除卡片，否则用户以为点过了，工具却一直卡着
      useToastStore().bad(`操作失败：${(e as Error)?.message || '请重试'}`)
    }
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
    if (data.kind === 'thinking') {
      if (showThinking) thinking.value += data.delta
    } else streaming.value += data.delta
  }

  function onToolStart(data: ToolStartData) {
    // 断线重连重放可能把同一条 tool_start 再送一次：按 tool_call_id 去重，
    // 不然运行列表里出现重复工具卡，渲染也会因重复 key 异常
    if (runs.value.some((r) => r.tool_call_id === data.tool_call_id)) return
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

  // 连接断过之后以服务端为准：断线瞬间已经决策掉的审批，本地那份还挂着。
  async function syncApprovals(sessionId: string) {
    if (!sessionId) return
    try {
      const list = await api.approvals.pending(sessionId)
      approvals.value = list.filter((a) => a.status === 'pending')
    } catch {
      // 对不上就保持原样：宁可多显示一张卡，也不要把还没决策的请求弄没
    }
  }

  function onDone(data: DoneData) {
    running.value = false
    // 收尾后用权威用量校准水位：SSE 的 context 是最后一轮开始时的值，
    // 不含最后一轮回复本身，差的那截在这里补上。
    if (data.usage) {
      contextUsed.value = data.usage.context
      if (data.usage.context > 0 && contextWindow.value > 0) {
        contextRatio.value = Math.min(100, Math.round((data.usage.context / contextWindow.value) * 100))
      }
    }
    // 收尾后清掉运行态记录：历史消息里已经有这些工具了，不清会渲染两遍。
    runs.value = []
    thinking.value = ''
    streaming.value = ''
    // 审批卡也要清：这份列表只在本窗口点过「放行/拒绝」时才会移除自己。
    // 决策若来自别处（SSE 断开、另一个窗口、或直接调接口），
    // 这张卡就会永远挂在已经结束的回合下面，用户看到的是一条永远等不着的请求。
    approvals.value = []
  }

  // 用户主动停止：立刻退出运行态，不等内核走完收尾。
  // 后端随后仍会补发 chat:done 做最终对账，两者幂等。
  function onStopped() {
    running.value = false
    notice.value = '已停止'
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
    contextKnown.value = data.known !== false
    contextRatio.value = Math.max(0, Math.min(100, Math.round(data.ratio || 0)))
  }

  function notify(text: string) {
    notice.value = text
  }

  function onGap() {
    notice.value = '连接出现过中断，正在对账刷新'
    // 断线期间 run 可能已经结束而 done 事件丢了（重放窗口溢出时无法补发）。
    // 这里先退出运行态，快照刷新给出权威消息列表；若 run 还在跑，
    // 后续 delta / done 事件到来时会自然恢复。
    running.value = false
    runs.value = []
    streaming.value = ''
    thinking.value = ''
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
    contextKnown,
    reset,
    send,
    stop,
    steer,
    decide,
    notify,
    setShowThinking,
    isThinkingVisible,
    onStart,
    onDelta,
    onToolStart,
    onToolEnd,
    onApproval,
    onDone,
    onStopped,
    onError,
    onCompressed,
    onContext,
    onGap,
    syncApprovals,
  }
})
