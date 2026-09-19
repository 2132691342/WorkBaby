<script setup lang="ts">
/**
 * Markdown 渲染器：markdown-it 单例（流式关高亮 + 节流；终态开高亮 + linkify），
 * 输出经 DOMPurify 消毒（`html:false` + 白名单双保险）。扩展：GFM 任务列表 / 脚注 /
 * 数学公式 / Mermaid（按需异步加载）。`#md-cursor-slot` 供外部光标挂载。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js'
import DOMPurify from 'dompurify'
// 不再引亮色固定的 hljs 官方主题（暗色下代码块白底刺眼），
// 改用下方自写的 wb 主题跟随 token 配色（亮暗两套，走 CSS 变量切换）
import { useThrottledContent } from '@/composables/useThrottledContent'
import { setupMarkdown } from '@/markdown/setup'
import { enhanceMarkdown, decorateCodeBlocks, codeTextForCopy } from '@/markdown/enhance'
import { stripThinkBlocks } from '@/chat/models/blocks'
import { openExternal } from '@/api/shellBridge'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'

const props = defineProps<{
  /** Markdown 源文本。 */
  content: string
  /** 是否处于流式状态（流式时关闭高亮 + 节流 100ms）。 */
  streaming?: boolean
  /** 最小高度（防 layout shift），默认 2.5rem。 */
  minHeightRem?: number
}>()

// ===== 模块顶层单例 MarkdownIt =====
// 流式模式（streaming=true）：关闭 hljs + linkify + breaks，避免每 token 做 URL 识别 / 自动换行
//                          （这两项是 markdown-it 流式场景下第二大耗时来源，第一是 hljs）
// 静态模式（streaming=false）：开启 hljs + linkify，做最终高亮与 URL 转换
// 两个实例装同一套扩展规则（setupMarkdown），保证流式 → 终态的 DOM 结构一致、不跳动
const mdStreaming = setupMarkdown(new MarkdownIt({
  html: false,
  linkify: false,  // 流式关闭 linkify：URL 识别延迟到终态，省掉每帧的链接扫描
  breaks: true     // 流式与终态保持一致：换行渲染口径不同会导致流结束时整段重排
  // streaming 模式不传 highlight，对未闭合代码块也按纯文本处理（性能优先；高亮延迟到终态）
}))

const mdFinal = setupMarkdown(new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
  highlight(code: string, lang: string): string {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value
      } catch {
        /* ignore */
      }
    }
    return ''
  }
}))

// ===== 流式节流 =====
const streamingRef = computed(() => props.streaming === true)
// 渲染前剥离 <think> 块：兼容端点把推理写进正文（原生 thinking 走独立通道）
const contentRef = computed(() => stripThinkBlocks(props.content))

// streamSafeMarkdown 流式期间的最小修复：未闭合的 code fence（最后一个 ``` 奇数个）
// 自动补一个闭合 ```——避免流到一半时 markdown-it 把"```python"当成正常段落渲染，
// 等下一字符落库再回炉重渲造成 layout shift / 闪烁。终态模式不做（避免污染落库内容）。
//
// 只影响渲染管道；contentRef 的源仍是 props.content（流结束时 rAF 重渲到终态视图）。
function streamSafeMarkdown(text: string): string {
  if (!text) return text
  // 数行内 ``` 出现次数；奇数说明少了一个闭合。
  let count = 0
  let idx = 0
  while ((idx = text.indexOf('```', idx)) !== -1) {
    count++
    idx += 3
  }
  if (count % 2 === 1) return text + '\n```'
  return text
}

/**
 * 节流间隔随正文长度自适应：流式每次刷新都是「全量重新解析 + 全量替换 DOM」，
 * 成本随长度线性上升；长答案放慢刷新，视觉上反而更跟手（不再一卡一卡地跳字）。
 */
const streamInterval = computed(() => {
  const n = contentRef.value.length
  if (n < 2000) return 70
  if (n < 8000) return 130
  return 200
})
const throttledContent = useThrottledContent(contentRef, streamingRef, streamInterval)
// 流式期才做"未闭合 code fence 自动补齐"修复；终态视图直接渲染源（落库内容不被修改）。
const renderableContent = computed(() =>
  streamingRef.value ? streamSafeMarkdown(throttledContent.value) : throttledContent.value,
)

// ===== 渲染 =====
// 优化：流式时 DOMPurify 简化钩子（跳过一些检查以加速 sanitize）
const renderedHtml = ref('')
/** 渲染是否曾失败并降级为纯文本（失败时展示提示条，内容本身仍完整可见）。 */
const renderFailed = ref(false)

/** HTML 转义表（降级渲染用，禁止把原始内容直接拼进 v-html）。 */
const HTML_ESCAPE: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;'
}

