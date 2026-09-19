<script setup lang="ts">
/**
 * 按事件到达顺序渲染 assistant 的完整过程块
 * （thinking / text / tool_call / tool_result / artifact / skill / genui）。
 * 同时接受历史稳定序列（ResolvedBlock[]）与流式暂存序列（StreamingBlock[]），
 * 内部统一转为 RenderBlock 后只写一遍渲染逻辑。
 */
import { computed, ref, watch } from 'vue'
import { Brain, Sparkles, Loader2, Square, Check, X, ChevronDown } from '@/components/common/icons'
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
  result?: { toolCallId: string; name: string; content: string; error?: string; durationMs?: number; refused?: boolean; uiHint?: string; data?: Record<string, unknown> | null }
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
          data: (b.data.data ?? null) as Record<string, unknown> | null
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
          data: (b.data?.data ?? null) as Record<string, unknown> | null
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
          :class="{ open: isOpen(b), err: isError(b) }"
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
            <span v-if="isError(b)" class="wb-tool-failed">{{ t('chat.execFailed') }}</span>
            <ChevronDown v-if="isExpandable(b)" class="wb-tool-chev" :class="{ rotate: isOpen(b) }" />
          </button>
          <div v-if="isOpen(b) && (b.call?.arguments || b.result?.content)" class="wb-tool-bd">
            <template v-if="b.call?.arguments">
              <p class="wb-tool-lb">args</p>
              <pre class="wb-tool-pre"><code>{{ b.call.arguments }}</code></pre>
            </template>
            <template v-if="b.result?.content">
              <p class="wb-tool-lb">result</p>
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
              <pre v-else class="wb-tool-pre">{{ b.result.content }}</pre>
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

/* ===== 过程卡（连续工具单元）=====
 * 一屏十几个工具若每行裸露，消息流就是十几行散文字；收进一张卡后
 * 行间用极浅分隔线、hover 整行高亮，形成卡内列表的秩序感。 */
.wb-trace {
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  background: var(--wb-surface-solid);
  overflow: hidden;
}
.wb-trace .wb-tool + .wb-tool {
  border-top: 1px solid color-mix(in srgb, var(--wb-border) 65%, transparent);
}
.wb-trace .wb-tool-hd {
  border-radius: 0;
}
.wb-trace .wb-tool-hd:hover {
  background: color-mix(in srgb, var(--wb-primary) 6%, transparent);
}
/* 展开区：卡内嵌块（不用漂白的大白块，避免把行切断） */
.wb-trace .wb-tool-bd {
  margin: 0 8px 8px 24px;
  background: var(--wb-surface-2);
  border-radius: 8px;
}
.wb-trace .wb-tool.err .wb-tool-hd {
  border-radius: 0;
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
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 6px;
  border: 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
  align-self: flex-start;
  border-radius: 4px;
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
  background: color-mix(in srgb, var(--wb-danger) 12%, transparent);
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
.wb-tool-pre {
  margin: 0 0 6px;
  font-family: ui-monospace, monospace;
  font-size: 11.5px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--wb-ink);
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
  background: color-mix(in srgb, var(--wb-danger) 8%, transparent);
  border-radius: 4px;
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