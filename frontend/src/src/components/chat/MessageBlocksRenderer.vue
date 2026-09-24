<script setup lang="ts">
/**
 * 按事件到达顺序渲染 assistant 的完整过程块
 * （thinking / text / tool_call / tool_result / artifact / skill / genui）。
 * 同时接受历史稳定序列（ResolvedBlock[]）与流式暂存序列（StreamingBlock[]），
 * 内部统一转为 RenderBlock 后只写一遍渲染逻辑。
 */
import { computed, ref, watch } from 'vue'
import { Brain, Sparkles, Loader2, Square, Check, X, ChevronDown, Copy } from '@/components/common/icons'
import { useClipboard } from '@vueuse/core'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import { groupToolRuns, looksLikeDiff } from '@/chat/models/blocks'
import { toolLabel } from '@/chat/models/toolVisuals'
import { traceIcon, traceKind, traceTarget, traceDelegateAgent, countDiffLines } from '@/chat/models/toolTrace'
import type { ResolvedBlock } from '@/chat/models/blocks'
import type { StreamingBlock } from '@/chat/models/streamingBlocks'
import type { UiNode } from '@/components/genui/GenUiRenderer.vue'
import GenUiRenderer from '@/components/genui/GenUiRenderer.vue'
import MarkdownRenderer from '@/components/chat/MarkdownRenderer.vue'
import DiffView from '@/components/chat/DiffView.vue'
import KnowledgeHits, { type KnowledgeHit } from '@/components/chat/KnowledgeHits.vue'

const props = defineProps<{
  /** 历史块序列（来自 chat.models.blocks）。 */
  blocks?: ResolvedBlock[] | null
  /** 流式块序列（来自 chat.models.streamingBlocks）。 */
  streaming?: StreamingBlock[] | null
  /** 消息正文（message.content；持久化模型里 text 不入块，作为最后一条 text 块补齐渲染）。
   *  流式期此字段为空（streamingContent 在 StreamingBubble 单独渲染 +cursor 增强）。 */
  content?: string
  /** 是否流式中（用于决定『光标』/running 态显示）。 */
  streaming_mode?: boolean
  /** 流式 tick（running 工具的实时计时）。 */
  now?: number
  /** 失败工具可重试（仅历史末条 assistant 消息，MessageList 控制）。 */
  retryable?: boolean
}>()

const emit = defineEmits<{
  retry: [toolCallId: string]
}>()
void emit

/** 统一渲染块：两种输入归一为同一结构。 */
interface RenderBlock {
  kind: 'thinking' | 'text' | 'tool_call' | 'tool_result' | 'artifact' | 'skill' | 'genui'
  /** 块内顺序（用于 DOM key）。 */
  seq: number
  /** thinking / text 全文。 */
  text: string
  /** 结构化数据：text 块为 null；其他 kind 按需使用 */
  data: Record<string, unknown> | null
  /** tool_call 块。 */
  call?: { id: string; name: string; arguments: string }
  /** tool_result 块。 */
  result?: {
    toolCallId: string
    name: string
    content: string
    error?: string
    durationMs?: number
    refused?: boolean
    uiHint?: string
    data?: Record<string, unknown> | null
    /**
     * 工具元数据：cwd / same_failure_count / adaptive_hint / truncated_bytes 等。
     * 后端 chat:tool-result 载荷里的 meta map 直接透传。
     */
    meta?: Record<string, string>
  }
  /** skill 块。 */
  skill?: Record<string, unknown>
  /** artifact 块。 */
  artifact?: { name: string; data: unknown }
  /** genui 块。 */
  genui?: UiNode
  /** running 态（流式期）：tool_result 还未到 → 显示转圈。 */
  running?: boolean
}

