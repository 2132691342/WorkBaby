<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ContextSegment, ContextUsageRESP } from '@/types/api'
import { t } from '@/i18n'
import { SEGMENT_COLOR as SHARED_SEGMENT_COLOR } from '@/chat/models/contextSegments'

/**
 * 上下文占用环形图。
 *
 * SVG 双环：外环 = 各段比例（首尾相连），内环 = 上下文窗口的「使用 vs 空闲」。
 * 详情面板悬停展开，鼠标离开折叠。Estimated 状态在副标题处标注。
 */
const props = defineProps<{ usage: ContextUsageRESP | null }>()

const SIZE = 76
const STROKE = 8
const R = (SIZE - STROKE) / 2
const CIRCUMFERENCE = 2 * Math.PI * R

/** 段颜色：与 wb-* 设计令牌一致，靠 dark mode 自动适配。 */
const SEGMENT_COLOR = SHARED_SEGMENT_COLOR

const segments = computed<ContextSegment[]>(() => props.usage?.segments ?? [])

/** 累积偏移量（SVG pathLength = CIRCUMFERENCE）。 */
const arcs = computed(() => {
  let offset = 0
  return segments.value.map((seg) => {
    const pct = seg.ratio / 1000
    const dash = CIRCUMFERENCE * pct
    const arc = { key: seg.key, color: SEGMENT_COLOR[seg.key] ?? 'var(--wb-primary)', dash, offset }
    offset += dash
    return arc
  })
})

const usedPercentDisplay = computed(() =>
  Math.round(((props.usage?.used_ratio ?? 0) / 1000) * 100)
)

const hovered = ref(false)
</script>

<template>
  <el-popover
    :width="280"
    placement="bottom-start"
    trigger="hover"
    :show-after="120"
    @show="hovered = true"
    @hide="hovered = false"
  >
    <template #reference>
      <button
        type="button"
        class="ctx-ring flex items-center gap-2 rounded-full border border-wb-border bg-wb-surface px-2.5 py-1 text-[11px] text-wb-muted transition-colors hover:border-wb-primary hover:text-wb-primary-strong"
        :title="t('chat.contextUsageTitle')"
      >
        <svg :width="SIZE / 2.4" :height="SIZE / 2.4" :viewBox="`0 0 ${SIZE / 2.4} ${SIZE / 2.4}`" class="shrink-0">
          <circle
            :cx="SIZE / 4.8"
            :cy="SIZE / 4.8"
            :r="R / 2.4"
            fill="none"
            stroke="var(--wb-border)"
            stroke-width="2"
          />
          <circle
            :cx="SIZE / 4.8"
            :cy="SIZE / 4.8"
            :r="R / 2.4"
            fill="none"
            :stroke="usedPercentDisplay >= 90 ? 'var(--wb-danger)' : 'var(--wb-primary)'"
            stroke-width="2"
            stroke-linecap="round"
            :stroke-dasharray="`${(usedPercentDisplay / 100) * (2 * Math.PI * (R / 2.4))} ${2 * Math.PI * (R / 2.4)}`"
            :transform="`rotate(-90 ${SIZE / 4.8} ${SIZE / 4.8})`"
          />
        </svg>
        <!-- 估算态用 ~ 前缀：百分比本身只是字符估算量级，不能当成实测占用读 -->
        <span class="tabular-nums">{{ usage?.estimated ? '~' : '' }}{{ usedPercentDisplay }}%</span>
      </button>
    </template>

    <div v-if="usage" class="space-y-3">
      <div class="flex items-baseline justify-between">
        <div class="text-sm font-medium text-wb-ink">{{ t('chat.contextUsageTitle') }}</div>
        <div class="text-[11px]" :class="usage.estimated ? 'text-wb-warning' : 'text-wb-muted'">
          {{ usage.estimated ? t('chat.contextEstimated') : t('chat.contextMeasured') }}
        </div>
      </div>

      <div class="flex items-center gap-4">
        <svg :width="SIZE" :height="SIZE" :viewBox="`0 0 ${SIZE} ${SIZE}`" class="shrink-0">
          <!-- 底环（空闲） -->
          <circle
            :cx="SIZE / 2"
            :cy="SIZE / 2"
            :r="R"
            fill="none"
            stroke="var(--wb-border)"
            :stroke-width="STROKE"
          />
          <!-- 段环（首尾相连） -->
          <g :transform="`rotate(-90 ${SIZE / 2} ${SIZE / 2})`">
            <circle
              v-for="arc in arcs"
              :key="arc.key"
              :cx="SIZE / 2"
              :cy="SIZE / 2"
              :r="R"
              fill="none"
              :stroke="arc.color"
              :stroke-width="STROKE"
              :stroke-dasharray="`${arc.dash} ${CIRCUMFERENCE}`"
              :stroke-dashoffset="-arc.offset"
              stroke-linecap="butt"
            />
          </g>
          <!-- 中心文本 -->
          <text
            :x="SIZE / 2"
            :y="SIZE / 2 - 4"
            text-anchor="middle"
            class="fill-wb-ink"
            font-size="14"
            font-weight="600"
          >
            {{ usedPercentDisplay }}%
          </text>
          <text
            :x="SIZE / 2"
            :y="SIZE / 2 + 12"
            text-anchor="middle"
            class="fill-wb-muted"
            font-size="9"
          >
            {{ Math.round(usage.used_tokens / 1000) }}k / {{ Math.round(usage.context_window / 1000) }}k
          </text>
        </svg>

        <ul class="flex-1 space-y-1.5 text-xs">
          <li v-for="seg in segments" :key="seg.key" class="flex items-center gap-2">
            <span
              class="inline-block h-2 w-2 shrink-0 rounded-full"
              :style="{ background: SEGMENT_COLOR[seg.key] ?? 'var(--wb-primary)' }"
            />
            <span class="text-wb-ink">{{ seg.title }}</span>
            <span class="ml-auto text-wb-muted tabular-nums">{{ Math.round(seg.tokens / 1000 * 10) / 10 }}k</span>
          </li>
        </ul>
      </div>

      <div class="text-[11px] text-wb-muted">
        {{ usage.message_count }} {{ t('chat.contextMessages') }} ·
        {{ usage.tool_count }} {{ t('chat.contextTools') }}
      </div>
    </div>
    <div v-else class="text-xs text-wb-muted">{{ t('chat.contextNoData') }}</div>
  </el-popover>
</template>
