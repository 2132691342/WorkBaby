<script setup lang="ts">
// 用量两图：按天的花费折线 + 按模型的 token 排行。
// 单独一个组件：echarts 实例生命周期（init / setOption / resize / dispose）
// 必须挂在画布自己的组件上，放在父组件里容易漏掉 dispose（实例泄漏）。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { SessionRow } from '@/types/api'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, CanvasRenderer])

const props = defineProps<{
  /** 时间窗：决定分桶粒度（今天按小时，其余按天） */
  span: 'today' | 'week' | 'month'
  /** 全部会话（本组件按 span 自己过滤） */
  rows: SessionRow[]
}>()

const { t } = useI18n()

const trendEl = ref<HTMLElement | null>(null)
const modelEl = ref<HTMLElement | null>(null)
let trendChart: echarts.ECharts | null = null
let modelChart: echarts.ECharts | null = null

const windowDays = computed(() =>
  props.span === 'today' ? 1 : props.span === 'week' ? 7 : 30,
)

/** 落在时间窗内的会话。 */
const scoped = computed(() => {
  const since = Date.now() - windowDays.value * 86_400_000
  return props.rows.filter((row) => {
    const time = new Date(row.updated_at).getTime()
    return Number.isNaN(time) ? true : time >= since
  })
})

/** 按天（或按小时）的花费分桶。 */
const byDay = computed(() => {
  const buckets: { label: string; cost: number }[] = []
  const now = new Date()
  if (props.span === 'today') {
    for (let i = 23; i >= 0; i--) {
      const hour = new Date(now)
      hour.setHours(now.getHours() - i, 0, 0, 0)
      buckets.push({ label: `${hour.getHours()}`, cost: 0 })
    }
  } else {
    for (let i = windowDays.value - 1; i >= 0; i--) {
      const day = new Date(now)
      day.setDate(now.getDate() - i)
      buckets.push({ label: `${day.getMonth() + 1}/${day.getDate()}`, cost: 0 })
    }
  }
  for (const row of scoped.value) {
    const time = new Date(row.updated_at)
    if (Number.isNaN(time.getTime())) continue
    const index =
      props.span === 'today'
        ? buckets.findIndex((_, i) => {
            const hour = new Date(now)
            hour.setHours(now.getHours() - (buckets.length - 1 - i), 0, 0, 0)
            return hour.getHours() === time.getHours()
          })
        : buckets.length - 1 - Math.floor((now.getTime() - time.getTime()) / 86_400_000)
    if (index < 0 || index >= buckets.length) continue
    buckets[index].cost += row.cost ?? 0
  }
  return buckets
})

/** 按模型的 token 合计，从大到小。 */
const byModel = computed(() => {
  const map = new Map<string, number>()
  for (const row of scoped.value) {
    const key = row.model || t('common.unknown')
    map.set(key, (map.get(key) ?? 0) + (row.tokens ?? 0))
  }
  return [...map.entries()]
    .map(([model, tokens]) => ({ model, tokens }))
    .sort((a, b) => b.tokens - a.tokens)
})

/** 主题色从 CSS 变量读：换主题时跟着变，不必在 JS 里维护第二份色表。 */
function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function render(): void {
  const axis = cssVar('--wb-faint')
  const line = cssVar('--wb-line')
  if (trendEl.value) {
    trendChart ??= echarts.init(trendEl.value)
    trendChart.setOption({
      animationDuration: 260,
      grid: { left: 46, right: 14, top: 16, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: {
        type: 'category',
        data: byDay.value.map((d) => d.label),
        axisLine: { lineStyle: { color: line } },
        axisLabel: { color: axis, fontSize: 10 },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: line } },
        axisLabel: { color: axis, fontSize: 10 },
      },
      series: [
        {
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: byDay.value.map((d) => Number(d.cost.toFixed(5))),
          lineStyle: { width: 2, color: cssVar('--wb-ch-1') },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: cssVar('--wb-brand-soft') },
              { offset: 1, color: 'transparent' },
            ]),
          },
        },
      ],
    })
  }
  if (modelEl.value) {
    modelChart ??= echarts.init(modelEl.value)
    const top = byModel.value.slice(0, 5)
    modelChart.setOption({
      animationDuration: 260,
      grid: { left: 8, right: 20, top: 8, bottom: 8, containLabel: true },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: line } },
        axisLabel: { color: axis, fontSize: 10 },
      },
      yAxis: {
        type: 'category',
        inverse: true,
        data: top.map((m) => m.model),
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: axis, fontSize: 10, width: 130, overflow: 'truncate' },
      },
      series: [
        {
          type: 'bar',
          barWidth: 11,
          data: top.map((m) => m.tokens),
          itemStyle: { color: cssVar('--wb-ch-2'), borderRadius: [0, 4, 4, 0] },
        },
      ],
    })
  }
}

function onResize(): void {
  trendChart?.resize()
  modelChart?.resize()
}

watch([() => props.span, byDay], () => render())

onMounted(render)
onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  trendChart?.dispose()
  modelChart?.dispose()
  trendChart = null
  modelChart = null
})
window.addEventListener('resize', onResize)
</script>

<template>
  <div class="chart-row">
    <section class="wb-card card-pad chart-card">
      <div class="wb-sec-head">
        <span class="wb-sec-title">{{ t('overview.byDay') }}</span>
      </div>
      <div ref="trendEl" class="chart" />
    </section>
    <section class="wb-card card-pad chart-card">
      <div class="wb-sec-head">
        <span class="wb-sec-title">{{ t('overview.byModel') }}</span>
      </div>
      <div v-if="byModel.length" ref="modelEl" class="chart" />
      <div v-else class="wb-empty">
        <p class="wb-empty-title">{{ t('overview.noUsage') }}</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.chart-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--wb-sp-4);
}

.chart-card {
  min-width: 0;
}

.chart {
  width: 100%;
  height: 200px;
}

.wb-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 200px;
}

.wb-empty-title {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}

@media (max-width: 1100px) {
  .chart-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