const renderBlocks = computed<RenderBlock[]>(() => {
  const out: RenderBlock[] = []
  if (props.blocks && props.blocks.length > 0) {
    for (let i = 0; i < props.blocks.length; i++) {
      const b = props.blocks[i]
      const rb: RenderBlock = { kind: b.kind as RenderBlock['kind'], seq: i, text: b.text, data: null }
      if (b.kind === 'text' && b.data) {
        // 正文块：payload 是 {"text": "…"}，不是裸文本
        rb.text = String(b.data.text ?? '')
      } else if (b.kind === 'tool_call' && b.data) {
        rb.call = {
          id: String(b.data.id ?? ''),
          name: String(b.data.name ?? 'unknown'),
          arguments: String(b.data.arguments ?? '')
        }
      } else if (b.kind === 'tool_result' && b.data) {
        rb.result = {
          toolCallId: String(b.data.tool_call_id ?? ''),
          name: String(b.data.name ?? 'unknown'),
          content: String(b.data.content ?? ''),
          error: typeof b.data.error === 'string' ? b.data.error : undefined,
          durationMs: typeof b.data.duration_ms === 'number' ? b.data.duration_ms : undefined,
          refused: b.data.refused === true,
          uiHint: typeof b.data.ui_hint === 'string' ? b.data.ui_hint : undefined,
          data: (b.data.data ?? null) as Record<string, unknown> | null,
          // 后端 meta 透传：cwd / same_failure_count / adaptive_hint 等按需渲染
          meta: (b.data.meta && typeof b.data.meta === 'object')
            ? (b.data.meta as Record<string, string>)
            : undefined
        }
      } else if (b.kind === 'skill' && b.data) {
        rb.skill = b.data
      } else if (b.kind === 'artifact' && b.data) {
        rb.artifact = { name: String(b.data.name ?? 'artifact'), data: b.data.data }
      } else if (b.kind === 'genui' && b.data) {
        rb.genui = (b.data as { root?: UiNode }).root ?? (b.data as unknown as UiNode)
      }
      out.push(rb)
    }
  } else if (props.streaming && props.streaming.length > 0) {
    for (let i = 0; i < props.streaming.length; i++) {
      const b = props.streaming[i]
      const rb: RenderBlock = { kind: b.kind as RenderBlock['kind'], seq: i, text: b.text, data: null }
      if (b.kind === 'tool_call') {
        rb.call = {
          id: b.toolCallId ?? b.id,
          name: b.name ?? 'unknown',
          arguments: String(b.data?.arguments ?? '')
        }
      } else if (b.kind === 'tool_result') {
        rb.result = {
          toolCallId: b.toolCallId ?? '',
          name: b.name ?? 'unknown',
          content: b.text,
          error: typeof b.data?.error === 'string' ? b.data.error : undefined,
          durationMs: b.durationMs,
          refused: b.state === 'refused',
          data: (b.data?.data ?? null) as Record<string, unknown> | null,
          // 流式期透传 meta（与持久化路径同源）；mergeBlockUpdate 时已合并到 data
          meta: (b.data?.meta && typeof b.data.meta === 'object')
            ? (b.data.meta as Record<string, string>)
            : undefined
        }
        rb.running = b.state === 'running'
      } else if (b.kind === 'skill') {
        rb.skill = b.data ?? undefined
      } else if (b.kind === 'artifact') {
        rb.artifact = { name: String(b.data?.name ?? 'artifact'), data: b.data?.data }
      } else if (b.kind === 'genui') {
        rb.genui = (b.data as { root?: UiNode })?.root ?? (b.data as unknown as UiNode)
      }
      out.push(rb)
    }
  }
  // 兜底补正文：仅当块序列里**没有** text 块时才把 message.content 补到末尾。
  // 一旦已按真实位置落了正文块，再补一次就是重复内容，
  // 而且会把正文整体拖到过程之后——顺序就乱了（这是历史回看顺序错乱的根因）。
  const tail = (props.content ?? '').trim()
  if (tail && !out.some((b) => b.kind === 'text')) {
    const seq = out.length
    out.push({ kind: 'text', seq, text: tail, data: null })
  }
  return out
})

/**
 * 把 tool_call 与同 tool_call_id 的 tool_result 配对为单个工具单元（避免同一工具渲染两次）；
 * 无对应 call 的孤立 tool_result 退化为单独展示。
 */
const renderUnits = computed(() => {
  const blocks = renderBlocks.value
  // 第一遍：建 call_id → result 的索引
  const resultByCallID = new Map<string, RenderBlock>()
  for (const b of blocks) {
    if (b.kind === 'tool_result' && b.result) {
      resultByCallID.set(b.result.toolCallId, b)
    }
  }
  // 第二遍：合并 call 与 result；从结果列表剔除已配对的 tool_result
  const out: RenderBlock[] = []
  for (const b of blocks) {
    if (b.kind === 'tool_call' && b.call) {
      const r = resultByCallID.get(b.call.id)
      if (r) {
        out.push({
          ...b,
          result: r.result,
          running: r.running
        })
      } else {
        out.push(b)
      }
    } else if (b.kind === 'tool_result') {
      // 已配对 → 跳过；未配对（无对应 call）→ 单独展示
      if (!resultByCallID.get(b.result?.toolCallId ?? '') || !blocks.some((x) => x.kind === 'tool_call' && x.call?.id === b.result?.toolCallId)) {
        out.push(b)
      }
      // 注意：上面条件等价于『存在对应的 tool_call 且已被合并』则跳过；否则保留。
    } else {
      out.push(b)
    }
  }
  return out
})

/** 渲染分组：连续工具单元归入同一张过程卡。纯函数见 chat/models/blocks.ts（含单测）。 */
const renderGroups = computed(() => groupToolRuns(renderUnits.value))

// ===== 折叠状态（历史块用；流式期不折叠） =====
const localOpen = ref(new Map<string, boolean>())
function isExpandable(b: RenderBlock): boolean {
  if (b.kind === 'tool_call') return !!(b.call?.arguments || b.result?.content)
  if (b.kind === 'tool_result') return !!b.result?.content
  return false
}

function isError(b: RenderBlock): boolean {
  return !!b.result?.error && !b.result.refused
}

