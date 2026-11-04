//! 会话回放链路：树分支、轮次分组、工具结果挂接、隐藏与压缩——历史视图的完整管线。
import { describe, expect, it } from 'vitest'
import { activePath, type PiEntry } from '../electron/main/service/parser/session_tree'
import { historyFromEntries, encodedWorkspace } from '../electron/main/service/session_service'

function entry(partial: Partial<PiEntry> & { id: string }): PiEntry {
  return { type: 'message', parentId: null, timestamp: '2026-01-01T00:00:00.000Z', ...partial } as PiEntry
}

function userMsg(id: string, parentId: string | null, text: string): PiEntry {
  return entry({ id, parentId, message: { role: 'user', content: text, timestamp: 1 } })
}

function assistantMsg(id: string, parentId: string | null, blocks: Record<string, unknown>[]): PiEntry {
  return entry({
    id,
    parentId,
    message: {
      role: 'assistant',
      content: blocks,
      model: 'test-model',
      usage: { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, totalTokens: 15, cost: { total: 0.01 } },
      stopReason: 'stop',
      timestamp: 2,
    },
  })
}

function toolResult(id: string, parentId: string | null, toolCallId: string, text: string, isError = false): PiEntry {
  return entry({
    id,
    parentId,
    message: {
      role: 'toolResult',
      toolCallId,
      toolName: 'bash',
      content: [{ type: 'text', text }],
      isError,
      timestamp: 3,
    },
  })
}

describe('会话回放', () => {
  it('一轮完整链路：用户 → 助手(文本+工具) → 工具结果 → 下一轮，输出与用量挂对位置', () => {
    const entries = [
      userMsg('a', null, '列出文件'),
      assistantMsg('b', 'a', [
        { type: 'text', text: '好的' },
        { type: 'toolCall', id: 'tc1', name: 'bash', arguments: { command: 'ls' } },
      ]),
      toolResult('c', 'b', 'tc1', 'file1.txt'),
      userMsg('d', 'c', '谢谢'),
    ]
    const turns = historyFromEntries(entries, new Set())
    expect(turns.map((t) => t.role)).toEqual(['user', 'assistant', 'user'])
    expect(turns[0].blocks[0].text).toBe('列出文件')
    const assistant = turns[1]
    expect(assistant.model).toBe('test-model')
    expect(assistant.usage?.output_tokens).toBe(5)
    const tool = assistant.blocks.find((b) => b.kind === 'tool')
    expect(tool?.tool_call_id).toBe('tc1')
    expect(tool?.output).toBe('file1.txt')
    expect(tool?.status).toBe('done')
    expect(turns[2].blocks[0].text).toBe('谢谢')
  })

  it('分叉会话沿叶子回放；隐藏轮剔除且失败的工具结果带错误态', () => {
    const a = userMsg('a', null, '第一问')
    const b = assistantMsg('b', 'a', [])
    const main = userMsg('main', 'b', '主分支')
    const side = userMsg('side', 'b', '旁支')
    const path = activePath([a, b, main, side])
    expect(path.map((e) => e.id)).toEqual(['a', 'b', 'side'])
    const turns = historyFromEntries([a, b, main, side], new Set(['a']))
    expect(turns.filter((t) => t.role === 'user').map((t) => t.blocks[0]?.text)).toEqual(['旁支'])
    // 失败工具结果落成独立工具轮
    const failed = historyFromEntries([toolResult('z', null, 'tc9', '炸了', true)], new Set())
    expect(failed[0].role).toBe('tool')
    expect(failed[0].blocks[0].is_error).toBe(true)
  })

  it('压缩条目转为引擎标记轮', () => {
    const compaction = entry({ type: 'compaction', id: 'x', summary: '摘要', tokensBefore: 5000 })
    const turns = historyFromEntries([compaction, userMsg('a', 'x', '继续')], new Set())
    expect(turns[0].role).toBe('engine-mark')
    expect(turns[0].blocks[0].text).toContain('5000')
  })

  it('工作区编码与 pi 会话目录规则一致', () => {
    expect(encodedWorkspace('D:\\GitFiles\\WorkBaby')).toBe('--D--GitFiles-WorkBaby--')
    expect(encodedWorkspace('/home/user/proj')).toBe('--home-user-proj--')
  })
})
