// 一轮回答里「活动段」的分组：连续的思考块与工具卡收成一段，段首给一行摘要，正文按原顺序穿插。
// 分组只改「怎么显示」不改吐块顺序；正在流式的那段不折叠——折叠后用户判断不出它是否卡住。
// 渲染单元的 fold=false 分支即全平铺：每个块各自成段，顺序与内容都不变。
import type { ChatBlock } from '@/types/api'

/** 一段连续的活动（思考 + 工具），界面收成一行摘要。 */
export interface ActivityGroup {
  kind: 'activity'
  /** 段内全部块，按原顺序 */
  blocks: ChatBlock[]
  /** 稳定键：用段内第一个工具的 call id，没有就用起始下标 */
  key: string
}

/** 一段普通内容（正文），原样渲染。 */
export interface TextGroup {
  kind: 'text'
  block: ChatBlock
  /** 原下标：缓存键要与平铺时一致，否则切一次折叠就全量重解析 */
  index: number
}

/** 一轮的渲染单元：要么一段活动，要么一块正文。 */
export type RenderGroup = ActivityGroup | TextGroup

/** 工具名 → 摘要里的说法。认不出的原名照用。 */
const VERB: Record<string, string> = {
  read_file: '读文件',
  write_file: '写文件',
  edit_file: '改文件',
  list_files: '列目录',
  search: '搜索',
  grep: '搜索',
  glob: '找文件',
  bash: '跑命令',
  shell: '跑命令',
  web_fetch: '读网页',
  web_search: '搜网',
  todo_write: '记待办',
  todo_read: '看待办',
}

/** 一个工具在摘要里叫什么。 */
export function verbOf(toolName: string): string {
  const key = toolName.trim()
  return VERB[key] ?? (key || '工具')
}

/** 把一轮的块切成渲染单元。 */
export function groupBlocks(blocks: ChatBlock[], fold: boolean): RenderGroup[] {
  const out: RenderGroup[] = []
  let run: ChatBlock[] = []
  let runStart = -1

  const flush = () => {
    if (!run.length) return
    if (fold && run.length > 1) {
      out.push({ kind: 'activity', blocks: run, key: groupKey(run, runStart) })
    } else {
      // 不折叠或段里只有一块：逐块平铺，下标从段首算起，
      // 保证与「完全不分组」时的缓存键一致。
      run.forEach((block, i) => out.push({ kind: 'text', block, index: runStart + i }))
    }
    run = []
    runStart = -1
  }

  for (let i = 0; i < blocks.length; i++) {
    const block = blocks[i]
    // 增量补丁可能先落到靠后的下标，数组短暂留洞：跳过空位，那块到了再画，
    // 不让一个未到位的块把整轮渲染崩掉。
    if (!block) continue
    if (block.block_kind === 'text') {
      flush()
      out.push({ kind: 'text', block, index: i })
      continue
    }
    if (runStart < 0) runStart = i
    run.push(block)
  }
  flush()
  return out
}

/** 活动段的稳定键：优先用第一个工具的 call id，否则退回段首下标。 */
function groupKey(run: ChatBlock[], start: number): string {
  for (const block of run) {
    if (block.tool_call_id) return block.tool_call_id
  }
  return `a-${start}`
}

/** 摘要里的一句话：「读了 3 个文件 · 搜了 2 次」。同一种动作合并计数。 */
export function summarise(blocks: ChatBlock[]): { verb: string; count: number }[] {
  const order: string[] = []
  const counts = new Map<string, number>()
  for (const block of blocks) {
    if (block.block_kind !== 'tool') continue
    const verb = verbOf(block.tool_name ?? '')
    if (!counts.has(verb)) order.push(verb)
    counts.set(verb, (counts.get(verb) ?? 0) + 1)
  }
  return order.map((verb) => ({ verb, count: counts.get(verb) ?? 0 }))
}

/** 段内有没有跑砸的：有就整体标红，用户不必逐个卡片找。 */
export function hasError(blocks: ChatBlock[]): boolean {
  return blocks.some((b) => b.tool_status === 'error' || b.tool_is_error)
}

/** 段内是不是还在跑：有就整体转圈。 */
export function isRunning(blocks: ChatBlock[]): boolean {
  return blocks.some((b) => b.tool_status === 'running' || b.tool_status === 'pending')
}

/** 段内工具耗时合计，毫秒。0 表示没有这个事实。 */
export function totalMs(blocks: ChatBlock[]): number {
  let sum = 0
  for (const block of blocks) {
    if (block.duration_ms && block.duration_ms > 0) sum += block.duration_ms
  }
  return sum
}