/** 工具结果的内容展示：识别行列表（file_list / doc_reader 等多行输出）按行渲染；其余按原文。 */
function toolResultLines(content: string): string[] {
  if (!content) return []
  return content.split('\n').filter((l) => l.length > 0)
}

/** cwd 短展示：取最后两级路径前缀，让用户在过程卡上一眼看见「命令落在哪」。 */
function shortCwd(cwd: string): string {
  if (!cwd) return ''
  const parts = cwd.replace(/\\/g, '/').split('/').filter(Boolean)
  if (parts.length <= 2) return cwd
  return '…/' + parts.slice(-2).join('/')
}

/** HTML 转义：v-html 渲染前的唯一入口（工具输出是模型可控内容，必须转义）。 */
function escapeHTML(s: string): string {
  return s.replace(/[&<>]/g, (c) => (c === '&' ? '&amp;' : c === '<' ? '&lt;' : '&gt;'))
}

// ===== 复制：长工具输出一键复制（args / result 各一个入口） =====
const toast = useToast()
// legacy: true 提供 execCommand 兜底；JCEF WebView 在非安全上下文下 navigator.clipboard 可能不可用。
const { copy: copyToClipboard } = useClipboard({ legacy: true })
/** 已复制的块 key（1.5s 后复位，用于按钮 ✓ 反馈）。 */
const copiedKey = ref<string | null>(null)

async function copyBlock(key: string, text: string): Promise<void> {
  if (!text) return
  try {
    await copyToClipboard(text)
    copiedKey.value = key
    setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = null
    }, 1500)
    toast.success(t('chat.copied'))
  } catch {
    /* 剪贴板失败静默（WebView 权限受限场景） */
  }
}

/** JSON 缩进美化；非 JSON（解析失败或非对象/数组开头）返回 null。 */
function prettyJSON(text: string): string | null {
  const t = (text ?? '').trim()
  if (!(t.startsWith('{') || t.startsWith('['))) return null
  try {
    return JSON.stringify(JSON.parse(t), null, 2)
  } catch {
    return null
  }
}

/** JSON 折叠阈值：超过此行数默认折叠，避免大输出把消息流拉长数倍。 */
const JSON_FOLD_LINES = 14

/** 已展开的 JSON 块 key（默认折叠，点击展开）。 */
const jsonOpen = ref<Set<string>>(new Set())

/** args / result 的块内 key（复制与折叠共用，避免模板里重复拼串）。 */
function argKey(b: RenderBlock): string {
  return `${b.kind}:${b.seq}:args`
}
function resultKey(b: RenderBlock): string {
  return `${b.kind}:${b.seq}:result`
}

/** 该内容是否为「可折叠的长 JSON」。 */
function jsonFoldable(text: string): boolean {
  const pretty = prettyJSON(text)
  return pretty !== null && pretty.split('\n').length > JSON_FOLD_LINES
}

/** 该 JSON 块当前是否处于折叠态（未手动展开 + 超阈值）。 */
function isJSONFolded(key: string, text: string): boolean {
  return !jsonOpen.value.has(key) && jsonFoldable(text)
}

function toggleJSON(key: string): void {
  const next = new Set(jsonOpen.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  jsonOpen.value = next
}

/** 对已缩进的 JSON 做转义 + 着色（不重新解析，供折叠版复用）。 */
function colorizeJSON(pretty: string): string {
  // 用捕获组区分 key 与 string value：key 后面紧跟 `:`（JSON.stringify 缩进格式）。
  // 不能靠「整体是否以冒号结尾」判断——字符串值自身可能以冒号结尾（如 "http://x:"）。
  return escapeHTML(pretty).replace(
    /("(?:\\.|[^\\"])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (_m, str?: string, colon?: string, lit?: string, num?: string) => {
      if (str !== undefined) {
        const cls = colon === undefined ? 'j-str' : 'j-key'
        return `<span class="${cls}">${str}${colon ?? ''}</span>`
      }
      if (lit !== undefined) {
        const cls = lit === 'null' ? 'j-null' : 'j-bool'
        return `<span class="${cls}">${lit}</span>`
      }
      return `<span class="j-num">${num}</span>`
    }
  )
}

/**
 * 代码块渲染：JSON 走缩进 + 着色，其余原样转义；长 JSON 按 {@link JSON_FOLD_LINES} 折叠。
 *
 * 安全性：先整体转义 `& < >`，再在**转义后**的文本上做 token 着色——
 * 着色只插入 class 为常量的 `<span>`，不含任何用户输入，因此 v-html 可安全使用。
 * 配色只取主色 + 灰阶（AGENTS.md：绿/红仅作状态灯，不用于语法高亮）。
 */
function codeBlockHTML(text: string, key?: string): string {
  if (!text) return ''
  const pretty = prettyJSON(text)
  if (pretty === null) return escapeHTML(text)
  if (key !== undefined && isJSONFolded(key, text)) {
    // 折叠版：保留前 N 行，末尾标记省略。着色仍按 token 生效（不重新解析）。
    return colorizeJSON(pretty.split('\n').slice(0, JSON_FOLD_LINES).join('\n')) +
      '\n<span class="j-fold">…</span>'
  }
  return colorizeJSON(pretty)
}

