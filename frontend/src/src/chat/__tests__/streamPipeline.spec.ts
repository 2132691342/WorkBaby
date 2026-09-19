import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatStreamEvent, Message, MessageBlock } from '@/types/api'
import { mapSSEEvent } from '@/api/stream'
import { applyStreamUpdate, decodeStreamEvent } from '@/stores/chat/ChatStreamDecoder'
import type { ApprovalRequest, StreamEventUpdate } from '@/stores/chat/ChatStreamDecoder'
import { StreamEventBatcher } from '@/stores/chat/StreamEventBatcher'
import { blocksToToolCalls, resolveMessageBlocks, stopReasonSeverity, stopReasonText } from '@/chat/models/blocks'

/**
 * 流式管线端到端契约：SSE 事件 → 解码为状态更新 → 批渲染落地 → 历史块还原与终态展示。
 * 这里只守跨模块的契约与边界（映射口径、批渲染时序、配对还原、终态文案），
 * 单个函数的内部细节不在本文件覆盖。
 */

// ===== 1. SSE 事件 → 前端事件 =====
describe('SSE → 前端事件', () => {
  it('增量 / 思考 / 工具调用 / 工具结果 映射', () => {
    expect(mapSSEEvent('chat:stream', { delta: 'hi' })).toEqual({ type: 'content', data: 'hi' })
    expect(mapSSEEvent('chat:thinking', { delta: 'reasoning' })).toEqual({ type: 'thinking', data: 'reasoning' })
    expect(mapSSEEvent('chat:tool', { id: 't1', name: 'exec', arguments: '{}' })).toEqual({
      type: 'tool_call',
      data: { id: 't1', name: 'exec', args: '{}', agent: '' }
    })

    // 工具结果按 error 字段区分成功/失败态
    expect(mapSSEEvent('chat:tool-result', { id: 't1', name: 'file_read', content: 'ok', error: '' })).toEqual({
      type: 'tool_result',
      data: { id: 't1', name: 'file_read', output: 'ok', state: 'success', agent: '' }
    })
    expect(mapSSEEvent('chat:tool-result', { id: 't1', name: 'exec', content: '', error: 'boom' })).toEqual({
      type: 'tool_result',
      data: { id: 't1', name: 'exec', output: '', state: 'error', agent: '' }
    })

    // 轮次与检查点事件（多轮任务进度可见）
    expect(mapSSEEvent('chat:turn-start', { turn: 2 })).toEqual({ type: 'turn_start', data: { turn: 2 } })
    expect(mapSSEEvent('chat:checkpoint', { turn: 2 })).toEqual({ type: 'checkpoint', data: { turn: 2 } })
  })

  it('终因归一化：LLM 原始 finish_reason 不直通前端，领域原因保留', () => {
    // end_turn / length 若不归一，横幅文案不命中
    expect(mapSSEEvent('chat:done', { status: 'completed', reason: 'end_turn', stop_reason: 'end_turn' }))
      .toEqual({ type: 'stopped', data: { reason: 'completed' } })
    expect(mapSSEEvent('chat:done', { stop_reason: 'length' }))
      .toEqual({ type: 'stopped', data: { reason: 'token_budget' } })
    expect(mapSSEEvent('chat:done', { reason: 'cancelled' }))
      .toEqual({ type: 'stopped', data: { reason: 'cancelled' } })

    // 解码器层同样落到 setStopReason
    expect(decodeStreamEvent({ type: 'stopped', data: { reason: 'cancelled' } })).toEqual({ setStopReason: 'cancelled' })

    expect(mapSSEEvent('chat:gap', { run_id: 'r1', last_seq: 3 }))
      .toEqual({ type: 'gap', data: { run_id: 'r1', last_seq: 3 } })
    expect(mapSSEEvent('chat:unknown', {})).toBeNull()
    expect(mapSSEEvent('workflow:started', {})).toBeNull()
  })
})

