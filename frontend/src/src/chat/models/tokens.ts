/**
 * 输入框草稿与用户消息里的引用令牌解析。
 *
 * <p>输入框镜像高亮与用户气泡 token 共用同一套扫描规则，避免「输入时高亮的」
 * 与「发出去显示的」不一致；`@名称` 的匹配口径与后端 Skill 触发词同源（最长名优先）。
 */
/** 令牌类别：skill / file 由调用方解析；cmd 为斜杠命令；unk 为未命中的 `@xxx`。 */
export type TokenKind = 'skill' | 'file' | 'cmd' | 'unk' | 'mention'

/** 分段结果：kind 为空串表示普通文本。 */
export interface DraftToken {
  text: string
  kind: TokenKind | ''
}

/** `@引用` 与 `/命令`；名称允许中英文、数字、下划线、点与连字符。 */
const TOKEN_RE = /@[A-Za-z0-9\u4e00-\u9fa5][A-Za-z0-9\u4e00-\u9fa5_.-]*|\/[A-Za-z]+/g

/** 触发字符左侧允许的前导符：行首 / 空白 / 常见中英文标点（避免把 a/b 路径当命令）。 */
const BOUNDARY = /[\s\u3000(（【［{「『"'、，。；：！？,.;:!?]/

function isBoundary(prev: string | undefined): boolean {
  return prev === undefined || BOUNDARY.test(prev)
}

/**
 * 文本 → 分段序列。
 *
 * @param resolve 把 `@名称` 解析为 skill / file；返回 null 视为未命中（unk）。
 *                省略时 `@名称` 一律标记 mention（历史消息气泡场景，无解析上下文）。
 */
export function splitTokens(text: string, resolve?: (name: string) => 'skill' | 'file' | null): DraftToken[] {
  const out: DraftToken[] = []
  let last = 0
  TOKEN_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TOKEN_RE.exec(text)) !== null) {
    const tok = m[0]
    if (tok.startsWith('/') && !isBoundary(text[m.index - 1])) continue
    let kind: TokenKind | '' = ''
    if (tok.startsWith('@')) {
      const name = tok.slice(1)
      if (!resolve) kind = 'mention'
      else {
        const hit = resolve(name)
        kind = hit === 'skill' ? 'skill' : hit === 'file' ? 'file' : 'unk'
      }
    } else {
      kind = 'cmd'
    }
    if (m.index > last) out.push({ text: text.slice(last, m.index), kind: '' })
    out.push({ text: tok, kind })
    last = m.index + tok.length
  }
  if (last < text.length) out.push({ text: text.slice(last), kind: '' })
  // 尾部补一个换行：镜像层与 textarea 的末尾空行高度对齐（pre-wrap 下末行换行不撑高）
  out.push({ text: '\n', kind: '' })
  return out
}

/** 历史用户消息里的 `@引用` 分段（统一 mention 样式，不做命中判定）。 */
export function splitMentions(text: string): DraftToken[] {
  return splitTokens(text)
}
