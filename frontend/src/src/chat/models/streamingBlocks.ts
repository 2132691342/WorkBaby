/**
 * 流式块累积（streamingBlocks）纯函数模型层。
 *
 * 按事件到达顺序累积块数组（而非按维度拆成正文/工具/技能三份），渲染层按序展示，
 * 保持「文本 / 工具 / 文本 / 工具」的真实穿插；同类连续块原地合并以减少 DOM 节点。
 * 与持久化 blocks（chat/models/blocks.ts）共享同一组 kind，是流式期的高频暂存序列。
 */
import type { MessageBlockKind } from '@/types/api'

/** 块的最小渲染单位：与后端 message_blocks 同源但额外带 streaming 态。 */
export interface StreamingBlock {
  /** 块内稳定 id（事件到达时分配；同类合并时不变）。 */
  id: string
  kind: MessageBlockKind
  /** 块内顺序：与 seq 同义（保持 blocks.ts 的口径一致）。 */
  seq: number
  /** thinking / text 累计；tool_call / tool_result 完整 payload。 */
  text: string
  /** 结构化数据：tool_call → {id,name,arguments}；tool_result → {tool_call_id,name,content,...}。 */
  data: Record<string, unknown> | null
  /** 工具块状态：running 转圈中；stopped 是用户中断时的收尾态（保留已产出内容）。 */
  state?: 'running' | 'success' | 'error' | 'refused' | 'stopped'
  /** 工具调用 id（同 tool_call_id）；新增 result 块时按此关联。 */
  toolCallId?: string
  /** 后端给定的工具名（前端 ActivityDescription / RiskClassifier 渲染用）。 */
  name?: string
  /** 子 Agent 来源标签（delegate_task 委派的工具事件）。 */
  agent?: string
  /** 后端声明的活动描述（"正在编辑 app.ts"）。 */
  activity?: string
  /** tool_call 开始时刻；用于 running 计时。 */
  startedAt?: number
  /** tool_result 时记录的耗时（毫秒）。 */
  durationMs?: number
}

/**
 * 按事件增量更新块序列：thinking/text delta 合并到同类末块，tool_call / skill / genui / artifact
 * 新建块，tool_result 按 tool_call_id 合并。
 */
export function applyBlockUpdate(
  blocks: StreamingBlock[],
  update: BlockUpdate
): StreamingBlock[] {
  switch (update.kind) {
    case 'thinking':
      return appendText(blocks, 'thinking', update.text)
    case 'text':
      return appendText(blocks, 'text', update.text)
    case 'tool_call':
      return [...blocks, {
        id: update.id,
        kind: 'tool_call',
        seq: blocks.length,
        text: '',
        data: { id: update.id, name: update.name, arguments: update.arguments ?? '' },
        state: 'running',
        toolCallId: update.id,
        name: update.name,
        agent: update.agent,
        activity: update.activity,
        startedAt: update.now
      }]
    case 'tool_result': {
      // 合并到同 tool_call_id 的最近一个 running/success result；若已存在则覆盖内容。
      const idx = findResultIndex(blocks, update.toolCallId)
      if (idx >= 0) {
        const next = blocks.slice()
        next[idx] = {
          ...next[idx],
          text: update.content ?? '',
          data: {
            tool_call_id: update.toolCallId,
            name: update.name,
            content: update.content,
            error: update.error,
            duration_ms: update.durationMs,
            refused: update.refused,
            ...(update.data ?? {})
          },
          state: update.refused ? 'refused' : (update.error ? 'error' : 'success'),
          durationMs: update.durationMs ?? next[idx].durationMs,
          toolCallId: update.toolCallId,
          name: update.name
        }
        return next
      }
      return [...blocks, {
        id: `r-${update.toolCallId}`,
        kind: 'tool_result',
        seq: blocks.length,
        text: update.content ?? '',
        data: {
          tool_call_id: update.toolCallId,
          name: update.name,
          content: update.content,
          error: update.error,
          duration_ms: update.durationMs,
          refused: update.refused,
          ...(update.data ?? {})
        },
        state: update.refused ? 'refused' : (update.error ? 'error' : 'success'),
        durationMs: update.durationMs,
        toolCallId: update.toolCallId,
        name: update.name
      }]
    }
    case 'skill':
      return [...blocks, {
        id: update.id,
        kind: 'skill',
        seq: blocks.length,
        text: '',
        data: update.payload,
        name: update.payload?.name as string | undefined
      }]
    case 'genui':
      return [...blocks, {
        id: update.id,
        kind: 'genui',
        seq: blocks.length,
        text: '',
        data: update.payload
      }]
    case 'artifact':
      return [...blocks, {
        id: update.id,
        kind: 'artifact',
        seq: blocks.length,
        text: '',
        data: update.payload
      }]
    default:
      return blocks
  }
}

