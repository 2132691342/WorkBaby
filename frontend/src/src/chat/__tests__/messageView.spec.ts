import { describe, expect, it } from 'vitest'
import { mergeLoadedMessages, toUiMessages, resolveRunAssistant } from '../models/merge'
import { groupToolRuns } from '../models/blocks'
import { expandCommandPrompt, expandSlashDraft, tokenizeArgs } from '../commandArgs'
import type { Message } from '@/types/api'

/**
 * 消息视图层测试：本地乐观消息与权威快照的对账、消息块分组、斜杠命令参数展开。
 * 每条断言都对应一类真实事故：重影、重复堆叠、已删消息残留、断线丢增量、分组乱序、参数被二次展开。
 */
function msg(over: Partial<Message> & { id: string }): Message {
  return { role: 'user', content: '', session_id: 'S1', status: 'completed', ...over } as Message
}

// ===== 消息对账 =====

describe('mergeLoadedMessages', () => {
  it('按内容对账：本地乐观行被权威行替换，重复内容不堆叠', () => {
    const replaced = mergeLoadedMessages(
      [msg({ id: 'local-1', role: 'user', content: '你好' })],
      [
        msg({ id: 'M1', role: 'user', content: '你好' }),
        msg({ id: 'M2', role: 'assistant', content: '你好呀' })
      ]
    )
    expect(replaced.map((m) => m.id)).toEqual(['M1', 'M2'])

    const deduped = mergeLoadedMessages(
      [msg({ id: 'local-2', role: 'user', content: '查一下' })],
      [
        msg({ id: 'M1', role: 'user', content: '查一下' }),
        msg({ id: 'M2', role: 'assistant', content: '结果' })
      ]
    )
    expect(deduped.filter((m) => m.role === 'assistant')).toHaveLength(1)
    expect(deduped.filter((m) => m.role === 'user')).toHaveLength(1)
  })

  it('assistant 占位：空正文跳过（避免与流式气泡重影），已产出正文保留', () => {
    const skipped = mergeLoadedMessages(
      [msg({ id: 'local-1', role: 'user', content: '在吗' })],
      [
        msg({ id: 'M1', role: 'user', content: '在吗' }),
        msg({ id: 'M2', role: 'assistant', content: '', status: 'streaming' })
      ]
    )
    expect(skipped.map((m) => m.id)).toEqual(['M1'])

    const kept = mergeLoadedMessages([], [msg({ id: 'M3', role: 'assistant', content: '答案' })])
    expect(kept.map((m) => m.id)).toEqual(['M3'])
  })

  it('权威为准：无本地消息时直接采用快照；权威已删的本地行被丢弃', () => {
    const loaded = [msg({ id: 'M1', role: 'user', content: 'a' })]
    expect(mergeLoadedMessages([], loaded)).toEqual(loaded)

    const pruned = mergeLoadedMessages(
      [msg({ id: 'M1', role: 'user', content: 'a' }), msg({ id: 'M9', role: 'user', content: '已删' })],
      loaded
    )
    expect(pruned.map((m) => m.id)).toEqual(['M1'])
  })
})

describe('toUiMessages', () => {
  it('过滤 role=tool 行（仅供 LLM 上下文，过程由 TaskTimeline 回放）', () => {
    const loaded = [
      msg({ id: 'M1', role: 'user', content: '看下工作区有啥' }),
      msg({ id: 'M2', role: 'assistant', content: '回复' }),
      msg({ id: 'M3', role: 'tool', content: 'AI工具应用.md\nwails开发手册.md' })
    ]
    expect(toUiMessages(loaded).map((m) => m.id)).toEqual(['M1', 'M2'])
  })
})

/** 流式收尾时本地累积正文如何与权威消息归位：dedupe / fill / missing 三种情境。 */
describe('resolveRunAssistant', () => {
  const REPLY = '工作区根目录下的内容……'

  it('dedupe：三种情境都不重复 push（tool 收尾 / 断线丢增量 / steer 插话）', () => {
    expect(
      resolveRunAssistant(
        [
          msg({ id: 'M1', role: 'user', content: '看下工作区有啥' }),
          msg({ id: 'M2', role: 'assistant', content: REPLY }),
          msg({ id: 'M3', role: 'tool', content: 'AI工具应用.md' })
        ],
        REPLY
      )
    ).toBe('dedupe')

    expect(
      resolveRunAssistant(
        [
          msg({ id: 'M1', role: 'user', content: 'hi' }),
          msg({ id: 'M2', role: 'assistant', content: '权威完整内容' })
        ],
        '流式缺失前缀的部分内容'
      )
    ).toBe('dedupe')

    expect(
      resolveRunAssistant(
        [
          msg({ id: 'M1', role: 'user', content: '问题' }),
          msg({ id: 'M2', role: 'assistant', content: REPLY }),
          msg({ id: 'M3', role: 'user', content: '中途插话' })
        ],
        REPLY
      )
    ).toBe('dedupe')
  })

  it('fill：本 run assistant 为空占位时返回该消息原地填充', () => {
    const messages = [
      msg({ id: 'M1', role: 'user', content: 'hi' }),
      msg({ id: 'M2', role: 'assistant', content: '', status: 'streaming' })
    ]
    expect(resolveRunAssistant(messages, '答案')).toEqual({ kind: 'fill', message: messages[1] })
  })

  it('missing：权威快照里没有本 run 的 assistant（后端未落库）', () => {
    expect(resolveRunAssistant([msg({ id: 'M1', role: 'user', content: 'hi' })], '答案')).toBe('missing')
  })
})

