// 子代理状态（仅 omp）：roster、进度、transcript 游标。
// 两条数据源并用：get_subagents 权威快照对账，subagent_progress 高频增量——
// 只靠推送会漏（终态后 id 从 roster 消失），只靠拉取会卡（进度要秒级）。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '@/api/http'
import type { AgentRow, AgentProgress, TranscriptPage } from '@/types/api'

export const useAgentsStore = defineStore('agents', () => {
  const roster = ref<AgentRow[]>([])
  const selected = ref('')
  const messages = ref<Record<string, unknown[]>>({})
  const cursors = ref<Record<string, number>>({})
  const open = ref(false)
  const loading = ref(false)
  const subscribed = ref(false)

  // 脏集合：progress 帧把它们标脏，500ms 合并后逐个拉一段。
  // 每帧都拉就是几十次 IPC，合并后用户看不出延迟。
  const dirty = new Set<string>()
  let flushTimer: number | undefined

  const running = computed(() => roster.value.filter((a) => a.status === 'running' || a.status === 'pending'))
  const active = computed(() => roster.value.find((a) => a.id === selected.value) ?? null)

  /** 打开面板时调一次：默认订阅档位是 off，不打开就什么都看不到。 */
  async function ensureSubscribed() {
    if (subscribed.value) return
    try {
      await api.subscribeAgents('progress')
      subscribed.value = true
    } catch {
      // 引擎不支持或没起来：保持 subscribed=false，下次再试
    }
  }

  async function refresh() {
    loading.value = true
    try {
      const data = await api.agents()
      roster.value = data.agents ?? []
      if (selected.value && !roster.value.some((a) => a.id === selected.value)) {
        selected.value = ''
      }
    } finally {
      loading.value = false
    }
  }

  /** 会话切换后由 app store 调用：引擎侧已丢弃旧 id 的帧，本地也必须清。 */
  function reset() {
    roster.value = []
    selected.value = ''
    messages.value = {}
    cursors.value = {}
    dirty.clear()
    subscribed.value = false
  }

  function applyLifecycle(data: Record<string, unknown>) {
    const id = String(data.id ?? '')
    if (!id) return
    const status = String(data.status ?? '')
    const existing = roster.value.find((a) => a.id === id)
    if (status === 'started') {
      if (!existing) {
        roster.value = [...roster.value, normalizeAgent(data)].sort(byIndex)
      }
    } else if (existing) {
      existing.status = status
    }
    dirty.add(id)
    scheduleFlush()
  }

  function applyProgress(data: Record<string, unknown>) {
    const id = String(data.id ?? '')
    if (!id) return
    const target = roster.value.find((a) => a.id === id)
    if (!target) return
    target.progress = normalizeProgress(data.progress)
    if (typeof data.task === 'string' && data.task) target.task = data.task
    if (typeof data.session_file === 'string' && data.session_file) target.session_file = data.session_file
    dirty.add(id)
    scheduleFlush()
  }

  function scheduleFlush() {
    if (flushTimer) return
    flushTimer = window.setTimeout(() => {
      flushTimer = undefined
      void flushDirty()
    }, 500)
  }

  async function flushDirty() {
    const ids = [...dirty]
    dirty.clear()
    for (const id of ids) {
      try {
        const page = await api.agentMessages(id, cursors.value[id] ?? 0)
        applyPage(id, page)
      } catch {
        // 单个子代理拉取失败不影响其他
      }
    }
  }

  function applyPage(id: string, page: TranscriptPage) {
    if (page.reset) {
      messages.value[id] = []
      cursors.value[id] = 0
    }
    messages.value = { ...messages.value, [id]: [...(messages.value[id] ?? []), ...(page.messages ?? [])].slice(-400) }
    cursors.value = { ...cursors.value, [id]: page.next ?? cursors.value[id] ?? 0 }
  }

  async function select(id: string) {
    selected.value = id
    if (messages.value[id]?.length) return
    try {
      const page = await api.agentMessages(id, cursors.value[id] ?? 0)
      applyPage(id, page)
    } catch {
      /* 拉不到就等下一次 progress 触发 */
    }
  }

  async function steer(id: string, message: string) {
    await api.steerAgent(id, message)
  }

  async function cancel(id: string) {
    return api.cancelAgent(id)
  }

  return {
    roster,
    running,
    active,
    selected,
    messages,
    open,
    loading,
    subscribed,
    ensureSubscribed,
    refresh,
    reset,
    applyLifecycle,
    applyProgress,
    select,
    steer,
    cancel,
  }
})

function byIndex(a: AgentRow, b: AgentRow): number {
  return a.index - b.index || (a.id < b.id ? -1 : 1)
}

function normalizeAgent(raw: Record<string, unknown>): AgentRow {
  return {
    id: String(raw.id ?? ''),
    index: Number(raw.index ?? 0),
    agent: String(raw.agent ?? ''),
    agent_source: String(raw.agent_source ?? ''),
    description: String(raw.description ?? ''),
    status: String(raw.status ?? 'pending'),
    task: String(raw.task ?? ''),
    session_file: String(raw.session_file ?? ''),
    parent_tool_call_id: String(raw.parent_tool_call_id ?? ''),
    last_update: Number(raw.last_update ?? 0),
    progress: normalizeProgress(raw.progress),
  }
}

function normalizeProgress(raw: unknown): AgentProgress {
  const p = (raw ?? {}) as Record<string, unknown>
  const recent = Array.isArray(p.recentTools) ? p.recentTools : []
  return {
    completion: Number(p.completionPercent ?? 0),
    stage: String(p.stage ?? ''),
    current_tool: String(p.currentTool ?? ''),
    recent_tools: recent
      .map((t) => (t && typeof t === 'object' ? String((t as Record<string, unknown>).tool ?? '') : ''))
      .filter(Boolean),
    tool_count: Number(p.toolCount ?? 0),
    tokens: Number(p.tokens ?? 0),
    context_used: Number(p.contextTokens ?? 0),
    context_max: Number(p.contextWindow ?? 0),
    cost: Number(p.cost ?? 0),
    duration_ms: Number(p.durationMs ?? 0),
    model: String(p.resolvedModel ?? ''),
  }
}