/** 同 kind 连续合并：避免每个 delta 都新增 DOM 节点。 */
function appendText(blocks: StreamingBlock[], kind: 'thinking' | 'text', delta: string): StreamingBlock[] {
  if (!delta) return blocks
  const last = blocks[blocks.length - 1]
  if (last && last.kind === kind) {
    const next = blocks.slice()
    next[next.length - 1] = { ...last, text: last.text + delta }
    return next
  }
  return [...blocks, {
    id: `${kind}-${blocks.length}`,
    kind,
    seq: blocks.length,
    text: delta,
    data: null
  }]
}

/** 找到指定 tool_call_id 对应的最近一个 result 块下标；不存在返回 -1。 */
function findResultIndex(blocks: StreamingBlock[], toolCallId: string): number {
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].kind === 'tool_result' && blocks[i].toolCallId === toolCallId) return i
  }
  return -1
}

/** 流式增量输入。 */
export type BlockUpdate =
  | { kind: 'thinking'; text: string }
  | { kind: 'text'; text: string }
  | { kind: 'tool_call'; id: string; name: string; arguments?: string; agent?: string; activity?: string; now: number }
  | { kind: 'tool_result'; toolCallId: string; name: string; content?: string; error?: string; durationMs?: number; refused?: boolean; data?: Record<string, unknown> | null }
  | { kind: 'skill'; id: string; payload: Record<string, unknown> }
  | { kind: 'genui'; id: string; payload: Record<string, unknown> }
  | { kind: 'artifact'; id: string; payload: Record<string, unknown> }

/**
 * 把流式累积序列 → 持久化 MessageBlock 形状（流式终态兜底）。
 *
 * 后端 chat:done 先于权威消息落库时，本地累积块序列直接用于 finalize：
 * 不依赖『流式气泡』已渲染，直接写入 messages[].blocks。
 */
export function streamingBlocksToMessageBlocks(blocks: StreamingBlock[], messageId: string) {
  const now = Date.now()
  return blocks.map((b, i) => ({
    id: `${messageId}-local-${i}`,
    message_id: messageId,
    seq: i,
    kind: b.kind,
    payload: b.data ? JSON.stringify(b.data) : b.text,
    created_at: now
  }))
}

/** 用于流式期 UI 提示：是否包含正文 / 工具 / 思考 / 产物。 */
export function summarizeStreaming(blocks: StreamingBlock[]): {
  thinking: boolean
  tools: number
  text: string
  skillHit: boolean
  artifacts: boolean
  genui: boolean
} {
  const out = { thinking: false, tools: 0, text: '', skillHit: false, artifacts: false, genui: false }
  for (const b of blocks) {
    if (b.kind === 'thinking') out.thinking = true
    else if (b.kind === 'text') out.text += b.text
    else if (b.kind === 'tool_call' || b.kind === 'tool_result') out.tools++
    else if (b.kind === 'skill') out.skillHit = true
    else if (b.kind === 'artifact') out.artifacts = true
    else if (b.kind === 'genui') out.genui = true
  }
  return out
}