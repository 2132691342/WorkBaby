import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import { Marked } from 'marked'

// 统一 Markdown 渲染出口：转义 + 高亮 + 收口样式。
const md = new Marked({
  gfm: true,
  breaks: true,
})

md.use({
  renderer: {
    code(token) {
      const lang = (token.lang || '').split(/\s+/)[0]
      let body = token.text
      if (lang && hljs.getLanguage(lang)) {
        try {
          body = hljs.highlight(token.text, { language: lang }).value
        } catch {
          /* 高亮失败退回原文 */
        }
      } else {
        body = escapeHtml(token.text)
      }
      return `<pre class="code"><code>${body}</code></pre>`
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

export function firstLine(s: string, max = 120): string {
  const line = (s || '').trim().split('\n')[0] || ''
  return line.length > max ? line.slice(0, max) + '…' : line
}

export function summarizeArgs(args: Record<string, unknown> | undefined): string {
  if (!args) return ''
  const parts: string[] = []
  for (const [k, v] of Object.entries(args)) {
    const text = typeof v === 'string' ? v : JSON.stringify(v)
    parts.push(`${k}=${firstLine(text || '', 48)}`)
  }
  return parts.join('  ')
}
