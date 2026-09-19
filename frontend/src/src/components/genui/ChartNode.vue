<script setup lang="ts">
/**
 * GenUI chart 节点真渲染。
 *
 * <p>动态 import ECharts（core + line/bar/pie + Canvas），chart 节点挂载时才加载，
 * 避免拖大首屏包（「体积大 → 动态 import」）。
 */
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  type?: string
  data?: unknown
  labels?: unknown
}>()

type ECharts = { setOption(o: unknown): void; dispose(): void }
const el = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null
let initPromise: Promise<ECharts> | null = null

function values(): number[] {
  const d = props.data
  if (Array.isArray(d)) {
    return d.map((v) => (typeof v === 'number' ? v : Number(v) || 0))
  }
  return []
}

function labels(): string[] {
  const l = props.labels
  if (Array.isArray(l)) return l.map((v) => String(v))
  return values().map((_, i) => String(i))
}

function option(): Record<string, unknown> {
  const t = props.type ?? 'line'
  const vs = values()
  if (t === 'pie') {
    return {
      tooltip: { trigger: 'item' },
      series: [{ type: 'pie', radius: '70%', data: vs.map((v, i) => ({ name: labels()[i], value: v })) }]
    }
  }
  const catAxis = { type: 'category', data: labels() }
  const valAxis = { type: 'value' }
  if (t === 'bar') {
    return { tooltip: { trigger: 'axis' }, xAxis: catAxis, yAxis: valAxis, series: [{ type: 'bar', data: vs }] }
  }
  return { tooltip: { trigger: 'axis' }, xAxis: catAxis, yAxis: valAxis, series: [{ type: 'line', data: vs }] }
}

function ensureInit(): Promise<ECharts> {
  if (chart) return Promise.resolve(chart)
  if (!initPromise) {
    initPromise = (async () => {
      const core = await import('echarts/core')
      const { LineChart, BarChart, PieChart } = await import('echarts/charts')
      const { GridComponent, TooltipComponent, LegendComponent } = await import('echarts/components')
      const { CanvasRenderer } = await import('echarts/renderers')
      core.use([LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])
      chart = core.init(el.value!) as unknown as ECharts
      return chart
    })()
  }
  return initPromise
}

async function render(): Promise<void> {
  if (!el.value) return
  const c = await ensureInit()
  c.setOption(option())
}

onMounted(render)
watch(() => [props.type, props.data, props.labels], render)
onBeforeUnmount(() => {
  if (chart) {
    chart.dispose()
    chart = null
    initPromise = null
  }
})
</script>

<template>
  <div ref="el" class="h-48 w-full" />
</template>
