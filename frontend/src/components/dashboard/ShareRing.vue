<script setup lang="ts">
// 按模型占比的圆环图（ECharts）：色值取 --wb-ch-1..4，环心放总量。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { StatsModelItem } from '../../types/api'
import { fmtCount } from '../../utils/num'

echarts.use([PieChart, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps<{ items: StatsModelItem[]; limit?: number }>()

const host = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#6366f1'
}

const rows = computed(() => {
  const list = [...props.items].sort((a, b) => (b.total || 0) - (a.total || 0))
  const sum = list.reduce((n, m) => n + (m.total || 0), 0)
  if (!sum) return []
  const top = list.slice(0, props.limit || 4)
  const rest = list.slice(top.length)
  const restTotal = rest.reduce((n, m) => n + (m.total || 0), 0)
  const out = top.map((m) => ({ key: m.model, model: m.model, total: m.total, calls: m.calls }))
  if (rest.length) {
    out.push({ key: '__rest__', model: `其他 ${rest.length} 个模型`, total: restTotal, calls: 0 })
  }
  return out
})

const total = computed(() => props.items.reduce((n, m) => n + (m.total || 0), 0))

function render() {
  if (!chart) return
  const palette = ['--wb-ch-1', '--wb-ch-2', '--wb-ch-3', '--wb-ch-4'].map((k) => cssVar(k))
  const surface = cssVar('--wb-surface-solid')
  const border = cssVar('--wb-border-strong')
  const ink2 = cssVar('--wb-ink-2')
  chart.setOption(
    {
      color: palette,
      tooltip: {
        trigger: 'item',
        backgroundColor: surface,
        borderColor: border,
        borderWidth: 1,
        textStyle: { color: ink2, fontSize: 11 },
        formatter: (p: { name: string; value: number; percent: number }) =>
          `${p.name}<br/>${fmtCount(p.value)} token · ${p.percent}%`,
      },
      series: [
        {
          type: 'pie',
          radius: ['62%', '88%'],
          center: ['50%', '50%'],
          padAngle: 2,
          itemStyle: { borderRadius: 4, borderColor: surface, borderWidth: 2 },
          label: { show: false },
          emphasis: { scaleSize: 4 },
          data: rows.value.map((r) => ({ name: r.model, value: r.total })),
        },
      ],
      graphic: [
        {
          type: 'text',
          left: 'center',
          top: '42%',
          style: {
            text: fmtCount(total.value),
            textAlign: 'center',
            fill: cssVar('--wb-ink'),
            fontSize: 16,
            fontWeight: 700,
            fontFamily: 'Bahnschrift, sans-serif',
          },
        },
        {
          type: 'text',
          left: 'center',
          top: '56%',
          style: {
            text: 'token',
            textAlign: 'center',
            fill: cssVar('--wb-muted'),
            fontSize: 10,
            fontFamily: 'Bahnschrift, sans-serif',
          },
        },
      ],
    },
    { notMerge: true },
  )
}

onMounted(() => {
  if (!host.value) return
  chart = echarts.init(host.value)
  render()
  const ob = new MutationObserver(render)
  ob.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  onBeforeUnmount(() => ob.disconnect())
})

watch(rows, render, { deep: true })

onBeforeUnmount(() => {
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div v-if="rows.length" class="ring-wrap">
    <div ref="host" class="echart" role="img" aria-label="按模型的 token 占比" />
    <ul class="legend">
      <li v-for="(r, i) in rows" :key="r.key">
        <i class="sw" :style="{ background: `var(--wb-ch-${Math.min(i, 3) + 1})` }" />
        <span class="nm" :title="r.model">{{ r.model }}</span>
        <span class="pct">{{ r.total ? Math.round((r.total / Math.max(1, total)) * 100) : 0 }}%</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.ring-wrap {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-5);
}
.echart {
  width: 148px;
  height: 148px;
  flex: none;
}
.legend {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  list-style: none;
  margin: 0;
  padding: 0;
}
.legend li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.sw {
  width: 9px;
  height: 9px;
  border-radius: var(--wb-radius-full);
  flex: none;
}
.legend .nm {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.legend .pct {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
</style>