// ===== 2. 解码器：事件 → 状态更新 =====
describe('解码器', () => {
  it('审批请求 / 决策映射（can_remember 决定是否显示「本会话允许」）', () => {
    expect(
      decodeStreamEvent({
        type: 'tool_approval_request',
        data: { id: 'APR_1', command: 'go test', reason: '需要你确认', risk: 'irreversible', can_remember: false }
      })
    ).toEqual({
      setApproval: { id: 'APR_1', command: 'go test', reason: '需要你确认', risk: 'irreversible', canRemember: false }
    })
    expect(
      decodeStreamEvent({
        type: 'tool_approval_request',
        data: { id: 'APR_2', command: 'go test', reason: '需要你确认', risk: 'needs_approval', can_remember: true }
      })
    ).toEqual({
      setApproval: { id: 'APR_2', command: 'go test', reason: '需要你确认', risk: 'needs_approval', canRemember: true }
    })
    expect(decodeStreamEvent({ type: 'approval_decided', data: { id: 'APR_1', decision: 'approved' } }))
      .toEqual({ clearApproval: { id: 'APR_1', decision: 'approved' } })
    expect(decodeStreamEvent({ type: 'approval_decided', data: null })).toBeNull()
  })

  /** 展示用审批请求 → 未决审批（与生产入队形状一致）。 */
  function toPending(a: ApprovalRequest): import('@/types/api').PendingApproval {
    return { id: a.id, command: a.command, reason: a.reason, risk: a.risk ?? 'needs_approval', can_remember: a.canRemember }
  }

  /** 最小 ref state（满足 applyStreamUpdate 签名）。 */
  function makeState(pending: ApprovalRequest | null = null) {
    return {
      streamingContent: { value: '' },
      streamingThinking: { value: '' },
      streamingStats: { value: null },
      streamingArtifacts: { value: null },
      streamingGenUi: { value: null },
      pendingApprovals: { value: pending ? [toPending(pending)] : [] },
      error: { value: null as string | null },
      stopReason: { value: null as string | null }
    }
  }
  const sample = (id: string): ApprovalRequest => ({ id, command: 'go test', reason: '需确认' })

  it('审批队列：clearApproval 仅按 id 移除（防误清并发新请求），多请求按到达顺序排队', () => {
    const matched = makeState(sample('APR_1'))
    applyStreamUpdate({ clearApproval: { id: 'APR_1', decision: 'approved' } }, matched)
    expect(matched.pendingApprovals.value).toHaveLength(0)

    // id 不匹配：保留队列中的请求
    const mismatched = makeState(sample('APR_2'))
    applyStreamUpdate({ clearApproval: { id: 'APR_1', decision: 'approved' } }, mismatched)
    expect(mismatched.pendingApprovals.value).toEqual([toPending(sample('APR_2'))])

    const chain = makeState(null)
    applyStreamUpdate({ setApproval: sample('APR_1') }, chain)
    expect(chain.pendingApprovals.value).toEqual([toPending(sample('APR_1'))])

    // 并发第二个请求：入队而不是覆盖（此前单值 ref 会让 APR_1 永久看不到）
    applyStreamUpdate({ setApproval: sample('APR_2') }, chain)
    expect(chain.pendingApprovals.value.map((p) => p.id)).toEqual(['APR_1', 'APR_2'])

    applyStreamUpdate({ clearApproval: { id: 'APR_1', decision: 'denied' } }, chain)
    expect(chain.pendingApprovals.value.map((p) => p.id)).toEqual(['APR_2'])
  })

  it('子 Agent 生命周期：start 建 running 任务，done 合并终态，未 start 的 id 不产生悬空任务', () => {
    const start = decodeStreamEvent({ type: 'subagent_start', data: { sub_run_id: 'RUN_c', agent: 'coding' } })
    expect(start?.upsertTask?.state).toBe('running')
    expect(start?.upsertTask?.agent).toBe('coding')

    const state = { ...makeState(), tasks: { value: [] as import('@/types/api').BackgroundTask[] } }
    applyStreamUpdate(start!, state)
    const done = decodeStreamEvent({ type: 'subagent_done', data: { sub_run_id: 'RUN_c', agent: 'coding' } })
    applyStreamUpdate(done!, state)
    expect(state.tasks.value).toHaveLength(1)
    expect(state.tasks.value[0].state).toBe('completed')
    expect(state.tasks.value[0].finished_at).toBeGreaterThan(0)

    const err = decodeStreamEvent({ type: 'subagent_error', data: { sub_run_id: 'RUN_x', agent: 'research', message: 'boom' } })
    expect(err?.updateTask?.state).toBe('failed')
    applyStreamUpdate(err!, state)
    expect(state.tasks.value).toHaveLength(1)
  })
})

