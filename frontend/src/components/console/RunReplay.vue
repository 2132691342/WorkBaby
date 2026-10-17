<script setup lang="ts">
// 运行回放：把一次 run 的时间线按真实时间轴走一遍。
// 数据不新采——step 的 at / ms 本来就在运行记录里（specs/21 明确不做第二步快照），
// 回放只是让这些既有数字按时间重新呈现一次。
import { computed, onBeforeUnmount, ref } from 'vue'
import type { RunStep, RunTimeline } from '../../types/api'
import { fmtMs } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  timeline?: RunTimeline
}>()

const steps = computed<RunStep[]>(() => props.timeline?.steps || [])

// 轴的两端：从第一个动作的开始到最后一个动作的结束。
const lo = computed(() => (steps.value.length ? Math.min(...steps.value.map((s) => s.at)) : 0))
const hi = computed(() => (steps.value.length ? Math.max(...steps.value.map((s) => s.at + s.ms)) : 0))
const span = computed(() => Math.max(1, hi.value - lo.value))

// 游标位置（毫秒，相对 lo）。播放时按倍速推进，暂停时停住。
const cursor = ref(0)
const playing = ref(false)
const speed = ref(1)
const SPEEDS = [0.5, 1, 2, 4]

// 当前进行到第几步：开始时间已过、结束时间未过的那个。
const activeIndex = computed(() => {
  const t = lo.value + cursor.value
  let idx = -1
  steps.value.forEach((s, i) => {
    if (s.at <= t && t < s.at + Math.max(1, s.ms)) idx = i
  })
  return idx
})
// 已经跑完的步数：进度条与「第几步 / 共几步」都用它。
const doneCount = computed(() => {
  const t = lo.value + cursor.value
  return steps.value.filter((s) => s.at + s.ms <= t).length
})

let raf = 0
let last = 0
function tick(now: number) {
  if (!playing.value) return
  // 首帧没有上一帧时间：直接记为起点，否则第一帧会跳一大截。
  if (!last) last = now
  cursor.value += (now - last) * speed.value
  last = now
  if (cursor.value >= span.value) {
    cursor.value = span.value
    pause()
    return
  }
  raf = requestAnimationFrame(tick)
}
function play() {
  if (!steps.value.length) return
  // 播完了再点播放 = 从头来：游标已在末端时不重置，用户会以为按钮失灵。
  if (cursor.value >= span.value) cursor.value = 0
  playing.value = true
  last = 0
  raf = requestAnimationFrame(tick)
}
function pause() {
  playing.value = false
  if (raf) cancelAnimationFrame(raf)
  raf = 0
}
function toggle() {
  playing.value ? pause() : play()
}
// 单步：跳到下一个动作的开始，或到末端。
function stepOnce() {
  pause()
  const t = lo.value + cursor.value
  const next = steps.value.find((s) => s.at > t + 1)
  cursor.value = next ? next.at - lo.value : span.value
}
function seek(frac: number) {
  pause()
  cursor.value = Math.max(0, Math.min(span.value, frac * span.value))
}
onBeforeUnmount(pause)

const pct = computed(() => (cursor.value / span.value) * 100)
const barLeft = (s: RunStep) => ((s.at - lo.value) / span.value) * 100
const barWidth = (s: RunStep) => Math.max(0.9, (s.ms / span.value) * 100)
</script>

<template>
  <div v-if="steps.length" class="rp">
    <div class="rp-bar">
      <button
        class="btn btn-sm btn-primary rp-play"
        type="button"
        :disabled="!steps.length"
        :aria-label="playing ? '暂停' : '播放'"
        @click="toggle"
      >
        <AppIcon :name="playing ? 'pause' : 'play'" size="ic-xs" />
      </button>
      <button class="btn btn-sm rp-step" type="button" :disabled="!steps.length" @click="stepOnce">
        单步
      </button>
      <div class="rp-speeds" role="group" aria-label="播放倍速">
        <button
          v-for="s in SPEEDS"
          :key="s"
          type="button"
          :class="{ on: speed === s }"
          @click="speed = s"
        >
          {{ s }}×
        </button>
      </div>
      <span class="sp" />
      <span class="rp-clock mono">{{ fmtMs(cursor) }} / {{ fmtMs(span) }}</span>
      <span class="rp-count dim">第 {{ Math.min(doneCount + 1, steps.length) }} / {{ steps.length }} 步</span>
    </div>

    <!-- 进度条：整条可点，点到哪游标到哪 -->
    <div
      class="rp-track"
      role="slider"
      :aria-valuenow="Math.round(pct)"
      aria-valuemin="0"
      aria-valuemax="100"
      tabindex="0"
      @click.self="seek(($event as MouseEvent).offsetX / ($event.target as HTMLElement).clientWidth)"
    >
      <span class="rp-fill" :style="{ width: pct + '%' }" />
      <span class="rp-head" :style="{ left: pct + '%' }" />
    </div>

    <ul class="rp-steps">
      <li
        v-for="(s, i) in steps"
        :key="i"
        class="rp-step-row"
        :class="{ 'is-active': i === activeIndex, 'is-done': s.at + s.ms <= lo + cursor, 'is-bad': !s.ok }"
        :title="s.detail || s.label || s.name"
      >
        <span class="rp-row-track">
          <span class="rp-row-bar" :style="{ left: barLeft(s) + '%', width: barWidth(s) + '%' }" />
        </span>
        <span class="rp-nm">{{ s.label || s.name }}</span>
        <span class="rp-ms mono">{{ fmtMs(s.ms) }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.rp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.rp-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.rp-bar .sp {
  flex: 1;
}
.rp-clock,
.rp-count {
  flex: none;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.rp-speeds {
  display: flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-2);
}
.rp-speeds button {
  min-width: 30px;
  height: 20px;
  border: 0;
  border-radius: var(--wb-radius-full);
  background: transparent;
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease);
}
.rp-speeds button:hover {
  color: var(--wb-ink);
}
.rp-speeds button.on {
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
}
/* 进度条：细、可点、有游标头 */
.rp-track {
  position: relative;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-2);
  cursor: pointer;
}
.rp-fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary-soft);
}
.rp-head {
  position: absolute;
  top: 50%;
  width: 10px;
  height: 10px;
  margin-left: -5px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  transform: translateY(-50%);
  transition: left 60ms linear;
}
.rp-steps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.rp-step-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 2px var(--wb-sp-1);
  border-radius: var(--wb-radius-sm);
  opacity: 0.55;
  transition:
    background var(--wb-dur) var(--wb-ease),
    opacity var(--wb-dur) var(--wb-ease);
}
.rp-step-row.is-done {
  opacity: 1;
}
.rp-step-row.is-active {
  background: var(--wb-primary-soft);
  opacity: 1;
}
.rp-row-track {
  position: relative;
  flex: 1;
  min-width: 40px;
  height: 4px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-2);
}
.rp-row-bar {
  position: absolute;
  top: 0;
  height: 100%;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
}
.rp-step-row.is-bad .rp-row-bar {
  background: var(--wb-danger);
}
.rp-nm {
  flex: none;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.rp-ms {
  flex: none;
  width: 62px;
  text-align: right;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
.dim {
  color: var(--wb-muted);
}
</style>
