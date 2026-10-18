<script setup lang="ts">
// 近 N 天 token 趋势图（ECharts）：输入与输出两条平滑面积线，主题色接 --wb-ch-*。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { StatsDailyItem } from '../../types/api'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{ daily: StatsDailyItem[] }>()

const host = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null
let ro: ResizeObserver | null = null

// 主题色从 CSS 变量读：浅暗两套自动跟随（ECharts 不认 var()，只能运行时取计算值）
function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#6366f1'
}

function buildOption(): echarts.EChartsCoreOption {
  const dates = props.daily.map((d) => (d.date || '').slice(5).replace('-', '/'))
  const input = props.daily.map((d) => d.input || 0)
  const output = props.daily.map((d) => d.output || 0)
  const ink2 = cssVar('--wb-ink-2')
  const muted = cssVar('--wb-muted')
  const line = cssVar('--wb-line')
  const surface = cssVar('--wb-surface-solid')
  const border = cssVar('--wb-border-strong')
  return {
    animationDuration: 300,
    grid: { top: 28, right: 16, bottom: 26, left: 52, containLabel: false },
    legend: {
      top: 0,
      right: 0,
      icon: 'roundRect',
      itemWidth: 14,
      itemHeight: 4,
      textStyle: { color: ink2, fontSize: 11 },
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: surface,
      borderColor: border,
      borderWidth: 1,
      textStyle: { color: ink2, fontSize: 11, fontFamily: 'Cascadia Mono, Consolas, monospace' },
      valueFormatter: (v: unknown) => Number(v).toLocaleString(),
    },
    xAxis: {
      type: 'category',
      data: dates,
      boundaryGap: false,
      axisLine: { lineStyle: { color: line } },
      axisTick: { show: false },
      axisLabel: { color: muted, fontSize: 10, fontFamily: 'Cascadia Mono, Consolas, monospace' },
    },
    yAxis: {
      type: 'value',
      splitNumber: 4,
      axisLabel: {
        color: muted,
        fontSize: 10,
        formatter: (v: number) => fmtAxis(v),
      },
      splitLine: { lineStyle: { color: line } },
    },
    series: [
      {
        name: '输入',
        type: 'line',
        data: input,
        smooth: 0.35,
        symbol: 'circle',
        symbolSize: 6,
        showSymbol: false,
        lineStyle: { width: 3, color: cssVar('--wb-ch-1') },
        itemStyle: { color: cssVar('--wb-ch-1'), borderColor: surface, borderWidth: 1.5 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: withAlpha(cssVar('--wb-ch-1'), 0.22) },
            { offset: 1, color: withAlpha(cssVar('--wb-ch-1'), 0) },
          ]),
        },
      },
      {
        name: '输出',
        type: 'line',
        data: output,
        smooth: 0.35,
        symbol: 'circle',
        symbolSize: 6,
        showSymbol: false,
        lineStyle: { width: 3, color: cssVar('--wb-ch-2') },
        itemStyle: { color: cssVar('--wb-ch-2'), borderColor: surface, borderWidth: 1.5 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: withAlpha(cssVar('--wb-ch-2'), 0.2) },
            { offset: 1, color: withAlpha(cssVar('--wb-ch-2'), 0) },
          ]),
        },
      },
    ],
  }
}

// y 轴刻度用短格式：12345 → 12.3k
function fmtAxis(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1000) return `${(v / 1000).toFixed(v >= 10_000 ? 0 : 1)}k`
  return String(v)
}

// hex → rgba：ECharts 渐变不接受 CSS var，也不接受 8 位 hex 的所有渲染路径
function withAlpha(hex: string, alpha: number): string {
  const m = hex.replace('#', '')
  if (m.length !== 6) return hex
  const r = parseInt(m.slice(0, 2), 16)
  const g = parseInt(m.slice(2, 4), 16)
  const b = parseInt(m.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function render() {
  if (!chart) return
  chart.setOption(buildOption(), { notMerge: true })
}

onMounted(() => {
  if (!host.value) return
  chart = echarts.init(host.value)
  render()
  ro = new ResizeObserver(() => chart?.resize())
  ro.observe(host.value)
  // 主题切换后重取 CSS 变量重绘
  const ob = new MutationObserver(render)
  ob.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  onBeforeUnmount(() => ob.disconnect())
})

watch(() => props.daily, render, { deep: true })

onBeforeUnmount(() => {
  ro?.disconnect()
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div ref="host" class="echart" role="img" aria-label="近几天 token 用量趋势" />
</template>

<style scoped>
.echart {
  width: 100%;
  height: calc(260px * var(--wb-vw-scale));
  min-width: 0;
}
</style>