// ===== 3. 批渲染：一帧一渲染，内容不丢不重 =====
describe('批渲染', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  function recorder(): { calls: StreamEventUpdate[][]; apply: (u: StreamEventUpdate[]) => void } {
    const calls: StreamEventUpdate[][] = []
    return { calls, apply: (u) => calls.push(u) }
  }
  const content = (s: string): ChatStreamEvent => ({ type: 'content', data: s })

  it('批量合并：一帧内多条增量只 apply 一次且拼接完整；长流逐帧按序不丢', () => {
    const { calls, apply } = recorder()
    const b = new StreamEventBatcher(apply)
    b.push(content('你'))
    b.push(content('好'))
    b.push(content('呀'))
    expect(calls).toHaveLength(0)

    vi.advanceTimersByTime(50)
    expect(calls).toHaveLength(1)
    expect(calls[0].map((u) => u.appendContent).join('')).toBe('你好呀')

    // 长流：10 帧全部落地，顺序与内容无损
    const long = recorder()
    const b2 = new StreamEventBatcher(long.apply)
    for (let i = 0; i < 10; i++) {
      b2.push(content(String(i)))
      vi.advanceTimersByTime(50)
    }
    b2.flush()
    expect(long.calls).toHaveLength(10)
    expect(long.calls.flatMap((c) => c.map((u) => u.appendContent)).join('')).toBe('0123456789')
  })

  it('终态与审批事件不被延迟，且排在积压增量之后', () => {
    const { calls, apply } = recorder()
    const b = new StreamEventBatcher(apply)
    b.push(content('已生成内容'))
    b.push({ type: 'stopped', data: { reason: 'cancelled' } })
    expect(calls).toHaveLength(2)
    expect(calls[0][0].appendContent).toBe('已生成内容')
    expect(calls[1][0].setStopReason).toBe('cancelled')

    const r2 = recorder()
    const b2 = new StreamEventBatcher(r2.apply)
    b2.push({ type: 'tool_approval_request', data: { id: 'a1', command: 'rm -rf build', reason: 'danger', risk: 'irreversible' } })
    expect(r2.calls[0][0].setApproval?.risk).toBe('irreversible')
  })

  it('flush 幂等；dispose 丢弃未落定增量', () => {
    const { calls, apply } = recorder()
    const b = new StreamEventBatcher(apply)
    b.push(content('尾巴'))
    b.flush()
    b.flush()
    vi.advanceTimersByTime(50)
    expect(calls).toHaveLength(1)

    const r2 = recorder()
    const b2 = new StreamEventBatcher(r2.apply)
    b2.push(content('会被丢弃'))
    b2.dispose()
    vi.advanceTimersByTime(50)
    expect(r2.calls).toHaveLength(0)
  })
})

