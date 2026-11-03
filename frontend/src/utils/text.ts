// 文本清洗：内核与扩展按终端习惯给文字上色（ESC[2m … ESC[22m），GUI 没有终端语义。
// 后端已剥，这里再兜一层——缓存里的旧响应与假后端都可能带着转义序列。
const ESC = String.fromCharCode(27)
const BEL = String.fromCharCode(7)
const ANSI = new RegExp(
  ESC + '\\[[0-9;]*[A-Za-z]|' + ESC + '\\][^' + BEL + ESC + ']*(?:' + BEL + '|' + ESC + '\\\\)?',
  'g',
)

export function stripAnsi(s: string): string {
  if (!s) return ''
  return s.replace(ANSI, '')
}

/** 过程输出的保留上限：它是「看它在干什么」，不是最终结果，攒满几百 KB 会让翻页都卡。 */
export const PROGRESS_TAIL_CHARS = 8000

/**
 * 追加一段过程输出并只保留尾部。
 * 收敛成一个函数：主对话与子代理两处都要这条规则，各写一遍必然漂移。
 */
export function appendTail(prev: string, chunk: string, limit = PROGRESS_TAIL_CHARS): string {
  const next = prev + chunk
  return next.length > limit ? next.slice(-limit) : next
}
