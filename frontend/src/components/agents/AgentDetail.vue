<script setup lang="ts">
// 子代理详情的读数区：阶段、进度条、六个读数、正文尾部。
// 数据由外部 ticker 每秒拉一次，进度条只呈现已到货的值——
// 每 900ms 从 12% 跳到 13% 会被读成「卡住了」。
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentRow } from '@/types/api'

const props = defineProps<{
  detail: AgentRow
  /** 展开态（宽面板）才允许展开正文尾部 */
  wide: boolean
  /** 正文尾部若干条消息（已抽好纯文本，渲染层不再解结构） */
  transcript: string[]
}>()

const { t } = useI18n()

// 正文默认收起：子代理的任务书与输出动辄几千字，
// 一选中就铺开一堵字墙，把进度读数顶到看不见的地方。
const expanded = ref(false)
watch(
  () => props.detail.id,
  () => {
    expanded.value = false
  },
)

function money(v: number): string {
  if (!v) return '$0'
  return v < 0.01 ? `$${v.toFixed(5)}` : `$${v.toFixed(3)}`
}

function tokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

function seconds(ms: number): string {
  if (!ms) return '0s'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m${s % 60}s`
}
</script>

<template>
  <div class="detail">
    <div class="detail-stage">
      {{ props.detail.progress.stage || props.detail.description || props.detail.task }}
    </div>

    <div class="bar">
      <div
        class="bar-fill"
        :style="{ width: `${Math.min(100, Math.max(0, props.detail.progress.completion))}%` }"
      />
    </div>

    <div class="readouts">
      <span>{{ props.detail.progress.completion }}%</span>
      <span v-if="props.detail.progress.current_tool" class="mono">
        {{ props.detail.progress.current_tool }}
      </span>
      <span>{{ money(props.detail.progress.cost) }}</span>
      <span>{{ tokens(props.detail.progress.tokens) }} tok</span>
      <span v-if="props.detail.progress.context_max">
        {{ Math.round((props.detail.progress.context_used / props.detail.progress.context_max) * 100) }}%
      </span>
      <span>{{ seconds(props.detail.progress.duration_ms) }}</span>
    </div>

    <template v-if="props.wide && props.transcript.length">
      <button class="transcript-toggle" type="button" @click="expanded = !expanded">
        {{ expanded ? t('agents.hideTranscript') : t('agents.showTranscript') }}
      </button>
      <div v-if="expanded" class="transcript selectable">
        <p v-for="(line, i) in props.transcript" :key="i" class="turn">{{ line }}</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
}

/* 阶段 / 任务书只给两行：任务书原文是几千字的 prompt，
   全文铺开会把详情区变成一道字墙，进度读数反而看不见。 */
.detail-stage {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  line-height: 1.55;
  overflow-wrap: anywhere;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.transcript-toggle {
  align-self: flex-start;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--wb-faint);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  cursor: pointer;
  transition: color var(--wb-dur-fast) var(--wb-ease);
}

.transcript-toggle:hover {
  color: var(--wb-brand);
}

.bar {
  height: 4px;
  border-radius: var(--wb-r-full);
  background: var(--wb-hover);
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background: var(--wb-brand-2);
  transition: width var(--wb-dur) var(--wb-ease);
}

.readouts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
}

.mono {
  color: var(--wb-brand);
}

/* 正文尾部：只给最后几条，且自己滚。 */
.transcript {
  max-height: 220px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  background: var(--wb-code-bg);
}

.turn {
  margin: 0;
  font-size: var(--wb-fs-3xs);
  line-height: 1.6;
  color: var(--wb-ink-5);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