/** 是否为最后一个块：流式光标只挂在末块上（收缩成一处，避免模板里重复长表达式）。 */
function isLastBlock(b: RenderBlock): boolean {
  const list = renderUnits.value
  return props.streaming_mode === true && list.length > 0 && b === list[list.length - 1]
}

function isOpen(b: RenderBlock): boolean {
  const key = `${b.kind}:${b.seq}`
  const m = localOpen.value.get(key)
  if (m !== undefined) return m
  // 默认只展开「正在执行」或「失败」的块：一次 run 几十个工具全展开会把消息流拉长数倍
  return isError(b) || b.running === true
}
function toggle(b: RenderBlock): void {
  const key = `${b.kind}:${b.seq}`
  const next = new Map(localOpen.value)
  next.set(key, !isOpen(b))
  localOpen.value = next
}

// ===== 动词叙事：一行 = 动词 + 目标 + 增删徽标 =====
/** 迹线动词（i18n key tool.verb.*）。 */
function unitVerb(b: RenderBlock): string {
  return t(`tool.verb.${traceKind(b.call?.name ?? b.result?.name)}`)
}
/** 迹线图标（按动词类别，弱化具体工具差异）。 */
function unitIcon(b: RenderBlock) {
  return traceIcon(b.call?.name ?? b.result?.name)
}
/** 迹线目标：文件名为主 / 命令 / 查询词（从 args 结构化提取）。 */
function unitTarget(b: RenderBlock) {
  return b.call ? traceTarget(b.call.arguments) : null
}
/** 委派行的子代理名。 */
function unitAgent(b: RenderBlock): string {
  return b.call?.name === 'delegate_task' ? traceDelegateAgent(b.call.arguments) : ''
}
/** 增删徽标：结果内容是 diff 时统计 +/- 行数（无增删返回 null 不显示）。 */
function unitDelta(b: RenderBlock): { added: number; removed: number } | null {
  const content = b.result?.content
  if (!content || !(b.result?.uiHint === 'diff' || looksLikeDiff(content))) return null
  const c = countDiffLines(content)
  return c.added > 0 || c.removed > 0 ? c : null
}

/** 知识库检索的结构化命中（data.hits；缺失/形状不符返回空数组走原始文本渲染）。 */
function knowledgeHits(b: RenderBlock): KnowledgeHit[] {
  const hits = b.result?.data?.hits
  return Array.isArray(hits) ? (hits as KnowledgeHit[]) : []
}

// ===== 思考面板：thinking 块在流式中自动展开；其他默认折叠 =====
const thinkingOpen = ref(true)
watch(
  () => renderBlocks.value.some((b) => b.kind === 'text'),
  (hasText, prev) => {
    // 首条 text 出现 → 自动折叠思考
    if (hasText && !prev) thinkingOpen.value = false
  }
)
</script>

