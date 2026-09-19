<script setup lang="ts">
/**
 * 输入框右下角上下文占用 popover：与顶栏 ContextRing 同源（裸数字 + 迷你环），
 * 点击展开分段详情 + 「压缩历史」快捷入口（原型 ctx-pop 形态）。
 */
import { computed } from 'vue'
import { t } from '@/i18n'
import type { ContextSegment } from '@/types/api'

const props = defineProps<{
  used: number
  max: number
  /** 各分段明细（不传则仅显示总数）。 */
  segments?: ContextSegment[]
  /** 真实 / 估算标识。 */
  estimated?: boolean
  /** 是否禁用压缩按钮（无会话或流式中）。 */
  compactDisabled?: boolean
}>()

const emit = defineEmits<{
  /** 用户点击「压缩历史」。 */
  compact: []
}>()

const pct = computed(() => (props.max <= 0 ? 0 : Math.min(100, Math.round((props.used / props.max) * 100))))

const color = computed(() => {
  if (pct.value >= 90) return 'var(--wb-danger)'
  if (pct.value >= 70) return 'var(--wb-warning)'
  return 'var(--wb-success)'
})

/** 千分位精简显示：128000 → 128k。 */
function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${Math.round(n / 1000)}k`
  return String(n)
}

const detail = computed(() => `${compact(props.used)} / ${compact(props.max)}`)

/** 迷你环尺寸：与顶栏 ContextRing 同源但缩到 14px。 */
const SIZE = 14
const R = 5
const CIRC = 2 * Math.PI * R

// segmentColor → 共享工具（避免 ContextRing / Popover 两份表漂移）。
import { colorForSegment } from '@/chat/models/contextSegments'
function segmentColor(key: string): string {
  return colorForSegment(key)
}
</script>

<template>
  <el-popover :width="280" placement="top-start" trigger="hover" :show-after="120">
    <template #reference>
      <!-- 紧凑形态：圆环 + 裸数字，与原型 ctx-pop 一致 -->
      <button
        type="button"
        class="flex shrink-0 cursor-default items-center gap-1 rounded px-1 py-0.5 transition-colors hover:bg-wb-surface-hover"
      >
        <svg :width="SIZE" :height="SIZE" :viewBox="`0 0 ${SIZE} ${SIZE}`" class="shrink-0">
          <circle :cx="SIZE / 2" :cy="SIZE / 2" :r="R" fill="none" stroke="var(--wb-border)" stroke-width="2" />
          <circle
            :cx="SIZE / 2"
            :cy="SIZE / 2"
            :r="R"
            fill="none"
            :stroke="color"
            stroke-width="2"
            stroke-linecap="round"
            :stroke-dasharray="`${(pct / 100) * CIRC} ${CIRC}`"
            :transform="`rotate(-90 ${SIZE / 2} ${SIZE / 2})`"
          />
        </svg>
        <span class="min-w-[18px] text-[10px] tabular-nums text-wb-muted">{{ pct }}</span>
      </button>
    </template>

    <div v-if="segments && segments.length > 0" class="space-y-3 p-1">
      <div class="flex items-baseline justify-between">
        <span class="text-sm font-medium text-wb-ink">{{ detail }} <span class="text-[10px] text-wb-muted">tokens</span></span>
        <span class="text-[11px]" :class="estimated ? 'text-wb-warning' : 'text-wb-muted'">
          {{ estimated ? t('chat.contextEstimated') : t('chat.contextMeasured') }}
        </span>
      </div>
      <ul class="space-y-1 text-xs">
        <li v-for="seg in segments" :key="seg.key" class="flex items-center gap-2">
          <span class="inline-block h-2 w-2 shrink-0 rounded-full" :style="{ background: segmentColor(seg.key) }" />
          <span class="text-wb-ink">{{ seg.title }}</span>
          <span class="ml-auto tabular-nums text-wb-muted">{{ compact(seg.tokens) }}</span>
        </li>
      </ul>
      <div
        v-if="pct >= 70"
        class="flex items-center justify-between rounded-md bg-wb-warning/10 px-2 py-1.5 text-[11px] text-wb-warning"
      >
        <span>{{ t('chat.ctxThresholdHint', 70) }}</span>
        <button
          type="button"
          class="rounded border border-wb-warning/40 bg-wb-surface px-2 py-0.5 text-[10.5px] font-medium text-wb-warning transition-colors hover:bg-wb-warning hover:text-white disabled:opacity-50"
          :disabled="compactDisabled"
          @click="emit('compact')"
        >
          {{ t('chat.compact') }}
        </button>
      </div>
    </div>
    <div v-else class="p-2 text-xs text-wb-muted">
      <div class="mb-1 text-sm font-medium text-wb-ink">{{ detail }} <span class="text-[10px] text-wb-muted">tokens</span></div>
      <div>{{ t('chat.contextNoData') }}</div>
    </div>
  </el-popover>
</template>

<style scoped>
/* 让 button 看起来不像 button，避免在 composer 行内出现点击感 */
button {
  background: transparent;
  border: 0;
}
</style>
