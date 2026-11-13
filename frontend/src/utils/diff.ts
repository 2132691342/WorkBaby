// 增删配对与词级差异：DiffView 的并排列与 DiffRows 的词级高亮共用一份。
// 配对规则「同一改动里的第 i 条删除配第 i 条新增」写两遍必有出入。
import type { DiffLine } from '@/types/api'

/** 一行拆出的片段；changed 的片段画词级高亮。 */
export interface Segment {
  text: string
  changed: boolean
}

/**
 * 把连续的删除段与新增段两两配对。
 * 上下文 / hunk / 文件行自己成对；段内多出来的删除或新增单独成对。
 */
export function pairRuns(lines: DiffLine[]): DiffLine[][] {
  const out: DiffLine[][] = []
  let dels: DiffLine[] = []
  let adds: DiffLine[] = []
  const flush = () => {
    const n = Math.max(dels.length, adds.length)
    for (let i = 0; i < n; i++) {
      const pair: DiffLine[] = []
      if (dels[i]) pair.push(dels[i])
      if (adds[i]) pair.push(adds[i])
      out.push(pair)
    }
    dels = []
    adds = []
  }
  for (const line of lines) {
    if (line.line_kind === 'del') {
      dels.push(line)
      continue
    }
    if (line.line_kind === 'add') {
      adds.push(line)
      continue
    }
    flush()
    out.push([line])
  }
  flush()
  return out
}

/**
 * 一行拆成「没变 / 变了」的片段（公共前后缀剥离，字符级）。
 * 词级 diff 的黄金标准是编辑距离，一屏几十行 × 每行几百字符不值那个代价；
 * 前后缀剥离抓住的恰好就是「改了哪几个字」这个用户想看的东西。
 */
export function wordSegments(text: string, partner: string | undefined): Segment[] {
  if (!text) return []
  if (partner === undefined) return [{ text, changed: false }]
  let start = 0
  const min = Math.min(text.length, partner.length)
  while (start < min && text[start] === partner[start]) start++
  let end = 0
  while (end < min - start && text[text.length - 1 - end] === partner[partner.length - 1 - end]) end++
  const out: Segment[] = [
    { text: text.slice(0, start), changed: false },
    { text: text.slice(start, text.length - end), changed: true },
    { text: text.slice(text.length - end), changed: false },
  ]
  return out.filter((seg) => seg.text.length > 0)
}
