<script setup lang="ts">
// 上下文水位：一枚环 + 两个读数，常驻输入框底栏；点开有详情弹层（剩余、缓存、速度）。
// 环用 conic-gradient 画，只占一个图标位却一眼可读；75% 警告色、90% 危险色。
// 没数据也渲染（降级成半透明「– / –」）——空着会让用户以为图标坏了。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TurnReading } from '@/composables/useMessageFlow'
import { fmtCount } from '@/utils/format'
import { useChatStore } from '@/stores/chat'

const { t } = useI18n()
const chat = useChatStore()

const open = ref(false)
const root = ref<HTMLElement | null>(null)

const percent = computed(() => Math.min(1, Math.max(0, chat.contextPercent)))
const shownPercent = computed(() => Math.round(percent.value * 100))
const level = computed(() => {
  if (percent.value >= 0.9) return 'bad'
  if (percent.value >= 0.75) return 'warn'
  return 'ok'
})

// 读数在数据缺失时显示「–」而不是 0：0 是一个测量结果，「–」才是没测过。
const tokens = computed(() => (chat.context ? formatTokens(chat.context.usage_tokens ?? 0) : '–'))
const windowTok = computed(() =>
  chat.context && chat.context.context_window > 0 ? formatTokens(chat.context.context_window) : '–',
)
const left = computed(() => {
  if (!chat.context || chat.context.context_window <= 0) return '–'
  return formatTokens(Math.max(0, chat.context.context_window - (chat.context.usage_tokens ?? 0)))
})

/** 有没有水位数据：决定整条的透明度与提示语，不决定渲染与否。 */
const known = computed(() => Boolean(chat.context && chat.context.context_window > 0))

// 缓存命中率取「最近一轮」的用量：直播中取直播轮，闲时取上一轮收尾的那次。
const cacheRate = computed(() => {
  const u: TurnReading | undefined = chat.turn
    ? chat.usageOf(chat.turn.run_id)
    : chat.usageOf(chat.lastFinishedRunId)
  if (!u) return ''
  const prompt = u.input_tokens + u.cache_read_tokens + u.cache_write_tokens
  if (prompt <= 0) return ''
  return `${Math.round((u.cache_read_tokens / prompt) * 100)}%`
})

const speed = computed(() => '')
const cost = computed(() => (chat.stats && chat.stats.cost_usd > 0 ? `$${chat.stats.cost_usd.toFixed(3)}` : ''))

const tip = computed(() =>
  known.value
    ? `${t('chat.contextHint', { used: tokens.value, win: windowTok.value })} · ${shownPercent.value}%`
    : t('chat.contextUnknown'),
)

function toggle(): void {
  open.value = !open.value
}

/** 点外面就收起：弹层是临时读数，不占用户的注意力预算。 */
function onDocClick(event: MouseEvent): void {
  if (!open.value) return
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocClick)
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick)
  window.removeEventListener('keydown', onKeydown)
})

/** token 数按量级压缩：界面上要能一眼比较，不是要精确到个位。 */
function formatTokens(n: number): string {
  return fmtCount(n)
}
</script>

<template>
  <div ref="root" class="meter-wrap" :class="{ 'is-unknown': !known }">
    <button type="button" class="meter" :class="level" :title="tip" @click="toggle">
      <span class="ring" :style="{ '--ctx-arc': `${shownPercent}%` }" />
      <span class="meter-text">{{ tokens }} / {{ windowTok }}</span>
    </button>

    <!-- 详情弹层：底栏放不下的那几个答案。只列有事实的行，没有就不占位。 -->
    <div v-if="open" class="ctx-panel">
      <div class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxUsed') }}</span>
        <span class="ctx-value">{{ tokens }} · {{ shownPercent }}%</span>
      </div>
      <div class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxWindow') }}</span>
        <span class="ctx-value">{{ windowTok }}</span>
      </div>
      <div class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxLeft') }}</span>
        <span class="ctx-value">{{ left }}</span>
      </div>
      <div v-if="cacheRate" class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxCache') }}</span>
        <span class="ctx-value">{{ cacheRate }}</span>
      </div>
      <div v-if="speed" class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxSpeed') }}</span>
        <span class="ctx-value">{{ speed }}</span>
      </div>
      <div v-if="cost" class="ctx-row">
        <span class="ctx-label">{{ t('chat.ctxCost') }}</span>
        <span class="ctx-value">{{ cost }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.meter-wrap {
  position: relative;
  display: inline-flex;
  flex: none;
}

/* 没数据时的降级态：整条半透明，说明「还没测」，而不是「没这个功能」 */
.meter-wrap.is-unknown .meter {
  opacity: 0.55;
}

/* 它现在是个控件：可点。但仍然克制——悬停只抬字色与描边，不做缩放胶囊，
   毕竟它一半的身份还是读数。 */
.meter {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 2px 6px;
  border: none;
  border-radius: var(--wb-r-sm);
  background: transparent;
  cursor: pointer;
  -webkit-user-select: none;
  user-select: none;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.meter:hover {
  background: var(--wb-sunken);
}

.meter:active {
  transform: scale(var(--wb-press-active));
}

/* 环：conic-gradient 画弧 + radial mask 挖内圈，一个元素完成。
   弧长直接拿百分比当停点，不用算圆周长。 */
.ring {
  width: 13px;
  height: 13px;
  flex: none;
  border-radius: 50%;
  background: conic-gradient(from -90deg, var(--wb-ok) var(--ctx-arc), var(--wb-line-strong) 0);
  -webkit-mask: radial-gradient(closest-side, transparent 70%, var(--wb-mask-ink) 72%);
  mask: radial-gradient(closest-side, transparent 70%, var(--wb-mask-ink) 72%);
}

.meter.warn .ring {
  background: conic-gradient(from -90deg, var(--wb-warn) var(--ctx-arc), var(--wb-line-strong) 0);
}

.meter.bad .ring {
  background: conic-gradient(from -90deg, var(--wb-bad) var(--ctx-arc), var(--wb-line-strong) 0);
}

.meter-text {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.meter.warn .meter-text {
  color: var(--wb-warn);
}

.meter.bad .meter-text {
  color: var(--wb-bad);
}

/* 详情弹层：贴着底栏往上弹，右对齐（它就在行末） */
.ctx-panel {
  position: absolute;
  bottom: calc(100% + 8px);
  right: 0;
  z-index: var(--wb-z-pop);
  min-width: 200px;
  padding: var(--wb-sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
}

.ctx-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--wb-sp-4);
}

.ctx-label {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.ctx-value {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
  font-variant-numeric: tabular-nums;
}
</style>
