<script setup lang="ts">
// 消息流：历史回合 + 正在进行的回合（思考 / 工具 / 流式正文按原始顺序）+ 提问卡 + 收尾提示。
// 顺序由 PI 的内容块决定：正文、思考、工具卡谁先谁后就按谁先谁后渲染。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import type { MessageVO } from '../../types/api'
import { renderMarkdown, renderMarkdownStream, resetStreamRender } from '../../utils/md'
import { fmtCount, fmtInt } from '../../utils/num'
import { groupTurns } from '../../utils/turns'
import AppIcon from '../common/AppIcon.vue'
import AskCard from './AskCard.vue'
import AssistantTurn from './AssistantTurn.vue'
import ChatMessage from './ChatMessage.vue'
import ThinkingBlock from './ThinkingBlock.vue'
import ToolCard from './ToolCard.vue'

// 收尾提示条上的动作：截断续写 / 出错重试由视图实现。
const props = defineProps<{
  onContinue?: () => Promise<void> | void
  onRetry?: () => Promise<void> | void
}>()

const { t } = useI18n()
const session = useSessionStore()
const chat = useChatStore()
const toast = useToastStore()
const router = useRouter()

const assistantName = computed(() => t('chat.assistant'))

const acting = ref<'' | 'continue' | 'retry'>('')
const cancellingRetry = ref(false)

async function stopRetry() {
  if (cancellingRetry.value) return
  cancellingRetry.value = true
  try {
    await chat.cancelRetry(session.currentId || '')
  } catch {
    /* store 层已弹提示，保持提示条与按钮可用即可 */
  } finally {
    cancellingRetry.value = false
  }
}

async function act(kind: 'continue' | 'retry') {
  if (acting.value || chat.running) return
  acting.value = kind
  try {
    if (kind === 'continue') await props.onContinue?.()
    else await props.onRetry?.()
  } finally {
    acting.value = ''
  }
}

// 模型配置类错误（重试解决不了）：出路都在设置页
const SETUP_CODES = new Set(['3101', '3111', '3113', '3114'])
const needSetup = computed(() => SETUP_CODES.has(chat.lastErrorCode))

const scroller = ref<HTMLElement | null>(null)
const stick = ref(true)

// 流式正文按段渲染：每段各自缓存 HTML，只有正在增长的那一段重算。
const partHtml = ref<Record<string, string>>({})
const renderedLen: Record<string, number> = {}
let renderTimer = 0

function renderGrowingParts() {
  for (const p of chat.parts) {
    if (p.kind !== 'text') continue
    if (renderedLen[p.key] === p.text.length) continue
    renderedLen[p.key] = p.text.length
    partHtml.value[p.key] = renderMarkdownStream(p.text)
  }
}

// 节流只负责把刷新放慢，不让每个 delta 都触发一次重排。
const STREAM_TICK_MS = 120
const STREAM_TICK_MS_LONG = 280
const STREAM_LONG_CHARS = 20000
const partsKey = computed(() =>
  chat.parts.map((p) => (p.kind === 'text' ? p.text.length : p.kind === 'tool' ? 1 : 0)).join(','),
)

watch(
  partsKey,
  () => {
    if (renderTimer) return
    const tick = chat.streaming.length > STREAM_LONG_CHARS ? STREAM_TICK_MS_LONG : STREAM_TICK_MS
    renderTimer = window.setTimeout(() => {
      renderTimer = 0
      renderGrowingParts()
    }, tick)
  },
  { immediate: true },
)

watch(
  () => chat.running,
  (running) => {
    if (renderTimer) {
      clearTimeout(renderTimer)
      renderTimer = 0
    }
    if (running) {
      // 新回合：上一轮的块缓存若留着，内容相同的块会被误判为未变而复用错误结果
      resetStreamRender()
      partHtml.value = {}
      for (const k of Object.keys(renderedLen)) delete renderedLen[k]
      return
    }
    // 收尾补一次高亮完整渲染：流式期间跳过了 hljs
    const out: Record<string, string> = {}
    for (const p of chat.parts) if (p.kind === 'text') out[p.key] = renderMarkdown(p.text)
    partHtml.value = out
  },
)
onBeforeUnmount(() => {
  if (renderTimer) clearTimeout(renderTimer)
})

// 正文一出现，正在思考的那段就自动收起：视线该落在答案上。
const answerStarted = computed(() => chat.streaming.length > 0)

// 工具记录按 id 直取（store 里就是一张表）：模板里逐段取多次也不变成 O(n²)
function runOf(id: string) {
  return chat.runOf(id)
}

