<script setup lang="ts">
// 按模型占比：环形图 + 右侧图例列表（颜色点 + 模型名 + 占比 + token 量）。
// 最多五段，其余归「其他」——超过五段就没有人能分辨颜色了。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { theme } from '../../composables/useTheme'
import type { StatsModelItem } from '../../types/api'
import { chartBase, chartPalette, echarts, tooltipStyle } from '../../utils/echarts'
import { fmtCount } from '../../utils/num'

const props = defineProps<{ models: StatsModelItem[] }>()
const { t } = useI18n()

const host = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null
let ro: ResizeObserver | null = null

const MAX_SEG = 5

const segments = computed(() => {
  const list = [...props.models].sort((a, b) => b.total - a.total)
  const top = list.slice(0, MAX_SEG)
  const rest = list.slice(MAX_SEG)
  const out = top.map((m) => ({ name: m.model || m.provider, value: m.total }))
  if (rest.length) {
    out.push({ name: t('dashboard.other'), value: rest.reduce((s, m) => s + m.total, 0) })
  }
  return out
})

function buildOption(): echarts.EChartsCoreOption {
  const base = chartBase()
  const pal = chartPalette()
  return {
    animationDuration: 320,
    tooltip: {
      trigger: 'item',
      ...tooltipStyle(),
      formatter: (p: { name: string; value: number; percent: number }) =>
        `${p.name}<br/>${fmtCount(p.value)} · ${p.percent}%`,
    },
    color: pal,
    series: [
      {
        type: 'pie',
        radius: ['58%', '82%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        padAngle: 2,
        itemStyle: { borderRadius: 6, borderColor: base.surface, borderWidth: 2 },
        label: { show: false },
        labelLine: { show: false },
        emphasis: { scale: true, scaleSize: 4 },
        data: segments.value,
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

watch(segments, render, { deep: true })
watch(theme, () => window.requestAnimationFrame(render))

const total = computed(() => segments.value.reduce((s, x) => s + x.value, 0))
const palette = computed(() => chartPalette())
</script>

<template>
  <div class="share">
    <div ref="host" class="share-chart" />
    <ul class="share-list">
      <li v-for="(s, i) in segments" :key="s.name" class="share-row">
        <span class="share-dot" :style="{ background: palette[i % palette.length] }" />
        <span class="share-name" :title="s.name">{{ s.name }}</span>
        <span class="share-val">{{ fmtCount(s.value) }}</span>
        <span class="share-pct">{{ total ? Math.round((s.value / total) * 100) : 0 }}%</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.share {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  min-height: 0;
}
.share-chart {
  width: calc(160px * var(--wb-vw-scale));
  height: calc(160px * var(--wb-vw-scale));
  flex: none;
}
.share-list {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.share-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
}
.share-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--wb-radius-xs);
  flex: none;
}
.share-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink-2);
  font-weight: 600;
}
.share-val {
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  flex: none;
}
.share-pct {
  width: calc(38px * var(--wb-vw-scale));
  text-align: right;
  color: var(--wb-ink);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  flex: none;
}
</style>
