// 子代理观测状态：活跃清单 + 每条的流式过程。
// 事件来自父会话通道的 agent:* 帧，与对话流共用一条 SSE 连接。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import type { DiffLine } from '../types/chatEvents'

/** 子代理里的一次工具调用（展示用）。 */
export interface AgentToolRun {
  /** 调用 id */
  tool_call_id: string
  /** 工具名 */
  tool_name: string
  /** 参数 */
  tool_args: Record<string, unknown>
  /** 是否还在跑 */
  running: boolean
  /** 是否错误 */
  is_error: boolean
  /** 结果文本（后端已截断） */
  result: string
  /** 耗时毫秒 */
  duration_ms: number
}

/** 一条子代理记录：界面上一行 + 展开后的过程。 */
export interface AgentRow {
  /** 子会话 id（SUB_ 前缀） */
  agent_session_id: string
  /** 所属父会话 id */
  agent_parent_session_id: string
  /** 角色码 */
  agent_role_code: string
  /** 任务描述 */
  agent_task: string
  /** 起建毫秒 */
  agent_started_at: number
  /** 结束毫秒（0 = 还在跑） */
  agent_finished_at: number
  /** 是否失败 */
  agent_failed: boolean
  /** 结论正文（结束后才有） */
  agent_conclusion: string
  /** 累计耗时毫秒 */
  agent_duration_ms: number
  /** 已收到的思考正文 */
  agent_thinking: string
  /** 已收到的正文 */
  agent_text: string
  /** 工具调用记录（按 tool_call_id 索引） */
  agent_tools: Record<string, AgentToolRun>
}

/** agent:start 载荷。 */
interface AgentStartData {
  agent_session_id: string
  agent_parent_session_id: string
  agent_role_code: string
  agent_task: string
  agent_started_at: number
}

/** agent:delta 载荷。 */
interface AgentDeltaData {
  agent_session_id: string
  agent_kind: 'text' | 'thinking'
  agent_delta: string
}

/** agent:tool_start 载荷。 */
interface AgentToolStartData {
  agent_session_id: string
  agent_tool_call_id: string
  agent_tool_name: string
  agent_tool_args: Record<string, unknown>
}

/** agent:tool_end 载荷。 */
interface AgentToolEndData {
  agent_session_id: string
  agent_tool_call_id: string
  agent_tool_name: string
  agent_tool_result: string
  agent_tool_error: boolean
  agent_duration_ms: number
}

/** agent:done 载荷。 */
interface AgentDoneData {
  agent_session_id: string
  agent_failed: boolean
  agent_conclusion: string
  agent_duration_ms: number
  agent_usage: { input: number; output: number; cached: number; cache_write: number }
}

/** 后端返回的活跃子代理（SubagentRunVO）。 */
interface AgentRunVO {
  run_session_id: string
  run_parent_session_id: string
  run_role_code: string
  run_task: string
  run_started_at: number
}

/** 一条空记录（只填运行期字段，过程字段留空）。 */
function blankOf(vo: AgentRunVO): AgentRow {
  return {
    agent_session_id: vo.run_session_id,
    agent_parent_session_id: vo.run_parent_session_id,
    agent_role_code: vo.run_role_code,
    agent_task: vo.run_task,
    agent_started_at: vo.run_started_at,
    agent_finished_at: 0,
    agent_failed: false,
    agent_conclusion: '',
    agent_duration_ms: 0,
    agent_thinking: '',
    agent_text: '',
    agent_tools: {},
  }
}

