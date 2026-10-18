<script setup lang="ts">
// 用量趋势：按天的输入 / 输出双面积线。主题切换后重读 --wb-* 重绘。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { theme } from '../../composables/useTheme'
import type { StatsDailyItem } from '../../types/api'
import { chartBase, chartPalette, cssVar, echarts, tooltipStyle } from '../../utils/echarts'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ daily: StatsDailyItem[] }>()
const { t } = useI18n()

const host = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null
let ro: ResizeObserver | null = null

function buildOption(): echarts.EChartsCoreOption {
  const base = chartBase()
  const pal = chartPalette()
  const dates = props.daily.map((d) => (d.date || '').slice(5).replace('-', '/'))
  const input = props.daily.map((d) => d.input || 0)
  const output = props.daily.map((d) => d.output || 0)
  const from = cssVar('--wb-area-from')
  const to = cssVar('--wb-area-to')

  const area = (color: string) => ({
    color: {
      type: 'linear',
      x: 0,
      y: 0,
      x2: 0,
      y2: 1,
      colorStops: [
        { offset: 0, color: from },
        { offset: 1, color: to },
      ],
    },
    // 第二条序列叠淡一点，避免两条渐变互相盖住
    opacity: color === pal[0] ? 1 : 0.75,
  })

  return {
    animationDuration: 320,
    grid: { top: 34, right: 12, bottom: 26, left: 46, containLabel: true },
    legend: {
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 4,
      itemGap: 14,
      textStyle: { color: base.muted, fontSize: 11, fontWeight: 600 },
    },
    tooltip: {
      trigger: 'axis',
      ...tooltipStyle(),
      axisPointer: { type: 'line', lineStyle: { color: base.strong, width: 1, type: 'dashed' } },
      valueFormatter: (v: number) => fmtCount(v),
    },
    xAxis: {
      type: 'category',
      data: dates,
      boundaryGap: false,
      axisLine: { lineStyle: { color: base.line } },
      axisTick: { show: false },
      axisLabel: { color: base.muted, fontSize: 10, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      splitNumber: 4,
      axisLine: { show: false },
      axisTick: { show: false },
      splitLine: { lineStyle: { color: base.line, type: 'dashed' } },
      axisLabel: { color: base.muted, fontSize: 10, formatter: (v: number) => fmtCount(v) },
    },
    series: [
      {
        name: t('dashboard.input'),
        type: 'line',
        smooth: 0.3,
        symbol: 'circle',
        symbolSize: 0,
        showSymbol: false,
        lineStyle: { width: 2.4, color: pal[0] },
        itemStyle: { color: pal[0] },
        areaStyle: area(pal[0]),
        data: input,
      },
      {
        name: t('dashboard.output'),
        type: 'line',
        smooth: 0.3,
        symbol: 'circle',
        symbolSize: 0,
        showSymbol: false,
        lineStyle: { width: 2, color: pal[1] },
        itemStyle: { color: pal[1] },
        areaStyle: area(pal[1]),
        data: output,
      },
    ],
  }
}

function render() {
  if (!chart) return
  chart.setOption(buildOption(), true)
}

onMounted(() => {
  if (!host.value) return
  chart = echarts.init(host.value, undefined, { renderer: 'canvas' })
  render()
  ro = new ResizeObserver(() => chart?.resize())
  ro.observe(host.value)
})

onBeforeUnmount(() => {
  ro?.disconnect()
  chart?.dispose()
  chart = null
})

watch(() => props.daily, render, { deep: true })
watch(theme, () => window.requestAnimationFrame(render))
</script>

<template>
  <div ref="host" class="trend" />
</template>

<style scoped>
.trend {
  width: 100%;
  height: 100%;
  min-height: calc(220px * var(--wb-vw-scale));
}
</style>