/** 把内容转义为纯文本 HTML（渲染失败时的最后兜底）。 */
function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (c) => HTML_ESCAPE[c] ?? c)
}

/**
 * 剥离模型吐进正文的 {@code <think>} 标签（兜底）。
 * 正常路径 thinking 由后端分离（thinking 事件 + 落库 thinking 列），
 * 但部分模型会把 think 标签混进 content —— 已闭合的直接删，未闭合（流式中）删到结尾。
 */
function stripThinkTags(s: string): string {
  const text = String(s ?? '')
  return text
    .replace(/<think>[\s\S]*?<\/think>/gi, '')
    .replace(/<think>[\s\S]*$/i, '')
}

function renderNow(content: string, isStreaming: boolean): void {
  // content 可能为 null/undefined（历史消息空正文 / 局部构造消息），统一空串兜底避免 strip 阶段抛错
  const text = String(content ?? '')
  try {
    const source = stripThinkTags(text)
    const raw = isStreaming ? mdStreaming.render(source) : mdFinal.render(source)
    // 流式跳过 DOMPurify：两个 markdown-it 实例都是 html:false，源文本里的 HTML 一律被转义，
    // 渲染结果不可能带可执行标记；sanitize 的耗时随长度线性上升，是长答案卡顿的主因之一。
    // 终态（用户会留存/复制的内容）仍然完整 sanitize，双保险不丢。
    renderedHtml.value = isStreaming ? raw : DOMPurify.sanitize(raw)
    renderFailed.value = false
  } catch (e) {
    // 畸形 markdown（超深嵌套 / 未闭合标签 / 异常 HTML 混排）会让 markdown-it 或 DOMPurify 抛错，
    // 未捕获时 Vue watcher 中断 → 整条消息空白。这里降级为转义纯文本，保证内容不可见性丢失。
    renderFailed.value = true
    renderedHtml.value = `<pre class="md-plain-fallback">${escapeHtml(text)}</pre>`
    // eslint-disable-next-line no-console
    console.warn('[markdown] render failed, fallback to plain text', e)
  }
}

/** 内容容器（异步增强 katex / mermaid 需要真实 DOM 节点）。 */
const bodyRef = ref<HTMLElement | null>(null)
const toast = useToast()

/**
 * 异步增强数学公式与流程图 + 代码块装饰。
 * 流式过程中不跑（每 100ms 重渲染一次，此时跑异步渲染既浪费又会产生闪烁），
 * 只在流式结束后的终态触发。
 */
async function enhance(): Promise<void> {
  await nextTick()
  try {
    await enhanceMarkdown(bodyRef.value)
    decorateCodeBlocks(bodyRef.value as HTMLElement)
  } catch (e) {
    // 增强失败只影响公式/图，正文已经渲染完成，绝不能因此清空消息
    // eslint-disable-next-line no-console
    console.warn('[markdown] enhance failed', e)
  }
}

/**
 * 容器内点击的一次性事件委托（v-html 只换 innerHTML，容器监听器始终存活）。
 * 链接一律 preventDefault 后交系统浏览器打开（WebView 内导航会替换整个 SPA）；
 * 代码块复制按钮需阻断所在 details>summary 的折叠切换。
 */
function onBodyClick(e: MouseEvent): void {
  const target = e.target instanceof Element ? e.target : null
  const link = target?.closest('a[href]')
  if (link) {
    e.preventDefault()
    e.stopPropagation()
    void openExternal(link.getAttribute('href') ?? '')
    return
  }
  const text = codeTextForCopy(e.target)
  if (text === null) return
  e.preventDefault()
  e.stopPropagation()
  void navigator.clipboard
    ?.writeText(text)
    .then(() => toast.success(t('chat.copied')))
    .catch(() => toast.error(t('common.saveFailed')))
}

onMounted(() => {
  bodyRef.value?.addEventListener('click', onBodyClick)
})

// 节流后内容变化时重新渲染
// 必须 immediate —— 静态内容（终态消息 / 内置文档）挂载时
// throttled 已是最终值且不再变化，非 immediate 的 watch 永不触发，
// renderedHtml 永远是空串 → 「流式可见、结束后整条消息空白」的根因。
watch(
  renderableContent,
  (v) => {
    renderNow(v, streamingRef.value)
    if (!streamingRef.value) void enhance()
  },
  { immediate: true }
)

// streaming 关闭瞬间（流式 → 非流式），重新触发渲染并做异步增强
watch(streamingRef, (_isStreaming) => {
  renderNow(props.content, streamingRef.value)
  if (!streamingRef.value) void enhance()
})

onBeforeUnmount(() => {
  bodyRef.value?.removeEventListener('click', onBodyClick)
  // 组件卸载后不再往已销毁的 DOM 里塞内容
  bodyRef.value = null
})
</script>