export const useAgentsStore = defineStore('agents', () => {
  /** 全部子代理（按开始时间倒序，最近的在最上） */
  const rows = ref<AgentRow[]>([])
  /** 后端活跃表已拉过 */
  const loaded = ref(false)
  /** 当前选中的子代理（详情面板） */
  const selected = ref('')
  /** 只显示某个父会话的（空 = 全部） */
  const scope = ref('')

  const list = computed(() => {
    const want = scope.value
    const all = want ? rows.value.filter((r) => r.agent_parent_session_id === want) : rows.value
    return [...all].sort((a, b) => b.agent_started_at - a.agent_started_at)
  })

  /** 正在跑的条数：导航角标与标题都用它。 */
  const liveCount = computed(() => list.value.filter((r) => !r.agent_finished_at).length)

  /** 选中的那条；没选就取列表第一条。 */
  const current = computed<AgentRow | null>(() => {
    const id = selected.value
    if (id) {
      const hit = rows.value.find((r) => r.agent_session_id === id)
      if (hit) return hit
    }
    return list.value[0] ?? null
  })

  /** 从后端拉一次活跃清单，与本地已有记录合并（过程正文不丢）。 */
  async function refresh(parentSessionId = '') {
    try {
      const resp = await api.agents.list(parentSessionId)
      const incoming = resp.resp_agents ?? []
      const known = new Map(rows.value.map((r) => [r.agent_session_id, r]))
      rows.value = incoming.map((vo) => known.get(vo.run_session_id) ?? blankOf(vo))
      loaded.value = true
    } catch {
      // 拉不到活跃表不挡界面：流式事件照样会建记录
      loaded.value = true
    }
  }

  /** 取一条记录；不存在就按事件里的最小字段建一条。 */
  function ensure(id: string, seed?: Partial<AgentRow>): AgentRow {
    let hit = rows.value.find((r) => r.agent_session_id === id)
    if (!hit) {
      hit = {
        ...blankOf({
          run_session_id: id,
          run_parent_session_id: seed?.agent_parent_session_id ?? '',
          run_role_code: seed?.agent_role_code ?? 'worker',
          run_task: seed?.agent_task ?? '',
          run_started_at: seed?.agent_started_at ?? Date.now(),
        }),
      }
      rows.value.push(hit)
    }
    return hit
  }

  // ---- SSE 入口：由 chatBridge 统一分发 ----

  function onStart(data: AgentStartData) {
    const row = ensure(data.agent_session_id)
    row.agent_parent_session_id = data.agent_parent_session_id
    row.agent_role_code = data.agent_role_code
    row.agent_task = data.agent_task
    row.agent_started_at = data.agent_started_at
    row.agent_finished_at = 0
    row.agent_failed = false
    row.agent_conclusion = ''
    row.agent_thinking = ''
    row.agent_text = ''
    row.agent_tools = {}
  }

  function onDelta(data: AgentDeltaData) {
    const row = ensure(data.agent_session_id)
    if (data.agent_kind === 'thinking') row.agent_thinking += data.agent_delta
    else row.agent_text += data.agent_delta
  }

  function onToolStart(data: AgentToolStartData) {
    const row = ensure(data.agent_session_id)
    row.agent_tools[data.agent_tool_call_id] = {
      tool_call_id: data.agent_tool_call_id,
      tool_name: data.agent_tool_name,
      tool_args: data.agent_tool_args ?? {},
      running: true,
      is_error: false,
      result: '',
      duration_ms: 0,
    }
  }

  function onToolEnd(data: AgentToolEndData) {
    const row = ensure(data.agent_session_id)
    const run = row.agent_tools[data.agent_tool_call_id]
    if (!run) return
    run.running = false
    run.is_error = data.agent_tool_error
    run.result = data.agent_tool_result
    run.duration_ms = data.agent_duration_ms
  }

  function onDone(data: AgentDoneData) {
    const row = ensure(data.agent_session_id)
    row.agent_finished_at = Date.now()
    row.agent_failed = data.agent_failed
    // 结论优先用事件里的：轮次用尽时它是最后一段正文，正文流可能被截断
    row.agent_conclusion = data.agent_conclusion
    row.agent_duration_ms = data.agent_duration_ms
  }

  /** 断线重连后的事件可能漏掉中间段：补拉一次活跃表与本条过程正文。 */
  async function reconcile(id: string) {
    const row = rows.value.find((r) => r.agent_session_id === id)
    if (!row || row.agent_finished_at) return
    try {
      const resp = await api.agents.transcript(id)
      row.agent_text = resp.resp_transcript || row.agent_text
    } catch {
      // 对账失败保持现状：界面上的过程至少是完整的开头
    }
  }

  function select(id: string) {
    selected.value = id
  }

  function clear() {
    rows.value = []
    selected.value = ''
    loaded.value = false
  }

  return {
    rows,
    loaded,
    selected,
    scope,
    list,
    liveCount,
    current,
    refresh,
    onStart,
    onDelta,
    onToolStart,
    onToolEnd,
    onDone,
    reconcile,
    select,
    clear,
  }
})

/** 工具结果里带的差异行（与对话流同形，供复用渲染）。 */
export type { DiffLine }