<template>
  <div class="wb-blocks">
    <!-- 块按顺序渲染：thinking / tool_call / tool_result / artifact / skill / genui / text。
         注意：循环 renderUnits（配对后的视图），不是 renderBlocks（原始数据）；
         这样 tool_call 与对应 tool_result 合并展示为一个工具单元，避免视觉上的『两次工具』。 -->
    <template v-for="g in renderGroups" :key="g.key">
      <!-- 连续工具单元 → 一张过程卡：行间极浅分隔线，形成卡内列表观感。
           正文块会自然切断分组，所以「叙述 → 工具组 → 叙述」的真实顺序得以保留。 -->
      <div v-if="g.tools.length > 0" class="wb-trace">
        <div
          v-for="b in g.tools"
          :key="`${b.kind}:${b.seq}`"
          class="wb-tool"
          :class="{
            open: isOpen(b),
            run: b.running,
            done: !b.running && !isError(b) && !b.result?.refused,
            err: isError(b),
            wait: Boolean(b.result?.refused)
          }"
        >
          <button
            type="button"
            class="wb-tool-hd"
            :style="!isExpandable(b) ? 'cursor: default' : ''"
            @click="isExpandable(b) && toggle(b)"
          >
            <span class="wb-tool-st">
              <Loader2 v-if="b.running" class="wb-loader animate-spin" />
              <Check v-else-if="b.result && !b.result.refused && !b.result.error" class="wb-ok" />
              <X v-else-if="b.result?.refused" class="wb-refused" />
              <Square v-else-if="b.result && b.result.error" class="wb-fail" />
              <Square v-else class="wb-pend" />
            </span>
            <component :is="unitIcon(b)" class="wb-ic" />
            <!-- 有调用：动词迹线（动词 + 目标 + Δ）；孤立结果：退化为工具名 -->
            <template v-if="b.call">
              <span class="wb-tool-verb">{{ unitVerb(b) }}</span>
              <!-- 委派：目标是子代理名 + 一行任务 -->
              <template v-if="b.call.name === 'delegate_task'">
                <span class="wb-tool-tgt-main">{{ unitAgent(b) }}</span>
                <span v-if="unitTarget(b)?.main" class="wb-tool-tgt-sub" :title="unitTarget(b)!.main">{{ unitTarget(b)!.main }}</span>
              </template>
              <!-- 常规：文件名粗体 + 目录淡色 / 命令 / 查询词 -->
              <template v-else-if="unitTarget(b)">
                <span class="wb-tool-tgt-main">{{ unitTarget(b)!.main }}</span>
                <span v-if="unitTarget(b)!.sub" class="wb-tool-tgt-sub">{{ unitTarget(b)!.sub }}</span>
              </template>
              <span v-else class="wb-tool-nm" :title="b.call.name">{{ toolLabel(b.call.name) }}</span>
            </template>
            <span v-else class="wb-tool-nm">{{ toolLabel(b.result?.name ?? '') }}</span>
            <span v-if="unitDelta(b)" class="wb-tool-delta">
              <span v-if="unitDelta(b)!.added" class="add">+{{ unitDelta(b)!.added }}</span>
              <span v-if="unitDelta(b)!.removed" class="del">-{{ unitDelta(b)!.removed }}</span>
            </span>
            <span v-if="b.result?.durationMs != null" class="wb-tool-ms">
              {{ (b.result.durationMs / 1000).toFixed(1) }}s
            </span>
            <!-- 内核改道信号：同工具连续失败计数（>=1 就显示，让用户看见模型在死磕） -->
            <span
              v-if="b.result?.meta?.same_failure_count && Number(b.result.meta.same_failure_count) >= 1"
              class="wb-tool-failcount"
              :title="t('chat.tool.failCountHint', { count: b.result.meta.same_failure_count })"
            >
              ×{{ b.result.meta.same_failure_count }}
            </span>
            <!-- AdaptiveLoopGuard 已注入 [guard] 改道提示 -->
            <span
              v-if="b.result?.meta?.adaptive_hint === '1'"
              class="wb-tool-guard"
              :title="t('chat.tool.guardHint')"
            >
              {{ t('chat.tool.guardBadge') }}
            </span>
            <!-- 工具实际执行目录：让用户能验证沙箱 cwd 与声称一致 -->
            <span
              v-if="b.result?.meta?.cwd"
              class="wb-tool-cwd"
              :title="b.result.meta.cwd"
            >
              {{ shortCwd(b.result.meta.cwd) }}
            </span>
            <span v-if="isError(b)" class="wb-tool-failed">{{ t('chat.execFailed') }}</span>
            <span v-else-if="b.result?.refused" class="wb-tool-refused">{{ t('chat.execRefused') }}</span>
            <ChevronDown v-if="isExpandable(b)" class="wb-tool-chev" :class="{ rotate: isOpen(b) }" />
          </button>
          <div v-if="isOpen(b) && (b.call?.arguments || b.result?.content)" class="wb-tool-bd">
            <template v-if="b.call?.arguments">
              <div class="wb-tool-lb-row">
                <p class="wb-tool-lb">args</p>
                <button
                  type="button"
                  class="wb-tool-copy"
                  :title="t('chat.copied')"
                  @click.stop="copyBlock(argKey(b), b.call?.arguments ?? '')"
                >
                  <Check v-if="copiedKey === argKey(b)" class="wb-tool-copy-ok" />
                  <Copy v-else class="wb-tool-copy-ic" />
                </button>
              </div>
              <!-- JSON 入参自动缩进 + 着色（已转义后再着色，v-html 安全）；长 JSON 默认折叠 -->
              <pre class="wb-tool-pre"><code v-html="codeBlockHTML(b.call.arguments, argKey(b))"></code></pre>
              <button
                v-if="jsonFoldable(b.call?.arguments ?? '')"
                type="button"
                class="wb-tool-fold"
                @click.stop="toggleJSON(argKey(b))"
              >
                {{ isJSONFolded(argKey(b), b.call?.arguments ?? '') ? t('chat.tool.expandLines') : t('chat.tool.collapseLines') }}
              </button>
            </template>
            <template v-if="b.result?.content">
              <div class="wb-tool-lb-row">
                <p class="wb-tool-lb">result</p>
                <button
                  type="button"
                  class="wb-tool-copy"
                  :title="t('chat.copied')"
                  @click.stop="copyBlock(resultKey(b), b.result?.content ?? '')"
                >
                  <Check v-if="copiedKey === resultKey(b)" class="wb-tool-copy-ok" />
                  <Copy v-else class="wb-tool-copy-ic" />
                </button>
              </div>
              <!-- 知识库检索：结构化命中 → 来源卡（可展开全文），替代文本墙 -->
              <KnowledgeHits
                v-if="b.result.name === 'knowledge_search' && knowledgeHits(b).length > 0"
                :hits="knowledgeHits(b)"
              />
              <!-- 多行结果按行展示（file_list / doc_reader 等） -->
              <ul
                v-else-if="toolResultLines(b.result.content).length > 1 && !(b.result.uiHint === 'diff' || looksLikeDiff(b.result.content))"
                class="wb-tool-lines"
              >
                <li v-for="(ln, li) in toolResultLines(b.result.content)" :key="li">
                  <span v-if="b.result.name === 'file_list'" class="wb-tool-line-path">📄</span>
                  <span v-else-if="b.result.name === 'file_glob'" class="wb-tool-line-path">🔍</span>
                  <code>{{ ln }}</code>
                </li>
              </ul>
              <DiffView
                v-else-if="b.result.uiHint === 'diff' || looksLikeDiff(b.result.content)"
                :diff="b.result.content"
                :max-height="224"
              />
              <!-- JSON 结果（exec 调 API / http 工具常见）自动缩进 + 着色；长 JSON 默认折叠 -->
              <template v-else>
                <pre class="wb-tool-pre"><code v-html="codeBlockHTML(b.result.content, resultKey(b))"></code></pre>
                <button
                  v-if="jsonFoldable(b.result.content)"
                  type="button"
                  class="wb-tool-fold"
                  @click.stop="toggleJSON(resultKey(b))"
                >
                  {{ isJSONFolded(resultKey(b), b.result.content) ? t('chat.tool.expandLines') : t('chat.tool.collapseLines') }}
                </button>
              </template>
            </template>
          </div>
        </div>
      </div>

      <!-- 思考块 -->
      <div v-else-if="g.one && g.one.kind === 'thinking' && g.one.text" class="wb-block-thinking">
        <button
          type="button"
          class="wb-think-toggle"
          :class="{ open: thinkingOpen }"
          @click="thinkingOpen = !thinkingOpen"
        >
          <Brain class="wb-ic" />
          <span>{{ thinkingOpen ? t('chat.thinking') : t('chat.thoughtDone') }}</span>
          <ChevronDown class="wb-chev" :class="{ rotate: thinkingOpen }" />
        </button>
        <pre v-if="thinkingOpen" class="wb-think-body">{{ g.one.text }}</pre>
      </div>

      <!-- 文本块：默认走内置 MarkdownRenderer，父组件可用 #text slot 覆盖（流式期挂光标）。
           内联 fallback 必须保留：父组件未传 slot 时内容为空，历史正文会整段消失。 -->
      <div v-else-if="g.one && g.one.kind === 'text' && g.one.text" class="wb-block-text">
        <slot name="text" :content="g.one.text" :streaming="isLastBlock(g.one)">
          <div class="flex items-end gap-1">
            <MarkdownRenderer :content="g.one.text" :streaming="isLastBlock(g.one)" />
            <span v-if="isLastBlock(g.one)" class="wb-cursor" />
          </div>
        </slot>
      </div>

      <!-- Skill 命中：单行 chip -->
      <div v-else-if="g.one && g.one.kind === 'skill' && g.one.skill" class="wb-block-skill">
        <span class="wb-skill-chip"><Sparkles class="wb-ic" /> {{ t('chat.skillHit', g.one.skill.name ?? '') }}</span>
      </div>

      <!-- Artifact 块：简化展示 -->
      <div v-else-if="g.one && g.one.kind === 'artifact' && g.one.artifact" class="wb-block-artifact">
        <div class="wb-art-card">{{ g.one.artifact.name }}</div>
      </div>

      <!-- GenUI：内联渲染 -->
      <div v-else-if="g.one && g.one.kind === 'genui' && g.one.genui" class="wb-block-genui">
        <GenUiRenderer :node="g.one.genui" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.wb-blocks {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* ===== 工具时间线（Agent「干活」的核心视觉）=====
 * 形态：左侧连续竖轨 + 节点状态灯 + 等宽行文——像日志流出的流水线，
 * 一眼能看出「跑到第几步、哪一步慢、哪一步失败」。 */
.wb-trace {
  position: relative;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wb-trace::before {
  content: '';
  position: absolute;
  left: 5px;
  top: 6px;
  bottom: 6px;
  width: 1px;
  background: var(--wb-line-2);
}
.wb-trace .wb-tool {
  position: relative;
  border-radius: var(--wb-radius);
  border: 1px solid transparent;
  transition: border-color var(--wb-dur) var(--wb-ease), background var(--wb-dur) var(--wb-ease);
}
.wb-trace .wb-tool::before {
  content: '';
  position: absolute;
  left: -16px;
  top: 9px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--wb-bg);
  border: 1.5px solid var(--wb-line-2);
  z-index: 1;
}
.wb-trace .wb-tool.run::before {
  border-color: var(--wb-primary);
  background: var(--wb-primary);
  animation: wb-node-pulse 1.8s infinite;
}
.wb-trace .wb-tool.done::before {
  border-color: var(--wb-success);
  background: var(--wb-success);
}
.wb-trace .wb-tool.err::before {
  border-color: var(--wb-danger);
  background: var(--wb-danger);
}
.wb-trace .wb-tool.wait::before {
  border-color: var(--wb-warning);
  background: var(--wb-warning);
}
@keyframes wb-node-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 var(--wb-primary-soft);
  }
  50% {
    box-shadow: 0 0 0 4px var(--wb-primary-soft);
  }
}
.wb-trace .wb-tool-hd {
  border-radius: var(--wb-radius);
}
.wb-trace .wb-tool-hd:hover {
  background: var(--wb-surface);
}
.wb-trace .wb-tool.open {
  background: var(--wb-surface);
  border-color: var(--wb-line);
}
/* 展开区：卡内嵌块（不用漂白的大白块，避免把行切断） */
.wb-trace .wb-tool-bd {
  margin: 0 6px 8px 6px;
  background: transparent;
  border-radius: var(--wb-radius-sm);
}
.wb-block-thinking {
  display: flex;
  flex-direction: column;
}
.wb-think-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 6px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  font-size: 11.5px;
  color: var(--wb-muted);
  cursor: pointer;
  align-self: flex-start;
  transition: color 0.15s ease, background 0.15s ease;
}
.wb-think-toggle:hover {
  background: var(--wb-surface-hover);
  color: var(--wb-ink);
}
.wb-think-toggle .wb-chev {
  width: 11px;
  height: 11px;
  transition: transform 0.18s ease;
}
.wb-think-toggle.open .wb-chev {
  transform: rotate(180deg);
}
.wb-think-body {
  margin: 4px 0 0 22px;
  padding: 0 0 0 10px;
  border-left: 2px solid var(--wb-border);
  font-family: inherit;
  font-size: 11.5px;
  line-height: 1.75;
  white-space: pre-wrap;
  color: var(--wb-muted);
  max-height: 8rem;
  overflow-y: auto;
}

