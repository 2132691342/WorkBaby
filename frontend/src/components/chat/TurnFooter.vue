<script setup lang="ts">
// 轮尾：读数（用量组成条 + 精确值）+ 一句动作（复制 / 分叉 / 隐藏）。
// 从 MessageTurn 拆出来守住单文件行数上限；它只读传进来的用量与文本，
// 自己持有「复制到了 / 正在分叉」这类瞬时态。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmAction, confirmDanger } from '@/utils/confirm'
import AppIcon from '@/components/common/AppIcon.vue'
import { fmtCount, fmtMs } from '@/utils/format'
import { useToasts } from '@/composables/useToasts'
import { useChatStore } from '@/stores/chat'
import type { TurnUsage } from '@/types/api'

const props = defineProps<{
  /** 这一轮的纯文本（复制用）；空串表示无物可复制 */
  text: string
  /** 用量：直播轮来自流（带耗时），历史轮来自后端投影（没有耗时那一项） */
  usage?: TurnUsage & { elapsed_ms?: number }
  /** 要不要显示读数：只在最后一轮显示，一次回答里的多条助手消息已并成一轮 */
  showMetrics: boolean
  /** 分叉 / 隐藏要的轮标识；空表示这一轮不可操作（直播中） */
  hid: string
  /** 用户轮才能从这里分叉 */
  isUser: boolean
}>()

const { t } = useI18n()
const toasts = useToasts()
const chat = useChatStore()

const copied = ref(false)
const forkBusy = ref(false)

const hasText = computed(() => props.text.trim().length > 0)

/** 确认框里复述这一轮的开头：一轮轮长得像，不带它等于让人盲点。 */
const excerpt = computed(() => {
  const line = props.text.trim().split('\n').find((l) => l.trim()) ?? ''
  const flat = line.trim()
  if (!flat) return t('common.untitled')
  return flat.length > 48 ? `${flat.slice(0, 48)}…` : flat
})

/** 读数：提示词总量 / 输出 / 缓存命中率 / 花费 / 耗时。 */
const metrics = computed(() => {
  if (!props.showMetrics) return []
  const u = props.usage
  if (!u) return []
  const prompt = u.input_tokens + u.cache_read_tokens + u.cache_write_tokens
  const out: { key: string; value: string; hit?: boolean }[] = []
  if (prompt > 0) out.push({ key: '↓', value: fmtCount(prompt) })
  if (u.output_tokens > 0) out.push({ key: '↑', value: fmtCount(u.output_tokens) })
  if (prompt > 0) {
    const rate = Math.round((u.cache_read_tokens / prompt) * 100)
    out.push({ key: 'cache', value: `${rate}%`, hit: rate > 0 })
  }
  if (u.cost_usd > 0) out.push({ key: '$', value: u.cost_usd.toFixed(u.cost_usd < 0.01 ? 4 : 3) })
  if (u.elapsed_ms && u.elapsed_ms > 0) out.push({ key: '⏱', value: fmtMs(u.elapsed_ms) })
  return out
})

/** token 组成条：缓存 / 输入 / 输出按占比分段，一眼看出「这轮的钱花在哪」。 */
const segments = computed(() => {
  if (!props.showMetrics) return []
  const u = props.usage
  if (!u) return []
  const total = u.cache_read_tokens + u.input_tokens + u.output_tokens
  if (total <= 0) return []
  return [
    { key: 'cache', cls: 'seg-cache', pct: (u.cache_read_tokens / total) * 100 },
    { key: 'input', cls: 'seg-input', pct: (u.input_tokens / total) * 100 },
    { key: 'output', cls: 'seg-output', pct: (u.output_tokens / total) * 100 },
  ].filter((s) => s.pct > 0)
})

/** 悬停给精确数：组成条看比例，精确值靠这行 title。 */
const usageTitle = computed(() => {
  const u = props.usage
  if (!u) return ''
  return `cache ${u.cache_read_tokens} · in ${u.input_tokens} · out ${u.output_tokens}`
})

async function copy(): Promise<void> {
  if (!hasText.value) return
  try {
    await navigator.clipboard.writeText(props.text)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1400)
  } catch {
    // 复制失败要说一声：按钮什么都不变，用户会以为「点了没反应」。
    toasts.push('warn', t('common.copyFailed'))
  }
}

