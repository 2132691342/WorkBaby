// 对话的消息流：当前轮、历史轮、增量补丁、收尾。
// 三条纪律：流式正文只追加当前轮的 blocks（整表替换是 O(字数²)）；
// 收尾交给服务器的历史不再本地并；流式缓存用完即弃——跨轮与否的分界才说得清。
import { ref } from 'vue'
import { dropAllCaches, dropCache } from '@/utils/md'
import type { ChatBlock, ChatPatch, HistoryTurn, TurnUsage } from '@/types/api'

/** 界面上一轮正在直播的对话。 */
export interface Turn {
  run_id: string
  user_message: string
  blocks: ChatBlock[]
}

/** 后端 chat:turn_end 报上来的用量（已经是 snake_case）。 */
export interface TurnUsagePayload {
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
  cost_usd?: number
}

// 一轮的用量与耗时：形状与后端 TurnUsage 一致，另加耗时（只有流式那边有，
// 历史里没有时间线）。
export interface TurnReading extends TurnUsage {
  /** 这一轮花了多久（毫秒）；0 表示没有这个事实 */
  elapsed_ms: number
}

// 把后端的用量字段名转成界面自己的：历史轮与流式帧已是同一种形状，
// 这里只需补耗时，不需要猜两套命名。
export function readingFrom(payload: TurnUsagePayload | undefined, elapsedMs = 0): TurnReading | undefined {
  if (!payload) return undefined
  const input = payload.input_tokens ?? 0
  const output = payload.output_tokens ?? 0
  const cacheRead = payload.cache_read_tokens ?? 0
  const cacheWrite = payload.cache_write_tokens ?? 0
  const costUsd = payload.cost_usd ?? 0
  if (!input && !output && !cacheRead && !cacheWrite && !costUsd) return undefined
  return {
    input_tokens: input,
    output_tokens: output,
    cache_read_tokens: cacheRead,
    cache_write_tokens: cacheWrite,
    cost_usd: costUsd,
    elapsed_ms: elapsedMs,
  }
}

// 两份内容是不是同一轮：判据是「块数、每块种类与文本全一样」，两边来自同一份
// 数据；不用 id 或时间戳是因为两套数据没有共同标识，文本是最难出错的一份。
function sameBlocks(a: ChatBlock[], b: ChatBlock[]): boolean {
  if (!a.length || a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i].block_kind !== b[i].block_kind) return false
    if ((a[i].block_text ?? '') !== (b[i].block_text ?? '')) return false
  }
  return true
}

// 这一轮的块是不是已经全在历史里：历史投影会把一次回答里的多条助手消息并成
// 一轮，块数常比直播多，做法是在最后一条用户消息之后的助手块里搜连续子序列。
function blocksCovered(blocks: ChatBlock[], turns: HistoryTurn[]): boolean {
  if (!blocks.length) return false
  let start = 0
  for (let i = turns.length - 1; i >= 0; i--) {
    if (turns[i].turn_role === 'user') {
      start = i + 1
      break
    }
  }
  const hay: ChatBlock[] = []
  for (let i = start; i < turns.length; i++) hay.push(...(turns[i].turn_blocks ?? []))
  if (blocks.length > hay.length) return false
  for (let offset = 0; offset + blocks.length <= hay.length; offset++) {
    if (sameBlocks(blocks, hay.slice(offset, offset + blocks.length))) return true
  }
  return false
}

// 这一轮在服务器历史里到了哪一步：0=都不在、1=用户那句在、2=整轮都在——
// 直播位与历史位是两个来源，不判清楚同一轮会画两遍。
export function turnPresence(turns: HistoryTurn[], userMessage: string, blocks: ChatBlock[]): 0 | 1 | 2 {
  const tail = turns[turns.length - 1]
  if (!tail) return 0
  if (tail.turn_role === 'assistant' && blocksCovered(blocks, turns)) return 2
  if (tail.turn_role === 'user' && (tail.turn_blocks?.[0]?.block_text ?? '') === userMessage) return 1
  return 0
}

/** 对话的消息流。 */
export function useMessageFlow() {
  // 当前轮：一次「提问 → 回答」
  const turn = ref<Turn | null>(null)
  // 上一次收尾的 run id。只用于「收尾后才到的权威快照要丢掉」这一条判断。
  const lastFinishedRunId = ref('')
  // 历史轮（由后端投影好的轮次，界面直接渲染）
  const history = ref<HistoryTurn[]>([])
  // 每轮的用量与耗时，按 run id 记：收尾后界面还要用它渲染那一轮的读数。
  const usages = ref<Record<string, TurnReading>>({})
  // 本轮开始的时刻：消息脚标的秒表从它起走。
  const startedAt = ref(0)

  /** 增量应用：delta 只追加尾部，block 只替换那一块。 */
  function applyPatch(patch: ChatPatch): void {
    if (!turn.value || patch.run_id !== turn.value.run_id) return
    const blocks = turn.value.blocks
    if (patch.mode === 'delta') {
      const block = blocks[patch.index]
      if (block) block.block_text = (block.block_text ?? '') + (patch.delta ?? '')
      return
    }
    if (patch.mode === 'block' && patch.block) blocks[patch.index] = patch.block
  }

  /** 开一轮新的提问。 */
  function beginTurn(runID: string, userMessage: string): void {
    turn.value = { run_id: runID, user_message: userMessage, blocks: [] }
    startedAt.value = Date.now()
  }

  // 用服务器的历史覆盖本地：已在历史里的那一轮顺手撤下来——它在直播位与
  // 历史位各画一遍会变成同一段回复出现两次。
  function applyHistory(turns: HistoryTurn[]): void {
    history.value = turns
    const current = turn.value
    if (!current) return
    if (turnPresence(turns, current.user_message, current.blocks ?? []) === 2) turn.value = null
  }

  // 收尾：这一轮从流里出来了，但不并进本地历史——服务器历史才是那一轮的真身；
  // 服务器落盘比收尾帧晚一点，立刻撤会让这轮凭空消失一下再出现。
  function finishTurn(): void {
    if (!turn.value) {
      lastFinishedRunId.value = ''
      return
    }
    lastFinishedRunId.value = turn.value.run_id
    dropCache(turn.value.run_id)
  }

  // 记下这一轮的用量：引擎每条助手消息报一次「到目前为止」的累计值，取最后一次
  // 非零帧而不是累加（累加会把同一批 token 加好几遍）。
  function setUsage(runID: string, payload: TurnUsagePayload | undefined): void {
    if (!runID) return
    const prev = usages.value[runID]
    const elapsed = startedAt.value ? Date.now() - startedAt.value : 0
    const incoming = readingFrom(payload, prev?.elapsed_ms ?? elapsed)
    if (!incoming) {
      if (!prev) return
      usages.value = { ...usages.value, [runID]: { ...prev, elapsed_ms: prev.elapsed_ms || elapsed } }
      return
    }
    usages.value = { ...usages.value, [runID]: incoming }
  }

  /** 取一轮的读数。 */
  function usageOf(runID: string): TurnReading | undefined {
    return usages.value[runID]
  }

  /** 清空消息流（换会话 / 新会话时）。 */
  function reset(): void {
    turn.value = null
    lastFinishedRunId.value = ''
    history.value = []
    usages.value = {}
    startedAt.value = 0
    // 换会话时把渲染缓存全清掉：旧会话的正文不该一直占着内存。
    dropAllCaches()
  }

  return {
    turn,
    lastFinishedRunId,
    history,
    usages,
    startedAt,
    applyPatch,
    beginTurn,
    applyHistory,
    finishTurn,
    setUsage,
    usageOf,
    reset,
  }
}