.wb-block-text {
  display: contents;
}

/* 工具行：与 TaskTimeline 同形态但精简（无外框） */
.wb-tool {
  display: flex;
  flex-direction: column;
  border-radius: 6px;
  background: transparent;
  transition: background 0.15s ease;
}
.wb-tool-hd {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 5px 6px;
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
  border-radius: var(--wb-radius);
}
.wb-tool-hd:hover {
  background: var(--wb-surface-hover);
}
.wb-ic {
  width: 13px;
  height: 13px;
  color: var(--wb-muted);
}
/* 动词叙事：动词弱色、目标文件名亮色、目录淡色 mono */
.wb-tool-verb {
  font-size: 12px;
  color: var(--wb-muted);
}
.wb-tool-tgt-main {
  font-size: 12px;
  font-weight: 500;
  color: var(--wb-ink);
  max-width: 22rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.wb-tool-tgt-sub {
  font-size: 10.5px;
  color: var(--wb-muted);
  font-family: var(--font-mono);
  max-width: 16rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.wb-tool-delta {
  display: inline-flex;
  gap: 4px;
  font-family: var(--font-mono);
  font-size: 10.5px;
  font-variant-numeric: tabular-nums;
}
.wb-tool-delta .add {
  color: var(--wb-mint);
}
.wb-tool-delta .del {
  color: var(--wb-danger);
}
/* 失败标记：行尾小红 chip（原先只是红字，扫读时容易被忽略） */
.wb-tool-failed {
  font-size: 10px;
  font-weight: 600;
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
  border-radius: 999px;
  padding: 1px 7px;
  flex: none;
}
/* 拒绝是回执不是错误：琥珀色，语义与「失败」区分 */
.wb-tool-refused {
  font-size: 10px;
  font-weight: 600;
  color: var(--wb-warning);
  background: var(--wb-warning-soft);
  border-radius: 999px;
  padding: 1px 7px;
  flex: none;
}
.wb-tool-chev {
  width: 11px;
  height: 11px;
  color: var(--wb-muted);
  transition: transform 0.18s ease;
}
.wb-tool-chev.rotate {
  transform: rotate(180deg);
}
.wb-tool-nm {
  font-size: 11.5px;
  font-weight: 500;
  color: var(--wb-ink);
}
.wb-tool-st {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 13px;
  height: 13px;
}
.wb-ok { color: var(--wb-mint); width: 13px; height: 13px; }
.wb-refused { color: var(--wb-warning); width: 13px; height: 13px; }
.wb-fail { color: var(--wb-danger); width: 13px; height: 13px; }
.wb-loader { width: 12px; height: 12px; color: var(--wb-muted); }
.wb-tool-ms {
  font-size: 10.5px;
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  margin-left: 2px;
}
/* 失败计数徽标：同工具连续失败 ≥1 时显示，「×N」让用户看见模型在死磕 */
.wb-tool-failcount {
  display: inline-flex;
  align-items: center;
  margin-left: 2px;
  padding: 0 5px;
  height: 16px;
  border-radius: 8px;
  font-size: 10px;
  font-weight: 600;
  color: var(--wb-danger);
  background: color-mix(in oklab, var(--wb-danger) 12%, transparent);
  font-variant-numeric: tabular-nums;
}
/* 改道徽标：AdaptiveLoopGuard 已注入 [guard] 改道提示时的视觉信号 */
.wb-tool-guard {
  display: inline-flex;
  align-items: center;
  margin-left: 2px;
  padding: 0 6px;
  height: 16px;
  border-radius: 8px;
  font-size: 10px;
  font-weight: 600;
  color: var(--wb-primary-strong);
  background: color-mix(in oklab, var(--wb-primary) 12%, transparent);
  border: 1px solid color-mix(in oklab, var(--wb-primary) 35%, transparent);
}
/* cwd 徽标：exec 工具实际执行目录的短展示（让用户能验证沙箱） */
.wb-tool-cwd {
  display: inline-flex;
  align-items: center;
  margin-left: 2px;
  padding: 0 6px;
  height: 16px;
  border-radius: 6px;
  font-size: 10px;
  color: var(--wb-muted);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  font-family: var(--font-mono, ui-monospace, monospace);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.wb-tool-bd {
  margin: 3px 0 1px 24px;
  padding: 6px 8px;
  background: var(--wb-surface);
  border-radius: 6px;
  max-height: 14rem;
  overflow: auto;
  overscroll-behavior: contain;
}
/* args / result 标签：小 chip（比裸大写字母更清楚地划分段落） */
.wb-tool-lb {
  display: inline-block;
  margin: 0 0 5px;
  font-size: 9.5px;
  font-weight: 600;
  color: var(--wb-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  background: color-mix(in srgb, var(--wb-ink) 6%, transparent);
  border-radius: 5px;
  padding: 1px 6px;
}
/* 标签行 = 标签 + 复制按钮（复制放在标签右侧，不占正文宽度） */
.wb-tool-lb-row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 5px;
}
.wb-tool-lb-row .wb-tool-lb {
  margin-bottom: 0;
}
.wb-tool-copy {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  color: var(--wb-muted);
  background: transparent;
  transition: color 0.15s, background 0.15s;
}
.wb-tool-copy:hover {
  color: var(--wb-primary-strong);
  background: var(--wb-surface-hover);
}
.wb-tool-copy-ic {
  width: 11px;
  height: 11px;
}
.wb-tool-copy-ok {
  width: 11px;
  height: 11px;
  color: var(--wb-primary-strong);
}
/* 长 JSON 展开 / 收起：胶囊镂空（与全局按钮语法一致，实底只在 hover） */
.wb-tool-fold {
  display: inline-block;
  margin: 0 0 6px;
  padding: 0 8px;
  height: 20px;
  line-height: 18px;
  border: 1px solid var(--wb-border);
  border-radius: 10px;
  font-size: 10px;
  color: var(--wb-primary-strong);
  background: transparent;
  transition: background 0.15s, border-color 0.15s;
}
.wb-tool-fold:hover {
  background: var(--wb-surface-hover);
  border-color: color-mix(in oklab, var(--wb-primary) 35%, transparent);
}
.wb-tool-pre .j-fold {
  color: var(--wb-muted);
}
.wb-tool-pre {
  margin: 0 0 6px;
  font-family: ui-monospace, monospace;
  font-size: 11.5px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--wb-ink);
}
/* JSON 语法着色：只取主色 + 灰阶（设计令牌约束：绿/红仅作状态灯，不用于语法高亮） */
.wb-tool-pre .j-key {
  color: var(--wb-primary-strong);
  font-weight: 600;
}
.wb-tool-pre .j-str {
  color: var(--wb-ink);
}
.wb-tool-pre .j-num {
  color: var(--wb-primary);
}
.wb-tool-pre .j-bool {
  color: var(--wb-primary);
  font-style: italic;
}
.wb-tool-pre .j-null {
  color: var(--wb-muted);
}
.wb-tool-lines {
  list-style: none;
  padding: 0;
  margin: 0 0 4px;
  font-family: ui-monospace, monospace;
  font-size: 11.5px;
  line-height: 1.6;
}
.wb-tool-lines li {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding: 1px 4px;
  border-radius: 3px;
}
.wb-tool-lines li:hover {
  background: var(--wb-surface-hover);
}
.wb-tool-line-path {
  flex: none;
  font-size: 10px;
  opacity: 0.7;
}
.wb-tool.err .wb-tool-hd {
  background: var(--wb-danger-soft);
  border-radius: var(--wb-radius);
}

.wb-block-skill {
  display: flex;
}
/* 技能命中 chip：走品牌蓝而非紫——单点紫色在蓝调界面里会显得「不属于这里」 */
.wb-skill-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--wb-primary-soft);
  color: var(--wb-primary-strong);
  font-size: 11px;
  font-weight: 500;
}
.wb-art-card {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--wb-mint) 8%, transparent);
  color: var(--wb-mint);
  font-size: 11.5px;
}
</style>