// 隐藏这一轮：会话文件与上下文一个字不动，撤销入口在提示上。
async function remove(): Promise<void> {
  if (!props.hid) return
  const ok = await confirmDanger(
    t('chat.hideTurnHint', { excerpt: excerpt.value }),
    t('chat.hideTurnTitle'),
    t('chat.hideTurnConfirm'),
    t('common.cancel'),
  )
  if (!ok) return
  try {
    await chat.hideTurn(props.hid)
    const id = props.hid
    toasts.push('ok', t('chat.turnHidden'), undefined, {
      label: t('common.undo'),
      run: () => {
        chat.unhideTurn(id).catch((err) => toasts.push('error', err instanceof Error ? err.message : String(err)))
      },
    })
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

// 从这一条用户消息分叉：原会话不动，新会话只带到这里。
async function fork(): Promise<void> {
  if (!props.hid || forkBusy.value) return
  const ok = await confirmAction(
    t('chat.forkTurnHint'),
    t('chat.forkTurnTitle'),
    t('chat.forkTurnConfirm'),
    t('common.cancel'),
    { type: 'info' },
  )
  if (!ok) return
  forkBusy.value = true
  try {
    await chat.forkChat(props.hid)
    toasts.push('ok', t('chat.forkDone'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    forkBusy.value = false
  }
}
</script>

<template>
  <div class="msg-acts">
    <span v-if="segments.length" class="msg-usage-bar" :title="usageTitle" aria-hidden="true">
      <span v-for="s in segments" :key="s.key" class="seg" :class="s.cls" :style="{ width: s.pct + '%' }" />
    </span>
    <span v-if="metrics.length" class="msg-metrics">
      <span v-for="m in metrics" :key="m.key" class="msg-metric" :class="{ 'is-hit': m.hit }">
        <b>{{ m.key }}</b>{{ m.value }}
      </span>
    </span>
    <button v-if="hasText" class="msg-act" type="button" :title="t('common.copy')" @click="copy">
      <AppIcon :name="copied ? 'check' : 'copy'" :size="11" />
      {{ copied ? t('common.copied') : t('common.copy') }}
    </button>
    <button
      v-if="hid && isUser"
      class="msg-act"
      type="button"
      :class="{ 'is-loading': forkBusy }"
      :disabled="forkBusy"
      :title="t('chat.forkTurnTitle')"
      @click="fork"
    >
      <AppIcon name="branch" :size="11" />
      {{ t('chat.forkTurn') }}
    </button>
    <button v-if="hid" class="msg-act danger" type="button" :title="t('chat.hideTurnTitle')" @click="remove">
      <AppIcon name="trash" :size="11" />
      {{ t('common.delete') }}
    </button>
  </div>
</template>

<style scoped>
/* 默认透明，悬停整个回合才浮现；键盘用户靠 :focus-within 也能看到。 */
.msg-acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-3);
  min-height: 18px;
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}

/* 父级回合的悬停状态要穿透进来：scoped 样式选不到 .msg，
   所以用 :global 挂一次——这一行是本组件唯一的显示开关。 */
:global(.msg:hover) .msg-acts,
:global(.msg:focus-within) .msg-acts,
.msg-acts:has(.msg-metrics) {
  opacity: 1;
}

/* 用量读数：等宽数字 + 极淡的分隔线。它是读数不是控件——不套描边胶囊、不做悬停缩放。 */
.msg-metrics {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding-right: var(--wb-sp-2);
  border-right: 1px solid var(--wb-line);
}

/* token 组成条：4px 高的占比段，缓存绿 / 输入灰 / 输出靛。 */
.msg-usage-bar {
  display: inline-flex;
  width: var(--wb-tile);
  height: var(--wb-sp-1);
  flex: none;
  border-radius: var(--wb-r-full);
  overflow: hidden;
  background: var(--wb-sunken);
}

.msg-usage-bar .seg {
  height: 100%;
}

.msg-usage-bar .seg-cache {
  background: var(--wb-ok);
}

.msg-usage-bar .seg-input {
  background: var(--wb-faint);
}

.msg-usage-bar .seg-output {
  background: var(--wb-brand);
}

.msg-metric {
  display: inline-flex;
  align-items: baseline;
  gap: 2px;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-variant-numeric: tabular-nums;
  font-family: var(--wb-font-mono);
}

.msg-metric b {
  font-weight: 600;
  opacity: 0.7;
}

/* 缓存命中率：命中就标绿——它是「这次几乎没花钱」的那件事 */
.msg-metric.is-hit {
  color: var(--wb-ok);
}

.msg-act {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  padding: 0 6px;
  border: none;
  background: transparent;
  color: var(--wb-faint);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.msg-act:hover:not(:disabled) {
  color: var(--wb-brand);
  transform: scale(var(--wb-press-hover));
}

.msg-act:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
}

.msg-act:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* 加载：转圈 + 锁交互 + 锁尺寸。转圈照抄 base-buttons .wb-btn.is-loading
   的伪元素实现，图标让位给它（visibility 占位不塌，文字保留）。 */
.msg-act.is-loading {
  cursor: progress;
  opacity: 0.85;
  pointer-events: none;
}

.msg-act.is-loading > * {
  visibility: hidden;
}

.msg-act.is-loading::before {
  content: '';
  position: absolute;
  left: 6px;
  top: 50%;
  margin-top: -6px;
  width: 12px;
  height: 12px;
  border: 1.6px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: wb-spin 640ms linear infinite;
}

/* 删除是破坏性动作：常规态就要与复制分开一点颜色，悬停转危险色 */
.msg-act.danger {
  color: var(--wb-mute);
}

.msg-act.danger:hover:not(:disabled) {
  color: var(--wb-bad);
}
</style>