<template>
  <div
    class="markdown-body-wrapper"
    :style="{ minHeight: `${minHeightRem ?? 2.5}rem` }"
  >
    <!-- 兜底提示：渲染异常时明确告知已降级，避免用户误以为内容丢失 -->
    <div v-if="renderFailed" class="md-fallback-tip">{{ t('chat.renderFallback') }}</div>
    <!-- 内容容器；外部光标挂到 #md-cursor-slot，不随 v-html 重写销毁 -->
    <div ref="bodyRef" class="markdown-body" v-html="renderedHtml" />
    <!-- 外部光标挂载点：调用方可往 #md-cursor-slot 注入独立 DOM 节点，节点不会随 v-html 重写而销毁 -->
    <span id="md-cursor-slot" class="md-cursor-slot" />
  </div>
</template>

<style scoped>
/* 只保留「本组件真实拥有的节点」样式：v-html 注入的 DOM 不带 data-v-*，
 * 写在这里的 .markdown-body 子孙规则全都不会命中（曾经就有一整块是死规则，
 * 唯一效果是让人以为改了那里能生效）。正文排版见 style.css，代码块见下方全局块。 */
.markdown-body-wrapper {
  contain: layout style;
}
.markdown-cursor-slot {
  display: inline;
  vertical-align: baseline;
}
.markdown-body {
  line-height: 1.6;
  word-wrap: break-word;
}
/* 兜底提示条：低视觉权重，不抢正文注意力 */
.md-fallback-tip {
  font-size: 0.75rem;
  color: var(--wb-warning, #b45309);
  background: color-mix(in srgb, var(--wb-warning, #b45309) 12%, transparent);
  border-radius: 6px;
  padding: 2px 8px;
  margin-bottom: 4px;
  width: fit-content;
}
/* 纯文本降级：保留换行与等宽，内容完整可见 */
.md-plain-fallback {
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: 0.85rem;
  margin: 4px 0;
}
</style>

<style>
/* ===== 代码块：全局样式（v-html 注入的 DOM 不带 scoped 属性，scoped 规则不可靠）=====
 * 配色跟随主题：token 全走 --wb-c-* 变量，两套主题只切变量值，token 规则一份。
 * 取色基准：GitHub Light / GitHub Dark 官方 hljs 主题。 */
:root[data-theme='light'] {
  --wb-code-bg: #f7f9fc;
  --wb-code-header-bg: #eef2f8;
  --wb-code-fg: #24292f;
  --wb-code-border: rgba(15, 23, 42, 0.1);
  --wb-c-keyword: #cf222e;
  --wb-c-string: #0a3069;
  --wb-c-number: #0550ae;
  --wb-c-comment: #6e7781;
  --wb-c-func: #8250df;
  --wb-c-meta: #0550ae;
  --wb-c-tag: #116329;
  --wb-c-built: #e16f24;
}
:root[data-theme='dark'] {
  --wb-code-bg: #12141a;
  --wb-code-header-bg: rgba(148, 163, 184, 0.1);
  --wb-code-fg: #e6edf3;
  --wb-code-border: rgba(148, 163, 184, 0.2);
  --wb-c-keyword: #ff7b72;
  --wb-c-string: #a5d6ff;
  --wb-c-number: #79c0ff;
  --wb-c-comment: #8b949e;
  --wb-c-func: #d2a8ff;
  --wb-c-meta: #79c0ff;
  --wb-c-tag: #7ee787;
  --wb-c-built: #ffa657;
}

.markdown-body pre {
  position: relative;
  background: var(--wb-code-bg);
  color: var(--wb-code-fg);
  border: 1px solid var(--wb-code-border);
  border-radius: 12px;
  padding: 12px 14px;
  margin: 0.7em 0;
  /* 长代码限高滚动而不是折叠：折叠会让用户以为内容丢了，滚动则始终可见。
   * 限高收到 24rem——过高的代码块会把消息流切成两段，正文被推到屏外。 */
  max-height: 24rem;
  overflow: auto;
  overscroll-behavior: contain;
}
/* header 覆盖在 pre 顶部：语言标签 + 复制按钮。
 * 负外边距抵消 pre 的内边距，让 header 顶满整条；数值必须与 pre 的 padding 同步。 */
.markdown-body .wb-code-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  margin: -12px -14px 10px;
  background: var(--wb-code-header-bg);
  border-bottom: 1px solid var(--wb-code-border);
  font-size: 11px;
  color: var(--wb-muted, #64748b);
}
.markdown-body .wb-code-lang {
  font-family: var(--font-mono);
  text-transform: lowercase;
  letter-spacing: 0.04em;
}
.markdown-body .wb-code-copy {
  border: none;
  background: transparent;
  color: var(--wb-muted, #64748b);
  font-size: 11px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 6px;
}
.markdown-body .wb-code-copy:hover {
  color: var(--wb-primary);
  background: color-mix(in srgb, var(--wb-primary) 12%, transparent);
}
/* 长代码折叠（原生 details） */
.markdown-body .wb-code-fold {
  margin: 8px 0;
  border-radius: 12px;
  border: 1px solid var(--wb-code-border);
  background: var(--wb-code-bg);
}
.markdown-body .wb-code-fold > pre {
  margin: 0;
  border: none;
  border-radius: 0 0 12px 12px;
}
.markdown-body .wb-code-summary {
  cursor: pointer;
  list-style: none;
  color: var(--wb-muted, #64748b);
  font-size: 11px;
  user-select: none;
}
.markdown-body .wb-code-summary::-webkit-details-marker {
  display: none;
}
.markdown-body .wb-code-summary::before {
  content: '▸ ';
}
.markdown-body .wb-code-fold[open] .wb-code-summary::before {
  content: '▾ ';
}
.markdown-body .wb-code-fold[open] .wb-code-summary {
  padding-bottom: 4px;
}
/* summary 内的 header（折叠条上的语言 + 复制） */
.markdown-body .wb-code-summary .wb-code-header {
  margin: 0;
  padding: 6px 10px;
  background: transparent;
  border-bottom: none;
}

/* ===== GFM 任务列表 ===== */
.markdown-body ul.contains-task-list {
  list-style: none;
  padding-left: 4px;
}
.markdown-body li.task-list-item {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.markdown-body li.task-list-item input[type='checkbox'] {
  margin: 0;
  accent-color: var(--wb-primary);
  pointer-events: none;
}

/* ===== 脚注 ===== */
.markdown-body .footnotes {
  margin-top: 12px;
  padding-top: 8px;
  border-top: 1px solid var(--wb-border);
  font-size: 0.85em;
  color: var(--wb-muted);
}
.markdown-body .footnote-ref a,
.markdown-body .footnote-backref {
  color: var(--wb-primary-strong);
  text-decoration: none;
}

/* ===== 数学公式 / Mermaid（占位 → 异步填充）===== */
.markdown-body .wb-math {
  overflow-x: auto;
  overflow-y: hidden;
  max-width: 100%;
}
.markdown-body .wb-math[data-display='block'] {
  margin: 0.8em 0;
  text-align: center;
}
.markdown-body .wb-mermaid-svg {
  margin: 0.8em 0;
  overflow-x: auto;
  text-align: center;
}
.markdown-body .wb-mermaid-error,
.markdown-body .wb-render-error {
  background: color-mix(in srgb, var(--wb-warning) 10%, transparent);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 0.85em;
  white-space: pre-wrap;
}

/* hljs token → 主题变量 */
.markdown-body .hljs {
  color: var(--wb-code-fg);
  background: transparent;
}
.markdown-body .hljs-keyword,
.markdown-body .hljs-selector-tag,
.markdown-body .hljs-doctag,
.markdown-body .hljs-name {
  color: var(--wb-c-keyword);
}
.markdown-body .hljs-string,
.markdown-body .hljs-regexp,
.markdown-body .hljs-addition {
  color: var(--wb-c-string);
}
.markdown-body .hljs-number,
.markdown-body .hljs-literal,
.markdown-body .hljs-selector-attr,
.markdown-body .hljs-selector-pseudo {
  color: var(--wb-c-number);
}
.markdown-body .hljs-comment,
.markdown-body .hljs-quote,
.markdown-body .hljs-deletion {
  color: var(--wb-c-comment);
  font-style: italic;
}
.markdown-body .hljs-title,
.markdown-body .hljs-title.class_,
.markdown-body .hljs-title.function_,
.markdown-body .hljs-section {
  color: var(--wb-c-func);
}
.markdown-body .hljs-meta,
.markdown-body .hljs-symbol,
.markdown-body .hljs-bullet,
.markdown-body .hljs-link,
.markdown-body .hljs-variable,
.markdown-body .hljs-template-variable,
.markdown-body .hljs-attr,
.markdown-body .hljs-attribute {
  color: var(--wb-c-meta);
}
.markdown-body .hljs-tag,
.markdown-body .hljs-type {
  color: var(--wb-c-tag);
}
.markdown-body .hljs-built_in,
.markdown-body .hljs-builtin-name,
.markdown-body .hljs-params {
  color: var(--wb-c-built);
}
.markdown-body .hljs-strong {
  font-weight: 600;
}
.markdown-body .hljs-emphasis {
  font-style: italic;
}
</style>
