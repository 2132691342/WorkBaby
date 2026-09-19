import { MATH_CLASS, MERMAID_CLASS } from './setup'

/**
 * 代码块折叠阈值（行数）。
 *
 * <p>折叠是给「超长 dump」准备的逃生门，不是默认形态：阈值 25 时，模型一次回答里
 * 两段代码中稍长的那段就只剩一条折叠条，用户以为内容丢了（历史反馈）。常态代码
 * 用 pre 自身的 max-height + 滚动限高，内容始终可见。
 */
export const CODE_FOLD_LINES = 400

/**
 * 为代码块加「语言标签 + 复制 + 长代码折叠」。v-html 输出无法组件化，故在终态 enhance 阶段做
 * 幂等 DOM 装饰（data-wb-deco 防重，流式期不跑）；折叠用原生 details，复制走容器事件委托。
 */
export function decorateCodeBlocks(root: HTMLElement): void {
  const pres = root.querySelectorAll<HTMLElement>('pre > code')
  for (const code of pres) {
    const pre = code.parentElement as HTMLElement
    if (!pre || pre.hasAttribute('data-wb-done')) continue
    // 防重标记挂 pre 本身（长代码分支里 header 位于 details>summary 内，不在 pre 子树）
    pre.setAttribute('data-wb-done', '1')
    const lang = (Array.from(code.classList).find((c) => c.startsWith('language-')) ?? '')
      .replace('language-', '')
    const codeText = code.textContent ?? ''
    const lines = codeText.split('\n').length
    const long = lines > CODE_FOLD_LINES

    // header：语言标签 + 复制按钮
    const header = document.createElement('div')
    header.className = 'wb-code-header'
    header.setAttribute('data-wb-deco', '1')
    const langTag = document.createElement('span')
    langTag.className = 'wb-code-lang'
    langTag.textContent = lang || 'text'
    const copyBtn = document.createElement('button')
    copyBtn.type = 'button'
    copyBtn.className = 'wb-code-copy'
    copyBtn.setAttribute('data-wb-copy-code', '1')
    copyBtn.textContent = '复制'
    header.append(langTag, copyBtn)

    if (long) {
      // 长代码：原生 details 折叠（summary 挂在 header 上，点击展开）
      const details = document.createElement('details')
      details.className = 'wb-code-fold'
      const summary = document.createElement('summary')
      summary.className = 'wb-code-summary'
      summary.textContent = `${lang || 'text'} · ${lines} 行 · 点击展开`
      summary.setAttribute('data-wb-deco', '1')
      summary.append(header)
      details.append(summary, pre)
      pre.replaceWith(details)
    } else {
      pre.prepend(header)
    }
  }
}

/** 从容器读取被委托的复制按钮对应的代码文本（closest 向上找 pre）。 */
export function codeTextForCopy(target: EventTarget | null): string | null {
  const btn = target instanceof Element ? target.closest('[data-wb-copy-code]') : null
  if (!btn) return null
  const holder = btn.closest('details') ?? btn.closest('pre')
  const code = holder?.querySelector('code')
  return code ? (code.textContent ?? '') : null
}

/**
 * Markdown 异步增强把 {@link setup.ts} 吐出的占位节点渲染成真实内容。
 *
 * <p>katex / mermaid 合计 >1MB，这里用<b>动态 import</b> 隔离在首屏之外：
 * 只有页面上真的出现公式 / 流程图时，才会去加载对应 chunk 与其 CSS。
 *
 * <p><b>幂等</b>：渲染过的节点打 {@code data-rendered} 标记，重复调用不会重渲染。
 * <p><b>失败不致命</b>：单个公式/图渲染失败只在原位退化显示源码，不影响整条消息。
 */

type MermaidApi = typeof import('mermaid').default
type KatexApi = typeof import('katex').default

let mermaidPromise: Promise<MermaidApi> | null = null
let katexPromise: Promise<{ api: KatexApi }> | null = null

function loadKatex(): Promise<{ api: KatexApi }> {
  if (!katexPromise) {
    katexPromise = Promise.all([
      import('katex'),
      import('katex/dist/katex.min.css')
    ]).then(([mod]) => ({ api: mod.default }))
  }
  return katexPromise
}

function loadMermaid(): Promise<MermaidApi> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then((mod) => {
      const api = mod.default
      api.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: document.documentElement.classList.contains('dark') ? 'dark' : 'default'
      })
      return api
    })
  }
  return mermaidPromise
}

function markRendered(el: HTMLElement): void {
  el.setAttribute('data-rendered', '1')
}

/** 渲染容器内所有数学占位节点。 */
async function renderMath(root: HTMLElement): Promise<void> {
  const nodes = Array.from(root.querySelectorAll<HTMLElement>(`.${MATH_CLASS}:not([data-rendered])`))
  if (nodes.length === 0) return
  const { api: katex } = await loadKatex()
  for (const el of nodes) {
    const tex = el.textContent ?? ''
    const display = el.dataset.display === 'block'
    markRendered(el)
    try {
      el.innerHTML = katex.renderToString(tex, {
        displayMode: display,
        throwOnError: false,
        strict: false,
        output: 'html'
      })
    } catch (e) {
      el.textContent = tex
      el.classList.add('wb-render-error')
      // eslint-disable-next-line no-console
      console.warn('[markdown] katex render failed', e)
    }
  }
}

/** 渲染容器内所有 Mermaid 占位节点。 */
async function renderMermaid(root: HTMLElement): Promise<void> {
  const nodes = Array.from(root.querySelectorAll<HTMLElement>(`.${MERMAID_CLASS}:not([data-rendered])`))
  if (nodes.length === 0) return
  const mermaid = await loadMermaid()
  for (const el of nodes) {
    const code = el.textContent ?? ''
    markRendered(el)
    let id = `wb-mmd-${Math.random().toString(36).slice(2, 10)}`
    try {
      const { svg } = await mermaid.render(id, code)
      const holder = document.createElement('div')
      holder.className = 'wb-mermaid-svg'
      holder.innerHTML = svg
      el.replaceWith(holder)
    } catch (e) {
      // mermaid 渲染失败会往 body 残留一个错误图，先清掉再退化成代码块
      document.querySelectorAll(`[id^="${id}"]`).forEach((n) => n.remove())
      const fallback = document.createElement('pre')
      fallback.className = 'wb-mermaid-error'
      fallback.textContent = code
      el.replaceWith(fallback)
      // eslint-disable-next-line no-console
      console.warn('[markdown] mermaid render failed', e)
      id = ''
    }
  }
}

/**
 * 异步增强：按需加载 katex / mermaid 并渲染占位节点。
 *
 * @param root 内容容器（必须是已插入 DOM 的节点）
 */
export async function enhanceMarkdown(root: HTMLElement | null): Promise<void> {
  if (!root) return
  if (root.querySelector(`.${MATH_CLASS}:not([data-rendered])`)) {
    await renderMath(root)
  }
  if (root.querySelector(`.${MERMAID_CLASS}:not([data-rendered])`)) {
    await renderMermaid(root)
  }
}