function onScroll() {
  if (jumping) return
  const el = scroller.value
  if (!el) return
  stick.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

let jumping = false
function jumpToLatest() {
  const el = scroller.value
  if (!el) return
  jumping = true
  stick.value = true
  el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  window.setTimeout(() => {
    jumping = false
  }, 400)
}

const fmtElapsed = computed(() => {
  const s = chat.elapsedSec
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${s % 60}s`
})

type TurnBlock = import('../../utils/turns').TurnBlock<unknown>
const turnBlocks = computed<TurnBlock[]>(() => groupTurns(session.messages, () => []))

// 贴底滚动走 rAF 合并：逐 delta 读 scrollHeight 会把主线程压成强制重排
let scrollPending = false
function stickToBottom() {
  if (!stick.value || scrollPending || jumping) return
  scrollPending = true
  requestAnimationFrame(async () => {
    scrollPending = false
    await nextTick()
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

// 进页 / 切回会话时钉底：content-visibility 让远处回合先按 120px 估高，
// 一帧之后才展开成真实高度。只钉一次会被后续的高度变化抛在后面，表现为「打开就在中间」。
async function pinBottom(rounds = 3) {
  for (let i = 0; i < rounds; i++) {
    await new Promise<void>((r) => requestAnimationFrame(() => r()))
    const el = scroller.value
    if (!el || !stick.value) return
    el.scrollTop = el.scrollHeight
  }
}
watch(
  () =>
    [
      session.messages.length,
      chat.parts.length,
      chat.streaming.length,
      chat.asks.length,
    ] as const,
  stickToBottom,
)

// 打开会话时消息可能已经加载完了才挂载本组件，watch 不会为「首次就有数据」触发
onMounted(async () => {
  await nextTick()
  stick.value = true
  await pinBottom()
})

// 一条消息有没有可读内容：只有 toolCall 的回合不画正文外壳（工具卡单独列出）。
// 不滤的话，从命令行跑出来的会话里会有上百个空框。
function hasBody(msg: MessageVO): boolean {
  return !!(
    msg.content?.trim() ||
    msg.thinking?.trim() ||
    msg.images?.length ||
    msg.tool_calls?.length
  )
}
type TurnPart = { msg: MessageVO; tools: MessageVO[] }
function shownTurn(parts: TurnPart[]): boolean {
  return parts.some((p) => hasBody(p.msg) || p.tools.length > 0)
}

/** 一个回合的用量脚注取最后一条带 usage 的助手消息 */
function turnUsage(parts: TurnPart[]) {
  for (let i = parts.length - 1; i >= 0; i--) {
    const u = parts[i].msg.usage
    if (u) return u
  }
  return null
}

function fmtTime(ms: number) {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// 用量读数四件：提示词总量（命中 + 未命中 + 写入）/ 输出 / 缓存命中率 / 上下文占用。
function metricsOf(u: ReturnType<typeof turnUsage>) {
  if (!u) return []
  const prompt = u.input + u.cached + (u.cache_write || 0)
  const out: { k: string; v: string; hi?: boolean }[] = [
    { k: '↓', v: fmtCount(prompt) },
    { k: '↑', v: fmtCount(u.output) },
  ]
  if (prompt > 0) {
    const rate = Math.round((u.cached / prompt) * 100)
    out.push({ k: 'cache', v: `${rate}%`, hi: rate > 0 })
  }
  if (u.context > 0) out.push({ k: 'ctx', v: fmtCount(u.context) })
  return out
}

// 代码块复制：正文是 v-html 注入的，用事件委托接住按钮
async function onStreamClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement | null)?.closest<HTMLElement>('.code-copy')
  if (!btn) return
  const code = btn.closest('.code-block')?.querySelector('pre')?.textContent || ''
  if (!code) return
  try {
    await navigator.clipboard.writeText(code)
    btn.classList.add('is-done')
    window.setTimeout(() => btn.classList.remove('is-done'), 1200)
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}

const copiedTurn = ref('')
async function copyTurn(text: string, key: string) {
  if (!text.trim()) return
  try {
    await navigator.clipboard.writeText(text)
    copiedTurn.value = key
    window.setTimeout(() => {
      if (copiedTurn.value === key) copiedTurn.value = ''
    }, 1200)
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}
</script>

<template>
  <div ref="scroller" class="msg-scroll" @scroll.passive="onScroll" @click="onStreamClick">
    <div class="msg-col">
      <template v-for="b in turnBlocks" :key="b.key">
        <ChatMessage v-if="b.kind === 'user'" :msg="b.msg" />

        <!-- 分隔条：已压缩上下文 / 换了模型 / 调整了思考档 -->
        <div v-else-if="b.kind === 'notice'" class="turn-divider">
          <span class="td-label">{{ b.msg.notice }}</span>
          <span v-if="b.msg.content" class="td-text" :title="b.msg.content">{{ b.msg.content }}</span>
        </div>

        <div v-else-if="b.kind === 'assistant' && shownTurn(b.parts)" class="turn">
          <div class="msg-ai">
            <div class="msg-badge"><span>{{ assistantName }}</span></div>
            <div class="msg-ai-body">
              <AssistantTurn
                v-for="p in b.parts"
                :key="p.msg.id"
                :msg="p.msg"
                :tools="p.tools"
              />
              <div v-if="metricsOf(turnUsage(b.parts)).length" class="msg-acts is-on turn-foot">
                <button
                  class="ai-act"
                  type="button"
                  :disabled="copiedTurn === b.key"
                  @click="
                    copyTurn(
                      b.parts.map((p) => p.msg.content || '').filter(Boolean).join('\n\n'),
                      b.key,
                    )
                  "
                >
                  <AppIcon :name="copiedTurn === b.key ? 'check' : 'copy'" size="ic-2xs" />
                  <span>{{ copiedTurn === b.key ? t('common.copied') : t('common.copy') }}</span>
                </button>
                <span
                  v-for="m in metricsOf(turnUsage(b.parts))"
                  :key="m.k"
                  class="ai-metric"
                  :class="{ 'is-hit': m.hi }"
                >
                  <b>{{ m.k }}</b>{{ m.v }}
                </span>
                <time v-if="b.parts[b.parts.length - 1]?.msg.created_at" class="ai-time">
                  {{ fmtTime(b.parts[b.parts.length - 1].msg.created_at) }}
                </time>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- 正在进行的回合：段顺序就是内核吐出的顺序。
           收尾期间（finalizing）继续挂在这儿，等权威快照接上再换掉。 -->
      <div v-if="chat.running || chat.finalizing" class="turn">
        <div class="msg-ai is-live">
          <div class="msg-badge is-live">
            <span>
              <span class="bd-dot" aria-hidden="true" />
              {{ assistantName }}
            </span>
          </div>
          <div class="msg-ai-body">
            <template v-for="p in chat.parts" :key="p.key">
              <ThinkingBlock
                v-if="p.kind === 'thinking'"
                :text="p.text"
                live
                :collapse="answerStarted"
              />
              <div
                v-else-if="p.kind === 'text'"
                class="prose"
                :class="{ 'is-last': p.key === chat.parts[chat.parts.length - 1]?.key }"
                v-html="partHtml[p.key] || ''"
              />
              <ToolCard
                v-else
                :label="runOf(p.toolCallId)?.label || 'tool'"
                :tool="runOf(p.toolCallId)?.tool || ''"
                :args="runOf(p.toolCallId)?.args"
                :running="runOf(p.toolCallId)?.running ?? true"
                :ok="runOf(p.toolCallId)?.ok ?? true"
                :title="runOf(p.toolCallId)?.title"
                :output="runOf(p.toolCallId)?.output"
                :duration_ms="runOf(p.toolCallId)?.duration_ms"
              />
            </template>

            <div v-if="chat.running && !chat.parts.length" class="typing">
              <span class="typing-dot" /><span class="typing-dot" /><span class="typing-dot" />
            </div>

            <div v-if="chat.parts.length && chat.running" class="run-meter">
              <span class="rm-item"><AppIcon name="clock" size="ic-2xs" />{{ fmtElapsed }}</span>
              <span v-if="chat.contextUsed > 0" class="rm-item">
                <AppIcon name="chart" size="ic-2xs" />{{ fmtCount(chat.contextUsed) }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- 引擎在问用户：问题来自 PI 进程，不挂在某一条消息上，统一排在流末尾 -->
      <AskCard v-for="a in chat.asks" :key="a.id" :ask="a" />

      <!-- 重试提示条：重试期间用户唯一的主动选择是「别试了」 -->
      <div v-if="chat.notice" class="notice-bar" :class="{ 'notice-bar--warn': chat.retrying }">
        <AppIcon name="info" size="ic-xs" />
        <span class="grow">{{ chat.notice }}</span>
        <el-button v-if="chat.retrying" size="small" :loading="cancellingRetry" :disabled="cancellingRetry" @click="stopRetry">
          {{ t('chat.stopRetry') }}
        </el-button>
      </div>

      <!-- 被截断：正文只写了一半，给一键续写 -->
      <div v-if="chat.truncated" class="notice-bar notice-bar--warn">
        <AppIcon name="alert" size="ic-xs" />
        <div class="grow">
          <b>{{ t('chat.noticeTruncated', { n: chat.truncBudget ? fmtInt(chat.truncBudget) : '—' }) }}</b>
        </div>
        <el-button
          size="small"
          :loading="acting === 'continue'"
          :disabled="chat.running || !!acting"
          @click="act('continue')"
        >
          {{ t('chat.continue') }}
        </el-button>
      </div>

      <div v-if="chat.lastError" class="notice-bar notice-bar--bad">
        <AppIcon name="alert" size="ic-xs" />
        <span class="grow">{{ chat.lastError }}</span>
        <el-button
          v-if="needSetup"
          size="small"
          @click="router.push({ name: 'settings', params: { tab: 'providers' } })"
        >
          {{ t('settings.providers.title') }}
        </el-button>
        <el-button
          size="small"
          :loading="acting === 'retry'"
          :disabled="chat.running || !!acting"
          @click="act('retry')"
        >
          {{ t('common.retry') }}
        </el-button>
      </div>
    </div>

    <!-- 回到底部：离开底部才出现，带文字说明，一眼知道是干什么的 -->
    <Transition name="jump-fade">
      <div v-if="!stick" class="jump-row">
        <button class="jump-btn" type="button" @click="jumpToLatest">
          <AppIcon name="chevron-down" size="ic-xs" />
          <span>{{ t('chat.jumpLatest') }}</span>
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.msg-col > * {
  animation: turn-in var(--wb-dur) var(--wb-ease);
}
@keyframes turn-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}
.turn-foot {
  margin-top: var(--wb-sp-1);
  padding-top: var(--wb-sp-1);
  border-top: 1px solid var(--wb-border);
}
/* 分隔条：两侧细线夹一行小字，比一整张卡片安静，也不会被当成一条回答 */
.turn-divider {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.turn-divider::before,
.turn-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--wb-border);
}
.td-label {
  flex: none;
  font-weight: 700;
  letter-spacing: 0.04em;
}
.td-text {
  flex: none;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
}
.ai-act {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: calc(22px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ai-act:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
  transform: scale(1.06);
}
.ai-act:active:not(:disabled) {
  transform: scale(0.92);
}
.ai-metric {
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
  padding: 0 var(--wb-sp-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  border-left: 1px solid var(--wb-border);
}
.ai-metric b {
  font-weight: 600;
  opacity: 0.7;
}
.ai-metric.is-hit {
  color: var(--wb-success);
}
.ai-time {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.run-meter {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.rm-item {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
}
.typing {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  padding: var(--wb-sp-2) 0;
}
.typing-dot {
  width: 5px;
  height: 5px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: typing-blink 1.1s var(--wb-ease) infinite;
}
.typing-dot:nth-child(2) {
  animation-delay: 0.18s;
}
.typing-dot:nth-child(3) {
  animation-delay: 0.36s;
}
@keyframes typing-blink {
  50% {
    opacity: 0.2;
    transform: translateY(-2px);
  }
}
.grow {
  flex: 1;
  min-width: 0;
}
.notice-bar b {
  font-weight: 700;
}
.jump-row {
  position: sticky;
  bottom: var(--wb-sp-4);
  height: 0;
  pointer-events: none;
  z-index: 2;
}
/* 行高是 0（不吃滚动内容的高度，也不会因为 v-if 切换把列表顶来顶去）：
   按钮以这行为基准往上贴，否则它会从这条零高的线往下长，下半截被滚动口切掉。 */
.jump-btn {
  position: absolute;
  left: 50%;
  bottom: 0;
  transform: translateX(-50%);
  pointer-events: auto;
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  border: 1px solid var(--wb-primary);
  box-shadow: var(--wb-shadow-3);
  color: #fff;
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    box-shadow var(--wb-dur-fast) var(--wb-ease);
}
.jump-btn:hover {
  transform: translateX(-50%) scale(1.06);
  box-shadow: var(--wb-shadow-4, var(--wb-shadow-3));
}
.jump-btn:active {
  transform: translateX(-50%) scale(0.92);
}
.jump-fade-enter-active,
.jump-fade-leave-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.jump-fade-enter-from,
.jump-fade-leave-to {
  opacity: 0;
}
</style>
