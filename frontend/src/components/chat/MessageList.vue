<script setup lang="ts">
// 消息流：历史回合 + 正在进行的回合（思考 / 工具 / 流式正文）+ 提问卡 + 收尾提示。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useChatStore, type ToolRun } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import { renderMarkdown, renderMarkdownStream, resetStreamRender } from '../../utils/md'
import { fmtCount, fmtInt } from '../../utils/num'
import { groupTurns } from '../../utils/turns'
import AppIcon from '../common/AppIcon.vue'
import AskCard from './AskCard.vue'
import ChatMessage from './ChatMessage.vue'
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
const liveThinkOpen = ref(true)

// 思考可能几万字：全量塞进 DOM 会让每帧重排压住主线程，也会把正文顶出屏幕。
// 只渲染尾部——推理是流式的，用户要看的是「现在想到哪」。
const THINK_TAIL = 4000
const liveThink = computed(() => {
  const v = chat.thinking
  if (v.length <= THINK_TAIL) return v
  return `…\n${v.slice(-THINK_TAIL)}`
})

// 正文一出现就自动收起思考：视线该落在答案上。只自动收一次。
let autoCollapsed = false
watch(
  () => chat.streaming,
  (text) => {
    if (text && !autoCollapsed) {
      autoCollapsed = true
      liveThinkOpen.value = false
    }
  },
)
watch(
  () => chat.running,
  (running) => {
    if (running) {
      autoCollapsed = false
      liveThinkOpen.value = true
    }
  },
)

// 流式渲染节流：渲染走块级增量缓存（见 utils/md），节流只负责把刷新放慢，
// 不让每个 delta 都触发一次重排。
const STREAM_TICK_MS = 120
const STREAM_TICK_MS_LONG = 280
const STREAM_LONG_CHARS = 20000
const renderedStream = ref('')
let renderTimer = 0
watch(
  () => chat.streaming,
  () => {
    if (renderTimer) return
    const tick = chat.streaming.length > STREAM_LONG_CHARS ? STREAM_TICK_MS_LONG : STREAM_TICK_MS
    renderTimer = window.setTimeout(() => {
      renderTimer = 0
      renderedStream.value = renderMarkdownStream(chat.streaming)
    }, tick)
  },
  { immediate: true },
)
watch(
  () => chat.running,
  (running) => {
    if (running) {
      // 新回合：上一轮的块缓存若留着，内容相同的块会被误判为未变而复用错误结果
      resetStreamRender()
      return
    }
    if (renderTimer) {
      clearTimeout(renderTimer)
      renderTimer = 0
    }
    // 收尾补一次高亮完整渲染：流式期间跳过了 hljs
    renderedStream.value = renderMarkdown(chat.streaming)
  },
)
onBeforeUnmount(() => {
  if (renderTimer) clearTimeout(renderTimer)
})

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

// 工具结果的参数不在自己身上，而在声明它的那条 assistant(tool_calls) 里
const toolArgsMap = computed(() => {
  const map = new Map<string, Record<string, unknown>>()
  for (const m of session.messages) {
    for (const tc of m.tool_calls || []) {
      if (tc.args) map.set(tc.id, tc.args)
    }
  }
  return map
})

const statusEntries = computed(() => Object.entries(chat.engineStatus))
const widgetEntries = computed(() => Object.entries(chat.engineWidgets))

