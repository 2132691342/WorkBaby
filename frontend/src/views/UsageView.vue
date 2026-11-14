<script setup lang="ts">
// 用量页：数字全部来自引擎统计的会话文件（通过 sessions 列表聚合），不另存一份。
// 图只画两处：按天趋势与按模型占比——都读 CSS 变量，主题切换后重绘。
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as echarts from 'echarts/core'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import AppIcon from '@/components/common/AppIcon.vue'
import WbSeg from '@/components/common/WbSeg.vue'
import type { SegItem } from '@/types/ui'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { SessionRow } from '@/types/api'

echarts.use([LineChart, BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const { t } = useI18n()
const toasts = useToasts()
const app = useAppStore()

const rows = ref<SessionRow[]>([])
const loading = ref(false)
const span = ref<'today' | 'week' | 'month' | 'total'>('week')

/** 时间窗分段：文案走 i18n，值就是 span 本身。 */
const spanItems = computed<SegItem[]>(() =>
  (['today', 'week', 'month', 'total'] as const).map((key) => ({ value: key, label: t(`usage.${key}`) })),
)
const trendEl = ref<HTMLElement | null>(null)
const modelEl = ref<HTMLElement | null>(null)
let trendChart: echarts.ECharts | null = null
let modelChart: echarts.ECharts | null = null

const windowDays = computed(() => (span.value === 'today' ? 1 : span.value === 'week' ? 7 : span.value === 'month' ? 30 : 3650))

const scoped = computed(() => {
  const since = Date.now() - windowDays.value * 86_400_000
  return rows.value.filter((row) => {
    const time = new Date(row.updated_at).getTime()
    return Number.isNaN(time) ? true : time >= since
  })
})

const totals = computed(() => {
  let tokens = 0
  let cost = 0
  for (const row of scoped.value) {
    tokens += row.total_tokens ?? 0
    cost += row.cost_usd ?? 0
  }
  return { tokens, cost, sessions: scoped.value.length }
})

const byModel = computed(() => {
  const map = new Map<string, { tokens: number; cost: number; sessions: number }>()
  for (const row of scoped.value) {
    const key = row.session_model || t('common.unknown')
    const entry = map.get(key) ?? { tokens: 0, cost: 0, sessions: 0 }
    entry.tokens += row.total_tokens ?? 0
    entry.cost += row.cost_usd ?? 0
    entry.sessions += 1
    map.set(key, entry)
  }
  return [...map.entries()]
    .map(([model, value]) => ({ model, ...value }))
    .sort((a, b) => b.cost - a.cost)
})

const byDay = computed(() => {
  const days = span.value === 'today' ? 24 : windowDays.value
  const buckets: { label: string; cost: number; tokens: number }[] = []
  const now = new Date()
  if (span.value === 'today') {
    for (let i = 23; i >= 0; i--) {
      const hour = new Date(now)
      hour.setHours(now.getHours() - i, 0, 0, 0)
      buckets.push({ label: `${hour.getHours()}`, cost: 0, tokens: 0 })
    }
  } else {
    for (let i = days - 1; i >= 0; i--) {
      const day = new Date(now)
      day.setDate(now.getDate() - i)
      buckets.push({ label: `${day.getMonth() + 1}/${day.getDate()}`, cost: 0, tokens: 0 })
    }
  }
  for (const row of scoped.value) {
    const time = new Date(row.updated_at)
    if (Number.isNaN(time.getTime())) continue
    const index =
      span.value === 'today'
        ? buckets.findIndex((_, i) => {
            const hour = new Date(now)
            hour.setHours(now.getHours() - (buckets.length - 1 - i), 0, 0, 0)
            return hour.getHours() === time.getHours()
          })
        : buckets.length - 1 - Math.floor((now.getTime() - time.getTime()) / 86_400_000)
    if (index < 0 || index >= buckets.length) continue
    buckets[index].cost += row.cost_usd ?? 0
    buckets[index].tokens += row.total_tokens ?? 0
  }
  return buckets
})

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function renderCharts() {
  const axis = cssVar('--wb-faint')
  const line = cssVar('--wb-line')
  if (trendEl.value) {
    trendChart ??= echarts.init(trendEl.value)
    trendChart.setOption({
      animationDuration: 260,
      grid: { left: 44, right: 16, top: 20, bottom: 26 },
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
    const top = byModel.value.slice(0, 6)
    modelChart.setOption({
      animationDuration: 260,
      grid: { left: 8, right: 24, top: 8, bottom: 8, containLabel: true },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'value', splitLine: { lineStyle: { color: line } }, axisLabel: { color: axis, fontSize: 10 } },
      yAxis: {
        type: 'category',
        inverse: true,
        data: top.map((m) => m.model),
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: axis, fontSize: 10, width: 140, overflow: 'truncate' },
      },
      series: [
        {
          type: 'bar',
          barWidth: 12,
          data: top.map((m) => Number(m.tokens.toFixed(0))),
          itemStyle: { color: cssVar('--wb-ch-2'), borderRadius: [0, 4, 4, 0] },
        },
      ],
    })
  }
}

async function load() {
  loading.value = true
  try {
    // 用量跨引擎合并：两个引擎都写 usage 进各自的会话文件，账本该是同一本。
    const data = await api.sessions()
    rows.value = data.sessions ?? []
    renderCharts()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

function formatCost(v: number): string {
  if (!v) return '$0'
  return v < 0.01 ? `$${v.toFixed(5)}` : `$${v.toFixed(3)}`
}

function formatTokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

// 图色在渲染时快照 CSS 变量：主题或数据变了都得重画，否则轴色停在旧主题上。
watch([span, byDay, () => app.settings['ui.theme']], () => renderCharts())

function onResize() {
  trendChart?.resize()
  modelChart?.resize()
}

onMounted(() => {
  void load()
  window.addEventListener('resize', onResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  trendChart?.dispose()
  modelChart?.dispose()
  trendChart = null
  modelChart = null
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('usage.title') }}</h1>
        <p class="page-sub">{{ t('usage.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <WbSeg v-model="span" :items="spanItems" />
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <section class="stat-row">
        <div class="wb-card card-pad stat">
          <span class="wb-metric-label">{{ t('usage.total') }}</span>
          <span class="wb-metric">{{ formatCost(totals.cost) }}</span>
        </div>
        <div class="wb-card card-pad stat">
          <span class="wb-metric-label">{{ t('usage.inputTokens') }} + {{ t('usage.outputTokens') }}</span>
          <span class="wb-metric">{{ formatTokens(totals.tokens) }}</span>
        </div>
        <div class="wb-card card-pad stat">
          <span class="wb-metric-label">{{ t('usage.sessionCount') }}</span>
          <span class="wb-metric">{{ totals.sessions }}</span>
        </div>
      </section>

      <section class="card-pad chart-card">
        <div class="wb-sec-head"><span class="wb-sec-title">{{ t('usage.byDay') }}</span></div>
        <div ref="trendEl" class="chart" />
      </section>

      <div class="chart-row">
        <section class="card-pad chart-card">
          <div class="wb-sec-head"><span class="wb-sec-title">{{ t('usage.byModel') }}</span></div>
          <div v-if="byModel.length" ref="modelEl" class="chart" />
          <div v-else class="wb-empty">
            <span class="wb-tile wb-tile--sm wb-tile--neutral"><AppIcon name="chart" :size="16" /></span>
            <p class="wb-empty-title">{{ t('usage.emptyHint') }}</p>
          </div>
        </section>

        <section class="card-pad table-card">
          <div class="wb-sec-head"><span class="wb-sec-title">{{ t('usage.model') }}</span></div>
          <el-table :data="byModel" size="small" max-height="260">
            <el-table-column prop="model" :label="t('usage.model')" min-width="150">
              <template #default="{ row }">
                <span class="wb-mono">{{ row.model }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('usage.inputTokens')" width="96" align="right">
              <template #default="{ row }">
                <span class="wb-mono">{{ formatTokens(row.tokens) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('usage.cost')" width="92" align="right">
              <template #default="{ row }">
                <span class="wb-mono">{{ formatCost(row.cost) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="sessions" :label="t('usage.sessionCount')" width="80" align="right" />
            <template #empty>
              <span class="wb-empty-title">{{ t('usage.emptyTitle') }}</span>
            </template>
          </el-table>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: 4px 0 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.seg {
  display: flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-window);
}

/* 分段控件不缩放：只换底与字色 */



.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

.stat-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--wb-sp-3);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.chart-card {
  min-width: 0;
}

.chart {
  width: 100%;
  height: 220px;
}

.chart-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--wb-sp-4);
}

.table-card {
  min-width: 0;
}
</style>