// ===== 消息块分组 =====

/** 造块：只需 kind + seq（分组只依赖这两者）。 */
const b = (kind: string, seq: number): { kind: string; seq: number } => ({ kind, seq })

describe('groupToolRuns', () => {
  it('连续工具块合并为一组，正文块切断分组', () => {
    const groups = groupToolRuns([
      b('text', 0),
      b('tool_call', 1),
      b('tool_result', 2),
      b('tool_call', 3),
      b('text', 4),
      b('tool_call', 5)
    ])
    expect(groups.map((g) => g.tools.length)).toEqual([0, 3, 0, 1])
    expect(groups[0].one?.kind).toBe('text')
    expect(groups[1].one).toBeNull()
    expect(groups[2].one?.kind).toBe('text')
  })

  it('孤立的 tool_result 也归入工具组（不因其缺少 call 而单独成组）', () => {
    const groups = groupToolRuns([b('tool_call', 0), b('tool_result', 1), b('tool_result', 2)])
    expect(groups).toHaveLength(1)
    expect(groups[0].tools.map((t) => t.seq)).toEqual([0, 1, 2])
  })

  it('分组不重排、不丢块（展开后仍是原始顺序）', () => {
    const input = [
      b('skill', 0),
      b('thinking', 1),
      b('tool_call', 2),
      b('tool_result', 3),
      b('text', 4),
      b('artifact', 5),
      b('genui', 6)
    ]
    const flat = groupToolRuns(input).flatMap((g) => (g.tools.length > 0 ? g.tools : [g.one!]))
    expect(flat.map((x) => x.seq)).toEqual(input.map((x) => x.seq))
    expect(flat.map((x) => x.kind)).toEqual(input.map((x) => x.kind))
  })

  it('空输入返回空数组（无块消息不渲染任何分组）', () => {
    expect(groupToolRuns([])).toEqual([])
  })
})

// ===== 斜杠命令参数 =====

describe('commandArgs', () => {
  it('切分参数时保留引号内的空格', () => {
    expect(tokenizeArgs('src/app.ts "my module"')).toEqual(['src/app.ts', 'my module'])
    expect(tokenizeArgs("'a b' c")).toEqual(['a b', 'c'])
    expect(tokenizeArgs('   ')).toEqual([])
  })

  it('展开 $ARGUMENTS 与位置参数', () => {
    expect(expandCommandPrompt('审查 $1，范围 $2', 'auth.ts "src/core"')).toBe('审查 auth.ts，范围 src/core')
    expect(expandCommandPrompt('全部参数：$ARGUMENTS', 'a b c')).toBe('全部参数：a b c')
    // 未提供参数：占位符注入空串，不把字面量 $ARGUMENTS 发给模型
    expect(expandCommandPrompt('审查 $1 / $ARGUMENTS', '')).toBe('审查  / ')
  })

  it('参数正文里的占位符不被二次展开', () => {
    // 先替换位置参数再替换 $ARGUMENTS，否则参数里的 $1 会被当成占位符吃掉
    expect(expandCommandPrompt('$ARGUMENTS', 'keep $1 literally')).toBe('keep $1 literally')
  })

  it('只对带模板的自定义命令做草稿展开', () => {
    const commands = [
      { name: 'review', prompt: '审查 $ARGUMENTS' },
      { name: 'compact' },
      { name: 'noop', prompt: '' }
    ]
    expect(expandSlashDraft('/review 登录模块', commands)).toBe('审查 登录模块')
    expect(expandSlashDraft('/compact', commands)).toBe('/compact')
    expect(expandSlashDraft('/noop x', commands)).toBe('/noop x')
    expect(expandSlashDraft('看看这个文件', commands)).toBe('看看这个文件')
    expect(expandSlashDraft('/unknown x', commands)).toBe('/unknown x')
  })
})
