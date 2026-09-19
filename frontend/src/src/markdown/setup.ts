import type MarkdownIt from 'markdown-it'
import type StateBlock from 'markdown-it/lib/rules_block/state_block.mjs'
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs'
import taskLists from 'markdown-it-task-lists'
import footnote from 'markdown-it-footnote'

/**
 * Markdown 扩展装配：任务列表 / 脚注为同步插件直接生效；
 * 数学公式与 Mermaid 只吐带 data-* 的占位节点，由渲染器异步按需加载填充（依赖 >1MB，不进首屏）。
 */

/** 需要异步增强的占位节点 class。 */
export const MATH_CLASS = 'wb-math'
export const MERMAID_CLASS = 'wb-mermaid'

/**
 * 转义为元素<b>文本内容</b>（不是属性值）。
 *
 * <p>刻意不用 {@code data-*} 属性承载公式 / 图源码：DOMPurify 会吞掉值里含换行的
 * {@code data-code}（实测），而且多行内容塞属性本身也脆弱。放文本节点最稳，
 * 且行内公式也能直接显示源码作为「未增强时的降级内容」。
 */
function escapeText(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

/** 行内公式 {@code $E=mc^2$}（跳过 {@code $$} 交由块级规则处理）。 */
function mathInline(state: StateInline, silent: boolean): boolean {
  const start = state.pos
  if (state.src.charCodeAt(start) !== 0x24 /* $ */) return false
  if (state.src.charCodeAt(start + 1) === 0x24) return false

  let found = -1
  let pos = start + 1
  while (pos < state.posMax) {
    const ch = state.src.charCodeAt(pos)
    if (ch === 0x5c /* \ */) {
      pos += 2
      continue
    }
    if (ch === 0x0a /* \n */) return false
    if (ch === 0x24 /* $ */) {
      found = pos
      break
    }
    pos += 1
  }
  // 未闭合或空内容：不识别为公式（避免把「价格 $5 和 $8」误判成公式）
  if (found < 0 || found === start + 1) return false

  if (!silent) {
    const token = state.push('math_inline', 'span', 0)
    token.content = state.src.slice(start + 1, found)
    token.markup = '$'
  }
  state.pos = found + 1
  return true
}

/** 块级公式：支持单行 {@code $$a^2+b^2=c^2$$} 与多行 {@code $$ ⏎ ... ⏎ $$} 两种写法。 */
function mathBlock(state: StateBlock, startLine: number, endLine: number, silent: boolean): boolean {
  const start = state.bMarks[startLine] + state.tShift[startLine]
  const lineEnd = state.eMarks[startLine]
  if (state.src.slice(start, start + 2) !== '$$') return false

  const firstLine = state.src.slice(start, lineEnd)
  const trimmed = firstLine.trimEnd()

  // 单行写法：$$ ... $$（内容非空，且首尾各占两个 $）
  if (trimmed.length > 4 && trimmed.endsWith('$$')) {
    if (silent) return true
    const token = state.push('math_block', 'div', 0)
    token.content = trimmed.slice(2, -2)
    token.markup = '$$'
    state.line = startLine + 1
    return true
  }

  // 多行写法：起始行只有 $$，向下找第一个以 $$ 收尾的结束行
  for (let line = startLine + 1; line <= endLine; line += 1) {
    const ls = state.bMarks[line] + state.tShift[line]
    const le = state.eMarks[line]
    if (state.src.slice(ls, le).trimEnd().endsWith('$$')) {
      if (silent) return true
      const token = state.push('math_block', 'div', 0)
      token.content = state.getLines(startLine + 1, line, 0, false)
      token.markup = '$$'
      state.line = line + 1
      return true
    }
  }
  return false
}

/**
 * 给 MarkdownIt 实例装上全部扩展。
 *
 * @param md 目标实例（流式实例与终态实例共用同一套规则，保证流式→终态不跳动）
 */
export function setupMarkdown(md: MarkdownIt): MarkdownIt {
  // ---- 同步插件（廉价）----
  md.use(taskLists, { enabled: true, label: true, labelAfter: true })
  md.use(footnote)

  // ---- 数学公式（占位，异步渲染）----
  md.inline.ruler.before('escape', 'math_inline', mathInline)
  md.block.ruler.before('fence', 'math_block', mathBlock, {
    alt: ['paragraph', 'reference', 'blockquote', 'list']
  })
  md.renderer.rules.math_inline = (tokens, idx) =>
    `<span class="${MATH_CLASS}" data-display="inline">${escapeText(tokens[idx].content)}</span>`
  md.renderer.rules.math_block = (tokens, idx) =>
    `<div class="${MATH_CLASS}" data-display="block">${escapeText(tokens[idx].content)}</div>`

  // ---- Mermaid（占位，异步渲染）----
  const defaultFence = md.renderer.rules.fence
  md.renderer.rules.fence = (tokens, idx, options, env, self) => {
    const token = tokens[idx]
    const lang = (token.info ?? '').trim().split(/\s+/)[0]?.toLowerCase()
    if (lang === 'mermaid') {
      return `<pre class="${MERMAID_CLASS}"><code>${escapeText(token.content)}</code></pre>`
    }
    return defaultFence
      ? defaultFence(tokens, idx, options, env, self)
      : self.renderToken(tokens, idx, options)
  }
  return md
}