// ===== 4. 历史消息还原与终态展示 =====
describe('历史消息还原', () => {
  function block(seq: number, kind: MessageBlock['kind'], payload: unknown): MessageBlock {
    return {
      id: `BLOCK_${seq}`,
      message_id: 'MESSAGE_1',
      seq,
      kind,
      payload: typeof payload === 'string' ? payload : JSON.stringify(payload),
      created_at: 0
    }
  }
  function msg(fields: Partial<Message>): Message {
    return {
      id: 'MESSAGE_1', session_id: 'SESSION_1', seq: 2, run_id: 'RUN_1', role: 'assistant',
      content: 'done', tool_call_id: null, tool_calls_json: null, status: 'completed',
      stop_reason: null, model: null, created_at: 0, updated_at: 0,
      ...fields
    } as Message
  }

  it('块按 seq 升序还原，脏 payload 不抛错', () => {
    const m = msg({
      blocks: [
        block(2, 'tool_result', { tool_call_id: 't1', name: 'exec', content: 'ok' }),
        block(1, 'tool_call', { id: 't1', name: 'exec', arguments: '{"cmd":"ls"}' })
      ]
    })
    const out = resolveMessageBlocks(m)
    expect(out.map((b) => b.seq)).toEqual([1, 2])

    const broken = msg({ blocks: [block(1, 'tool_call', '{broken json')] })
    expect(resolveMessageBlocks(broken)[0].data).toBeNull()
  })

  it('无 blocks 时从 tool_calls_json 降级重建（旧历史消息）', () => {
    const m = msg({
      tool_calls_json: JSON.stringify([
        { id: 't9', type: 'function', function: { name: 'file_read', arguments: '{"path":"a.txt"}' } }
      ])
    })
    const out = resolveMessageBlocks(m)
    expect(out[0].kind).toBe('tool_call')
    expect(out[0].data).toMatchObject({ id: 't9', name: 'file_read' })
  })

  it('call/result 配对：成功、失败、拒绝（拒绝≠故障）、孤儿', () => {
    const paired = blocksToToolCalls([
      { kind: 'tool_call', seq: 1, data: { id: 't1', name: 'exec', arguments: '{"cmd":"ls"}' }, text: '' },
      { kind: 'tool_result', seq: 2, data: { tool_call_id: 't1', name: 'exec', content: 'ok', duration_ms: 12 }, text: '' }
    ])
    expect(paired).toEqual([
      { id: 't1', name: 'exec', args: '{"cmd":"ls"}', result: 'ok', state: 'success', duration_ms: 12 }
    ])

    const failed = blocksToToolCalls([
      { kind: 'tool_call', seq: 1, data: { id: 't1', name: 'exec' }, text: '' },
      { kind: 'tool_result', seq: 2, data: { tool_call_id: 't1', name: 'exec', error: 'boom', content: '' }, text: '' },
      { kind: 'tool_call', seq: 3, data: { id: 't2', name: 'file_write' }, text: '' },
      { kind: 'tool_result', seq: 4, data: { tool_call_id: 't2', name: 'file_write', refused: true, content: '{"refused":true}' }, text: '' }
    ])
    expect(failed[0].state).toBe('error')
    expect(failed[1].state).toBe('success')
    expect(failed[1].result).toContain('已拒绝')

    const orphan = blocksToToolCalls([
      { kind: 'tool_result', seq: 1, data: { tool_call_id: 'tx', name: 'exec', content: 'hi' }, text: '' }
    ])
    expect(orphan[0]).toMatchObject({ id: 'tx', name: 'exec', state: 'success' })
  })

  it('终因分级与文案（防横幅空白）', () => {
    const expected: [string | null, string][] = [
      ['completed', 'info'],
      [null, 'info'],
      ['cancelled', 'warning'],
      ['max_turns', 'warning'],
      ['tool_error_limit', 'warning'],
      ['token_budget', 'warning'],
      ['stagnation', 'warning'],
      ['error', 'error'],
      ['unknown_reason', 'error']
    ]
    for (const [reason, severity] of expected) {
      expect(stopReasonSeverity(reason as never)).toBe(severity)
    }
    for (const r of ['completed', 'cancelled', 'max_turns', 'tool_error_limit', 'token_budget', 'stagnation', 'error']) {
      expect(stopReasonText(r)).not.toBe('')
    }
  })
})
