import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import { ApiError } from '../api/http'
import type {
  ApprovalData,
  ApprovalVO,
  AttachmentREQ,
  CompressedData,
  ContextData,
  DeltaData,
  DoneData,
  MessageVO,
  ModelCapability,
  SessionVO,
  StartData,
  ToolEndData,
  ToolStartData,
} from '../types/api'
import { useSettingsStore } from './settings'
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
  // 本轮因输出上限被截断：正文可能只写了一半，工具调用一律作废。
  // 这件事必须让用户看见并能一键续上，否则表现就是「助手说着说着没了」。
  const truncated = ref(false)
  // 撞上限时那一次实际下发的输出预算。界面必须报出这个数字：
  // 「撞的是我们给的额度」和「撞的是厂商硬限制」在信号上分不出来，
  // 不给数字，用户就没法判断该不该去把额度调大。
  const truncBudget = ref(0)
  // 上下文水位：ratio 为 0-100 整数，分母是「可用预算」（窗口减掉给输出留的余量），
  // 与助手真正开始整理较早内容的位置一致；contextKnown=false 表示窗口是估算值
  const contextUsed = ref(0)
  const contextWindow = ref(0)
  const contextRatio = ref(0)
  const contextKnown = ref(false)
  const contextReserve = ref(0)
  // 排队中的插话：与后端队列一一对应的本地视图，注入时刻随 chat:user 消掉。
  const queued = ref<string[]>([])

  function reset() {
    running.value = false
    thinking.value = ''
    streaming.value = ''
    runs.value = []
    approvals.value = []
    lastError.value = ''
    notice.value = ''
    truncated.value = false
    truncBudget.value = 0
    contextUsed.value = 0
    contextWindow.value = 0
    contextRatio.value = 0
    contextKnown.value = false
    contextReserve.value = 0
    queued.value = []
  }

  // 思考内容可能被用户关掉：关掉后不累积，也不出现在历史渲染里。
  let showThinking = true

  function setShowThinking(on: boolean) {
    showThinking = on
    if (!on) thinking.value = ''
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

  // 排队：后端与插话同一个队列，这里只多记一份本地视图供排队条展示；
  // 注入时刻的 chat:user 会按内容消掉对应条目。
  async function followUp(sessionId: string, content: string) {
    await api.chat.followUp(sessionId, content)
    queued.value.push(content)
  }

  function onDequeued(content: string) {
    const i = queued.value.indexOf(content)
    if (i >= 0) queued.value.splice(i, 1)
    else if (queued.value.length) queued.value.shift()
  }

  function clearQueued() {
    queued.value = []
  }

  async function decide(approvalId: string, approved: boolean, scope = 'once') {
    try {
      if (approved) await api.approvals.approve(approvalId, scope)
      else await api.approvals.deny(approvalId)
      approvals.value = approvals.value.filter((a) => a.id !== approvalId)
    } catch (e) {
      // 4102 = 这张审批的等待已经结束（超时 / 会话被取消 / 已在别处决策）：
      // 它永远点不动了，摘掉卡片并说清原因，否则用户对着一个点不掉的卡反复尝试。
      if (e instanceof ApiError && e.code === 4102) {
        approvals.value = approvals.value.filter((a) => a.id !== approvalId)
        useToastStore().info('这张确认已经失效（等待超时或已在别处处理）')
        return
      }
      // 其他失败（本地服务抖动等）保留卡片：决策没生效就移除，
      // 用户以为点过了，工具却一直卡着。
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
    truncated.value = false
    truncBudget.value = 0
    // 上一轮的「已停止 / 连接中断」提示必须清掉：留着它会挂在下一轮顶部，
    // 用户刚点发送就看到一句旧的对账提示，会以为这次又断了。
    notice.value = ''
  }

  function onDelta(data: DeltaData) {
    // delta 到达即说明 run 还活着：对账（onGap）可能提前退了运行态，
    // 不在这里补回来的话，live 回合的 v-if 关着，正文攒到下次快照才突然出现。
    running.value = true
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
    // stop_reason 是内核唯一的收尾口径，前端必须照实表达：
    // length = 撞到输出上限（正文半截、工具作废），max_turns = 步数用尽。
    // 不说的话，界面看起来就是「助手莫名其妙不说话了」。
    switch (data.stop_reason) {
      case 'length':
        // 不再另发一句 notice：流末尾那条警告已经把事情和「继续」都讲清楚了，
        // 两处说同一件事只会稀释注意力。
        truncated.value = true
        truncBudget.value = data.max_tokens || 0
        break
      case 'max_turns':
        notice.value = '这一轮步骤太多，已经停下。可以点「继续」接着做'
        break
    }
    // 收尾后用权威用量校准水位：SSE 的 context 是最后一轮开始时的值，
    // 不含最后一轮回复本身，差的那截在这里补上。
    if (data.usage) {
      seed(data.usage.context, contextWindow.value, contextKnown.value)
    }
    // 收尾后清掉运行态记录：历史消息里已经有这些工具了，不清会渲染两遍。
    runs.value = []
    thinking.value = ''
    streaming.value = ''
    // 审批卡也要清：这份列表只在本窗口点过「放行/拒绝」时才会移除自己。
    // 决策若来自别处（SSE 断开、另一个窗口、或直接调接口），
    // 这张卡就会永远挂在已经结束的回合下面，用户看到的是一条永远等不着的请求。
    approvals.value = []
    // 没来得及消费的排队消息由后端 flush 兜底落库（稍后以 chat:user 到达，
    // 时间线里有它们），排队条到此收口。
    queued.value = []
  }

  // 用户主动停止：立刻退出运行态，不等内核走完收尾。
  // 后端随后仍会补发 chat:done 做最终对账，两者幂等。
  function onStopped() {
    running.value = false
    notice.value = '已停止'
    queued.value = []
  }

  function onError(data: { code: number; message: string }) {
    running.value = false
    lastError.value = data.message
    queued.value = []
  }

  // 上下文被裁剪：只说一句发生了什么，不弹窗不打断
  function onCompressed(data: CompressedData) {
    notice.value = `对话太长了，已整理较早的内容（${data.tokens_before} → ${data.tokens_after}）`
  }

  function onContext(data: ContextData) {
    contextUsed.value = data.used
    contextWindow.value = data.window
    contextKnown.value = data.known !== false
    contextReserve.value = data.reserve || 0
    contextRatio.value = Math.max(0, Math.min(100, Math.round(data.ratio || 0)))
  }

  // ---- 快照回填：进入 / 切换会话立刻有水位读数，不必等下一轮 chat:context ----

  // 模型能力按「服务 + 模型」缓存：切会话时同一模型不重复请求。
  const capCache = new Map<string, ModelCapability>()

  function seed(used: number, win: number, known: boolean, reserve = 0) {
    contextUsed.value = Math.max(0, used || 0)
    if (win > 0) contextWindow.value = win
    if (reserve > 0) contextReserve.value = reserve
    contextKnown.value = win > 0 ? known : false
    // 与后端 ratioOf 同一条公式：分母是可用预算，不是整窗口（否则读数会比整理线慢半拍）。
    // 这里只能拿模型默认余量（能力画像里的 max_output）估，用户手填过余量时以下一轮
    // chat:context 的权威值为准。
    const usable = win - reserve > 0 ? win - reserve : Math.floor(win / 2)
    contextRatio.value =
      usable > 0
        ? Math.max(0, Math.min(100, Math.round((contextUsed.value / usable) * 100)))
        : 0
  }

  // 已用量取最近一条带 usage.context 的助手条目的值：它就是「下一轮发出时的上下文量」。
  function lastContextOf(messages: MessageVO[]): number {
    for (let i = messages.length - 1; i >= 0; i--) {
      const m = messages[i]
      if (m.role === 'assistant' && (m.usage?.context || 0) > 0) return m.usage?.context || 0
    }
    return 0
  }

  // 空态（还没建会话）时用默认模型把窗口刻度填上：水位环一开始就显示
  // 「0 / 128k」，用户不必等第一轮回答才知道这个窗口多大。
  async function seedDefaultWindow() {
    const boot = useSettingsStore().boot
    const model = boot?.default_model
    if (!model) return
    const key = `${boot?.default_provider_id || ''}|${model}`
    let cap = capCache.get(key)
    if (!cap) {
      try {
        cap = await api.models.capability(model, boot?.default_provider_id)
        capCache.set(key, cap)
      } catch {
        return
      }
    }
    if (cap && cap.context_window > 0 && contextUsed.value === 0) {
      seed(0, cap.context_window, cap.known !== false, cap.max_output || 0)
    }
  }

  async function seedContext(sess: SessionVO | null, messages: MessageVO[]) {
    if (running.value) return
    if (!sess) {
      void seedDefaultWindow()
      return
    }
    const used = lastContextOf(messages)
    const key = `${sess.provider_id}|${sess.model}`
    let cap = capCache.get(key)
    if (!cap && sess.model) {
      try {
        cap = await api.models.capability(sess.model, sess.provider_id)
        capCache.set(key, cap)
      } catch {
        // 能力查不到不是错误：窗口留给下一轮 chat:context，这里先把已用量放上去
      }
    }
    if (cap && cap.context_window > 0) {
      // 余量按模型默认值（能力画像的 max_output）估：手填过 context_reserve_tokens
      // 的用户，读数会在下一轮 chat:context 用权威值纠正。
      seed(used, cap.context_window, cap.known !== false, cap.max_output || 0)
    } else {
      seed(used, contextWindow.value, contextKnown.value, contextReserve.value)
    }
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
    truncated,
    truncBudget,
    contextUsed,
    contextWindow,
    contextRatio,
    contextKnown,
    contextReserve,
    queued,
    reset,
    send,
    stop,
    steer,
    followUp,
    onDequeued,
    clearQueued,
    decide,
    notify,
    setShowThinking,
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
    seedContext,
  }
})
