// Markdown 渲染的唯一出口：marked + sanitize + 代码高亮都收在这里。
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import { Marked } from 'marked'

const md = new Marked({
  gfm: true,
  breaks: true,
})

// streaming 期间跳过语法高亮：每 120ms 全量重解析代码块没有意义，收尾那次补上。
let streaming = false

md.use({
  renderer: {
    code(token) {
      const lang = (token.lang || '').split(/\s+/)[0]
      let body: string
      if (!streaming && lang && hljs.getLanguage(lang)) {
        try {
          body = hljs.highlight(token.text, { language: lang }).value
        } catch {
          body = escapeHtml(token.text)
        }
      } else {
        body = escapeHtml(token.text)
      }
      const label = lang ? escapeHtml(lang) : 'text'
      const icon =
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" width="13" height="13"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a1 2 0 0 1-1-1V4a2 2 0 0 1 1-1h10a2 2 0 0 1 1 1v1"/></svg>'
      return (
        `<div class="code-block"><div class="code-head"><span class="code-lang">${label}</span>` +
        `<button class="code-copy" type="button" aria-label="copy">${icon}</button></div>` +
        `<pre class="code-body"><code class="hljs">${body}</code></pre></div>`
      )
    },
  },
})

function escapeHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function renderMarkdown(src: string): string {
  const raw = md.parse(src || '', { async: false }) as string
  return DOMPurify.sanitize(raw, { ADD_ATTR: ['target'] })
}

export function renderMarkdownStream(src: string): string {
  streaming = true
  try {
    return renderMarkdown(src)
  } finally {
    streaming = false
  }
}

function firstLine(s: string, max = 120): string {
  const line = (s || '').trim().split('\n')[0] || ''
  return line.length > max ? line.slice(0, max) + '…' : line
}

/** 工具卡的一行参数摘要：键=值，每段截断到 48 字 */
export function summarizeArgs(args: Record<string, unknown> | undefined): string {
  if (!args) return ''
  const parts: string[] = []
  for (const [k, v] of Object.entries(args)) {
    const text = typeof v === 'string' ? v : JSON.stringify(v)
    parts.push(`${k}=${firstLine(text || '', 48)}`)
  }
  return parts.join('  ')
}
