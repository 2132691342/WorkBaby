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
