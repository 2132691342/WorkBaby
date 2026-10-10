<script setup lang="ts">
// 近 N 天 token 趋势折线图：SVG 自绘，不引任何图表库。
// 折线而不是柱：读数要回答的是「趋势往哪走」，柱形在天数一多时只能挤成一片色块。
// 两条序列各自带一层面积：输入是成本主体，输出是产出，叠着色一眼分得开。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { StatsDailyItem } from '../../types/api'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ daily: StatsDailyItem[] }>()

// 绘图区尺寸：宽由容器实测（ResizeObserver），高固定——趋势图的纵向刻度不需要跟着窗口变。
const H = 236
const PAD = { top: 16, right: 14, bottom: 26, left: 48 }
const Y_GRID = 4

const host = ref<HTMLElement | null>(null)
const width = ref(720)
let ro: ResizeObserver | null = null

onMounted(() => {
  const el = host.value
  if (!el) return
  ro = new ResizeObserver((entries) => {
    const w = Math.round(entries[0]?.contentRect.width || 0)
    if (w > 0) width.value = w
  })
  ro.observe(el)
})
onBeforeUnmount(() => ro?.disconnect())

const count = computed(() => props.daily.length)
const plotW = computed(() => Math.max(96, width.value - PAD.left - PAD.right))
const plotH = H - PAD.top - PAD.bottom

// 上限取到「好读的刻度」：直接用量峰值会让 y 轴标签出现 9182 这种数字
const maxValue = computed(() => {
  let peak = 0
  for (const d of props.daily) peak = Math.max(peak, d.input || 0, d.output || 0)
  return niceCeil(Math.max(1, peak))
})

function niceCeil(v: number): number {
  const pow = Math.pow(10, Math.floor(Math.log10(v)))
  const n = v / pow
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10
  return step * pow
}

const px = (i: number) =>
  count.value <= 1 ? PAD.left + plotW.value / 2 : PAD.left + (plotW.value * i) / (count.value - 1)
const py = (v: number) =>
  PAD.top + plotH - (plotH * Math.min(Math.max(v || 0, 0), maxValue.value)) / maxValue.value

const pointsIn = computed(() => pointsFor((d) => d.input || 0))
const pointsOut = computed(() => pointsFor((d) => d.output || 0))

function pointsFor(pick: (d: StatsDailyItem) => number): string {
  return props.daily.map((d, i) => `${px(i).toFixed(1)},${py(pick(d)).toFixed(1)}`).join(' ')
}

// 面积 = 折线两端垂到基线闭合；用 point 串直接拼，省掉一套 path builder
function areaOf(line: string): string {
  if (!line) return ''
  const parts = line.split(' ')
  const first = parts[0].split(',')[0]
  const last = parts[parts.length - 1].split(',')[0]
  const base = (PAD.top + plotH).toFixed(1)
  return `${first},${base} ${line} ${last},${base}`
}

const areaIn = computed(() => areaOf(pointsIn.value))
const areaOut = computed(() => areaOf(pointsOut.value))

const yTicks = computed(() =>
  Array.from({ length: Y_GRID + 1 }, (_, i) => {
    const v = (maxValue.value * i) / Y_GRID
    return { v, y: py(v), label: fmtCount(v) }
  }),
)

// x 轴只标稀疏刻度：天数一多就每 2~5 天一个，全标必然糊成一条黑杠
const xTicks = computed(() => {
  const step = count.value <= 7 ? 1 : count.value <= 14 ? 2 : 5
  const out: { key: string; x: number; label: string }[] = []
  props.daily.forEach((d, i) => {
    const show = i === 0 || i === count.value - 1 || (i + 1) % step === 0
    if (!show) return
    out.push({ key: d.date || String(i), x: px(i), label: (d.date || '').slice(5).replace('-', '/') })
  })
  return out
})

// 悬浮：只取最近的一个点，不做多轴联动——趋势图上读单日量级已经够了
const hoverI = ref<number | null>(null)

function onMove(e: MouseEvent) {
  const rect = (e.currentTarget as SVGRectElement).getBoundingClientRect()
  const rel = e.clientX - rect.left
  if (count.value <= 1) {
    hoverI.value = 0
    return
  }
  const ratio = (rel - PAD.left) / plotW.value
  hoverI.value = Math.max(0, Math.min(count.value - 1, Math.round(ratio * (count.value - 1))))
}