const fmtElapsed = computed(() => {
  const s = chat.elapsedSec
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${s % 60}s`
})

type TurnBlock = import('../../utils/turns').TurnBlock<unknown>
const turnBlocks = computed<TurnBlock[]>(() => groupTurns(session.messages, () => []))

type RunBlock = { key: string; run: ToolRun }
const runBlocks = computed<RunBlock[]>(() =>
  chat.runs.map((r) => ({ key: `r-${r.tool_call_id}`, run: r })),
)

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
watch(
  () =>
    [session.messages.length, chat.streaming.length, chat.thinking.length, chat.runs.length, chat.asks.length] as const,
  stickToBottom,
)

// 打开会话时消息可能已经加载完了才挂载本组件，watch 不会为「首次就有数据」触发
onMounted(async () => {
  await nextTick()
  const el = scroller.value
  if (el) el.scrollTop = el.scrollHeight
})

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
</script>

<template>
  <div ref="scroller" class="msg-scroll" @scroll.passive="onScroll" @click="onStreamClick">
    <div class="msg-col">
      <template v-for="b in turnBlocks" :key="b.key">
        <ChatMessage v-if="b.kind === 'user'" :msg="b.msg" />

        <div v-else-if="b.kind === 'assistant'" class="turn">
          <div class="turn-body">
            <template v-for="p in b.parts" :key="p.msg.id">
              <ChatMessage :msg="p.msg" plain />
              <div v-if="p.tools.length" class="turn-tools">
                <ToolCard
                  v-for="tc in p.tools"
                  :key="tc.id"
                  :label="tc.tool_name || 'tool'"
                  :tool="tc.tool_name || ''"
                  :args="toolArgsMap.get(tc.tool_call_id || '')"
                  :running="false"
                  :ok="!tc.is_error"
                  :output="tc.content || ''"
                />
              </div>
            </template>
          </div>
        </div>
      </template>

      <!-- 正在进行的回合 -->
      <div v-if="chat.running" class="turn">
        <div class="msg-ai is-live">
          <div class="msg-badge is-live">
            <span>
              <span class="bd-dot" aria-hidden="true" />
              {{ assistantName }}
            </span>
          </div>
          <div class="msg-ai-body">
            <div v-if="chat.thinking" class="think is-live">
              <button
                class="think-head"
                type="button"
                :aria-expanded="liveThinkOpen"
                @click="liveThinkOpen = !liveThinkOpen"
              >
                <span class="live-dot" />
                <AppIcon name="brain" size="ic-xs" />
                <span>{{ t('chat.thinkingLive') }}</span>
                <span class="think-count">{{ t('chat.thinkChars', { n: chat.thinking.length }) }}</span>
                <AppIcon class="think-chev" :name="liveThinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
              </button>
              <div v-if="liveThinkOpen" class="think-body">{{ liveThink }}</div>
            </div>

            <div v-if="runBlocks.length" class="turn-tools">
              <ToolCard
                v-for="b in runBlocks"
                :key="b.key"
                :label="b.run.label"
                :tool="b.run.tool"
                :args="b.run.args"
                :running="b.run.running"
                :ok="b.run.ok"
                :title="b.run.title"
                :output="b.run.output"
                :duration_ms="b.run.duration_ms"
              />
            </div>

            <div v-if="chat.streaming" class="prose" v-html="renderedStream" />
            <div v-if="chat.streaming" class="stream-caret" />

            <div v-if="!chat.streaming && !chat.runs.length && !chat.thinking" class="typing">
              <span class="typing-dot" /><span class="typing-dot" /><span class="typing-dot" />
            </div>

            <div v-if="chat.streaming || chat.thinking || chat.runs.length" class="run-meter">
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

      <!-- 引擎扩展的状态条与小部件：来自 setStatus / setWidget -->
      <div v-if="statusEntries.length || widgetEntries.length" class="engine-fx">
        <div v-for="[k, v] in statusEntries" :key="`s-${k}`" class="efx-row">
          <span class="efx-dot" />
          <span class="efx-txt">{{ v }}</span>
        </div>
        <div v-for="[k, lines] in widgetEntries" :key="`w-${k}`" class="efx-widget">
          <div v-for="(ln, i) in lines" :key="i" class="efx-line">{{ ln }}</div>
        </div>
      </div>

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

    <!-- 回到底部：sticky 钉在滚动口下缘，只有离开底部才出现 -->
    <div v-if="!stick" class="jump-row">
      <el-tooltip :content="t('chat.jumpLatest', 'Bottom')" placement="left" :show-after="300">
        <button class="jump-btn" type="button" @click="jumpToLatest">
          <AppIcon name="chevron-down" size="ic" />
        </button>
      </el-tooltip>
    </div>
  </div>
</template>

<style scoped>
.msg-avatar.is-live {
  box-shadow: 0 0 0 3px var(--wb-live-soft);
}
.msg-col > * {
  animation: turn-in var(--wb-dur) var(--wb-ease);
}
@keyframes turn-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}
.engine-fx {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding-left: calc(var(--wb-avatar-size) + var(--wb-sp-3));
}
.efx-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.efx-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: efx-breathe 1.6s var(--wb-ease) infinite;
  flex: none;
}
@keyframes efx-breathe {
  0%,
  100% {
    opacity: 0.45;
  }
  50% {
    opacity: 1;
  }
}
.efx-widget {
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.efx-line {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
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
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: wb-pulse 1.4s var(--wb-ease) infinite;
}
.think-count {
  font-variant-numeric: tabular-nums;
  opacity: 0.75;
  font-weight: 500;
}
.think-chev {
  margin-left: auto;
  transition: transform var(--wb-dur-fast) var(--wb-ease);
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
  bottom: var(--wb-sp-3);
  height: 0;
  display: flex;
  justify-content: center;
  pointer-events: none;
}
.jump-btn {
  pointer-events: auto;
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  display: grid;
  place-items: center;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border-strong);
  box-shadow: var(--wb-shadow-3);
  color: var(--wb-ink-2);
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.jump-btn:hover {
  transform: scale(1.06);
  color: var(--wb-primary);
}
.jump-btn:active {
  transform: scale(0.94);
}
</style>
