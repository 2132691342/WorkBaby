<script setup lang="ts">
// 执行时间线：一次 run 里每个工具动作的瀑布条。
// 隐藏会话按保留期归档后，逐字对话没了，这张图是「当时到底干了什么」的凭据。
import { computed } from 'vue'
import type { RunStep, RunTimeline } from '../../types/api'

const props = defineProps<{
  timeline?: RunTimeline
  /** 整场跨度（毫秒）。传了就把每根条放在同一条轴上，跨角色可直接比较谁拖了后腿 */
  spanFrom?: number
  spanTo?: number
}>()

const steps = computed<RunStep[]>(() => props.timeline?.steps || [])
const chars = computed(() => props.timeline?.chars || 0)

const lo = computed(() => {
  if (props.spanFrom) return props.spanFrom
  if (!steps.value.length) return 0
  return Math.min(...steps.value.map((s) => s.at))
})
const hi = computed(() => {
  if (props.spanTo) return props.spanTo
  if (!steps.value.length) return 0
  return Math.max(...steps.value.map((s) => s.at + s.ms))
})
const span = computed(() => Math.max(1, hi.value - lo.value))

// 时间轴刻度：5 等分，标成相对起点的耗时。
// 没有刻度时每根条只能看出「相对长短」，看不出「第几秒发生的」。
const ticks = computed(() => {
  const out: { left: string; label: string }[] = []
  if (!steps.value.length) return out
  for (let i = 0; i <= 4; i++) {
    const frac = i / 4
    out.push({ left: `${frac * 100}%`, label: fmtMs(frac * span.value) })
  }
  return out
})

function leftOf(s: RunStep): string {
  return `${((s.at - lo.value) / span.value) * 100}%`
}
// 再快的动作也要看得见：给一个最小宽度，否则毫秒级调用会缩成没有。
function widthOf(s: RunStep): string {
  return `${Math.max(0.9, (s.ms / span.value) * 100)}%`
}
function fmtMs(ms: number): string {
  if (ms < 1000) return `${ms} 毫秒`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} 秒`
  const min = Math.floor(ms / 60000)
  const sec = Math.round((ms % 60000) / 1000)
  return sec ? `${min} 分 ${sec} 秒` : `${min} 分钟`
}
</script>

<template>
  <div v-if="steps.length || chars" class="tl">
    <div class="tl-sum">
      <span v-if="steps.length">{{ steps.length }} 个动作 · 合计 {{ fmtMs(hi - lo) }}</span>
      <span v-if="chars">正文 {{ chars.toLocaleString() }} 字</span>
    </div>
    <div class="tl-axis" aria-hidden="true">
    <span v-for="(t, i) in ticks" :key="i" class="tl-tick" :style="{ left: t.left }">{{ t.label }}</span>
  </div>
  <ul v-if="steps.length" class="tl-steps">
      <li v-for="(s, i) in steps" :key="i" class="tl-step" :title="s.detail || s.label || s.name">
        <span class="tl-track">
          <span class="tl-bar" :class="{ 'is-bad': !s.ok }" :style="{ left: leftOf(s), width: widthOf(s) }" />
        </span>
        <span class="tl-nm">{{ s.label || s.name }}</span>
        <span class="tl-ms mono">{{ fmtMs(s.ms) }}</span>
        <span class="tl-dot" :class="s.ok ? 'is-ok' : 'is-bad'" />
      </li>
    </ul>
  </div>
</template>

<style scoped>
.tl {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-3);
  padding-top: var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
}
.tl-sum {
  display: flex;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
/* 时间轴刻度：贴在轨道上方，随轨道同宽 */
.tl-axis {
  position: relative;
  height: 14px;
  margin-bottom: 2px;
}
.tl-tick {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-size: var(--wb-fs-2xs);
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  color: var(--wb-muted);
  white-space: nowrap;
}
/* 首尾两个刻度靠边对齐，避免探出容器被裁掉 */
.tl-tick:first-child {
  transform: none;
}
.tl-tick:last-child {
  transform: translateX(-100%);
}
.tl-steps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.tl-step {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 2px var(--wb-sp-1);
  border-radius: var(--wb-radius-sm);
  transition: background var(--wb-dur) var(--wb-ease);
}
.tl-step:hover {
  background: var(--wb-tint);
}
.tl-track {
  position: relative;
  flex: 1;
  min-width: 40px;
  height: 4px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-2);
}
.tl-bar {
  position: absolute;
  top: 0;
  height: 100%;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
}
.tl-bar.is-bad {
  background: var(--wb-danger);
}
.tl-nm {
  flex: none;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.tl-ms {
  flex: none;
  width: 62px;
  text-align: right;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.tl-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
}
.tl-dot.is-ok {
  background: var(--wb-success);
}
.tl-dot.is-bad {
  background: var(--wb-danger);
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