const hover = computed(() => {
  const i = hoverI.value
  if (i === null) return null
  const d = props.daily[i]
  if (!d) return null
  const cx = px(i)
  // 提示条贴边展开：首尾两天居中会伸出卡片被裁掉
  const near = cx < PAD.left + 60 ? 'left' : cx > PAD.left + plotW.value - 60 ? 'right' : 'mid'
  return {
    d,
    cx,
    yi: py(d.input || 0),
    yo: py(d.output || 0),
    near,
    text: `${d.date}：输入 ${fmtCount(d.input)} · 输出 ${fmtCount(d.output)} · 合计 ${fmtCount(d.total)}`,
  }
})
</script>

<template>
  <div ref="host" class="plot" @mouseleave="hoverI = null">
    <svg :width="width" :height="H" role="img" aria-label="近几天 token 用量趋势">
      <defs>
        <!-- 面积渐变：从序列色淡出到透明，避免两条实色块互压 -->
        <linearGradient id="wbAreaIn" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" class="g-in" stop-opacity="0.22" />
          <stop offset="100%" class="g-in" stop-opacity="0" />
        </linearGradient>
        <linearGradient id="wbAreaOut" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" class="g-out" stop-opacity="0.2" />
          <stop offset="100%" class="g-out" stop-opacity="0" />
        </linearGradient>
      </defs>

      <!-- 网格与 y 轴刻度 -->
      <g class="grid">
        <line
          v-for="t in yTicks"
          :key="`g${t.v}`"
          :x1="PAD.left"
          :x2="PAD.left + plotW"
          :y1="t.y"
          :y2="t.y"
        />
      </g>
      <g class="ylab">
        <text v-for="t in yTicks" :key="`y${t.v}`" :x="PAD.left - 10" :y="t.y + 4" text-anchor="end">
          {{ t.label }}
        </text>
      </g>

      <!-- 面积 + 折线 -->
      <polygon class="area-in" :points="areaIn" />
      <polygon class="area-out" :points="areaOut" />
      <polyline class="line-in" :points="pointsIn" />
      <polyline class="line-out" :points="pointsOut" />

      <!-- x 轴标签 -->
      <g class="xlab">
        <text v-for="t in xTicks" :key="t.key" :x="t.x" :y="H - 8" text-anchor="middle">
          {{ t.label }}
        </text>
      </g>

      <!-- 悬浮：竖线 + 两个数据点 -->
      <g v-if="hover" class="cursor">
        <line :x1="hover.cx" :x2="hover.cx" :y1="PAD.top" :y2="PAD.top + plotH" />
        <circle class="dot-in" :cx="hover.cx" :cy="hover.yi" r="3.4" />
        <circle class="dot-out" :cx="hover.cx" :cy="hover.yo" r="3.4" />
      </g>

      <rect
        class="hit"
        :x="PAD.left"
        :y="PAD.top"
        :width="plotW"
        :height="plotH"
        @mousemove="onMove"
      />
    </svg>

    <div v-if="hover" class="tip" :class="hover.near" :style="{ left: `${hover.cx}px` }">
      {{ hover.text }}
    </div>
  </div>
</template>

<style scoped>
.plot {
  position: relative;
  width: 100%;
  min-width: 0;
}
svg {
  display: block;
  width: 100%;
  overflow: visible;
}
.grid line {
  stroke: var(--wb-line);
  stroke-width: 1;
}
.ylab text,
.xlab text {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  fill: var(--wb-muted);
}
.area-in {
  fill: url(#wbAreaIn);
}
.area-out {
  fill: url(#wbAreaOut);
}
/* 渐变端点色跟着主题：浅色是深紫 / 亮紫，暗色换成浅紫 / 淡紫 */
.g-in {
  stop-color: var(--wb-primary);
}
.g-out {
  stop-color: var(--wb-ch-2);
}
.line-in,
.line-out {
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.line-in {
  stroke: var(--wb-ch-1);
}
.line-out {
  stroke: var(--wb-ch-2);
}
.cursor line {
  stroke: var(--wb-line-2);
  stroke-width: 1;
  stroke-dasharray: 3 3;
}
.cursor circle {
  stroke: var(--wb-surface-solid);
  stroke-width: 1.6;
}
.dot-in {
  fill: var(--wb-primary);
}
.dot-out {
  fill: var(--wb-ch-2);
}
.hit {
  fill: transparent;
  cursor: crosshair;
}
.tip {
  position: absolute;
  top: 4px;
  transform: translateX(-50%);
  z-index: var(--wb-z-pop);
  padding: 5px var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface-solid);
  border: 1px solid var(--wb-border-strong);
  box-shadow: var(--wb-shadow-pop);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: nowrap;
  pointer-events: none;
}
.tip.left {
  transform: none;
}
.tip.right {
  transform: translateX(-100%);
}
</style>
