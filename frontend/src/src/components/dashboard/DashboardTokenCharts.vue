<script setup lang="ts">
import { computed as computed2, onBeforeUnmount, ref, watch } from 'vue'
import { useDashboardStore } from '@/stores/dashboard'
import { useWbChartTheme, withAlpha } from '@/composables/useWbChartTheme'
import { t } from '@/i18n'

/**
 * 仪表盘 · Token 消耗区：三线折线 + 缓存命中率环形图（同源聚合）。
 */
type ECharts = { setOption(o: unknown): void; dispose(): void }

const dashboard = useDashboardStore()
const chartTheme = useWbChartTheme()

// ===== Token 消耗折线（三线：输入/输出/缓存命中） =====
const tokenEl = ref<HTMLDivElement | null>(null)
let tokenChart: ECharts | null = null
/** 缓存命中率饼图：与 token 折线同窗口同数据源（tokenTrend 数组直接聚合），无额外请求。 */
const pieEl = ref<HTMLDivElement | null>(null)
let pieChart: ECharts | null = null
const tokenScope = ref<'today' | 'week' | 'month' | 'custom'>('today')
/** 自定义范围（精确到天）；换 scope 时清空，避免上次选区残留误导。 */
const tokenCustomRange = ref<[Date, Date] | null>(null)

const tokenTotalText = computed2(() => {
  const d = dashboard.tokenTrend
  if (!d) return '—'
  return d.total.toLocaleString()
})
// eslint-disable-next-line @typescript-eslint/no-unused-vars
void tokenTotalText.value

/** 当前窗口整体聚合（饼图与折线同源；cache 是 input 的子维度，命中率 = cache/input）。 */
const tokenPieSummary = computed2(() => {
  const d = dashboard.tokenTrend
  if (!d) return null
  const sum = (arr: number[]): number => arr.reduce((a, b) => a + (Number(b) || 0), 0)
  const input = sum(d.input)
  const output = sum(d.output)
  const cache = Math.min(sum(d.cache_read), input) // 数据异常（cache>input）时钳制，避免负值扇区
  return { input, output, cache, uncached: Math.max(input - cache, 0) }
})
const tokenCacheRate = computed2(() => {
  const s = tokenPieSummary.value
  if (!s || s.input <= 0) return 0
  return Math.round((s.cache / s.input) * 100)
})

function onTokenScopeChange(v: 'today' | 'week' | 'month' | 'custom'): void {
  // 先更新高亮态：之前只发请求不改 tokenScope，导致数据切了、按钮永远停在「今日」
  tokenScope.value = v
  if (v === 'custom') {
    // 切到自定义但尚未选区间：默认给最近 7 天，用户可即时改
    const end = new Date()
    const start = new Date(end.getTime() - 6 * 86400000)
    if (!tokenCustomRange.value) tokenCustomRange.value = [start, end]
    void applyTokenCustom()
    return
  }
  void dashboard.loadTokenTrend({ scope: v })
}

async function applyTokenCustom(): Promise<void> {
  const r = tokenCustomRange.value
  if (!r || r.length !== 2) return
  const startAt = new Date(r[0]).setHours(0, 0, 0, 0)
  const endAt = new Date(r[1]).setHours(0, 0, 0, 0)
  await dashboard.loadTokenTrend({ scope: 'custom', start_at: startAt, end_at: endAt })
}

function onScope(s: 'today' | 'week' | 'month' | 'custom'): void {
  onTokenScopeChange(s)
}

/** 供父组件刷新：按当前 scope 重取 token 趋势。 */
function reload(): Promise<void> {
  return dashboard.loadTokenTrend({ scope: tokenScope.value })
}
defineExpose({ reload })

async function initLineChart(el: HTMLDivElement): Promise<ECharts> {
  const core = await import('echarts/core')
  const { LineChart } = await import('echarts/charts')
  const { TooltipComponent, LegendComponent, GridComponent } = await import('echarts/components')
  const { CanvasRenderer } = await import('echarts/renderers')
  core.use([LineChart, TooltipComponent, LegendComponent, GridComponent, CanvasRenderer])
  return core.init(el) as unknown as ECharts
}

/** token 三线折线：蓝色梯度（主蓝 / 亮蓝 / 青绿点缀）+ 面积渐隐，今日视图横轴为 24 小时。 */
function renderTokenChart(): void {
  if (!tokenEl.value || !dashboard.tokenTrend) return
  const d = dashboard.tokenTrend
  const th = chartTheme.value
  const series = [
    { key: 'input' as const, name: t('dashboard.tokenInput'), color: th.primary, alpha: 0.30 },
    { key: 'output' as const, name: t('dashboard.tokenOutput'), color: th.sky, alpha: 0.16 },
    { key: 'cache_read' as const, name: t('dashboard.tokenCache'), color: th.mint, alpha: 0.10 }
  ]
  void initLineChart(tokenEl.value).then((c) => {
    tokenChart = c
    c.setOption({
      tooltip: {
        trigger: 'axis',
        backgroundColor: th.surface,
        borderColor: 'transparent',
        textStyle: { color: th.ink },
        valueFormatter: (v: unknown) => (typeof v === 'number' ? v.toLocaleString() : '0')
      },
      legend: { bottom: 0, textStyle: { color: th.muted }, icon: 'roundRect' },
      grid: { left: 8, right: 12, top: 20, bottom: 36, containLabel: true },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: d.labels,
        axisLabel: { color: th.muted, fontSize: 10, interval: d.granularity === 'hour' ? 2 : 'auto' },
        axisLine: { lineStyle: { color: th.border } },
        axisTick: { show: false }
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: th.muted, fontSize: 10, formatter: (v: number) => fmtAxisTokens(v) },
        splitLine: { lineStyle: { color: th.border } }
      },
      series: series.map((s) => ({
        name: s.name,
        type: 'line' as const,
        smooth: true,
        showSymbol: d.labels.length <= 24,
        data: d[s.key],
        lineStyle: { width: 2, color: s.color },
        itemStyle: { color: s.color },
        areaStyle: {
          color: {
            type: 'linear' as const, x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: withAlpha(s.color, s.alpha) },
              { offset: 1, color: withAlpha(s.color, 0) }
            ]
          }
        }
      }))
    })
  })
}

/** 轴标签缩写：12.3k / 1.2M，避免长数字挤压横轴。 */
function fmtAxisTokens(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`
  return String(v)
}

async function initPieChart(el: HTMLDivElement): Promise<ECharts> {
  const core = await import('echarts/core')
  const { PieChart } = await import('echarts/charts')
  const { TooltipComponent, LegendComponent, TitleComponent } = await import('echarts/components')
  const { CanvasRenderer } = await import('echarts/renderers')
  core.use([PieChart, TooltipComponent, LegendComponent, TitleComponent, CanvasRenderer])
  return core.init(el) as unknown as ECharts
}

/** 缓存命中率环形图：三个扇区 = 命中缓存 / 未命中输入 / 输出，中心大字号命中率。 */
function renderPieChart(): void {
  if (!pieEl.value) return
  const s = tokenPieSummary.value
  const th = chartTheme.value
  const hasData = !!s && s.input + s.output > 0
  void initPieChart(pieEl.value).then((c) => {
    pieChart = c
    c.setOption({
      tooltip: {
        trigger: 'item',
        backgroundColor: th.surface,
        borderColor: 'transparent',
        textStyle: { color: th.ink },
        formatter: (p: { name: string; value: number; percent: number }) =>
          `${p.name}: ${(p.value as number).toLocaleString()} (${p.percent}%)`
      },
      legend: {
        bottom: 0,
        left: 'center',
        itemWidth: 8,
        itemHeight: 8,
        icon: 'circle',
        textStyle: { color: th.muted, fontSize: 10 }
      },
      title: {
        text: hasData ? `${tokenCacheRate.value}%` : '0%',
        subtext: t('dashboard.tokenCacheRate'),
        left: 'center',
        top: '34%',
        textStyle: { color: th.ink, fontSize: 24, fontWeight: 700 },
        subtextStyle: { color: th.muted, fontSize: 10 }
      },
      series: [
        {
          name: t('dashboard.tokenCacheRate'),
          type: 'pie',
          radius: ['52%', '74%'],
          center: ['50%', '42%'],
          avoidLabelOverlap: true,
          itemStyle: { borderColor: th.surface, borderWidth: 2, borderRadius: 4 },
          label: { show: false },
          emphasis: { label: { show: false } },
          data: hasData
            ? [
                { value: s!.cache, name: t('dashboard.tokenCached'), itemStyle: { color: th.sky } },
                { value: s!.uncached, name: t('dashboard.tokenUncached'), itemStyle: { color: th.primary } },
                { value: s!.output, name: t('dashboard.tokenOutput'), itemStyle: { color: th.mint } }
              ]
            : []
        }
      ]
    })
  })
}

watch(() => dashboard.tokenTrend, (v) => {
  if (v) {
    renderTokenChart()
    renderPieChart()
  }
}, { deep: true })

// 主题切换时重绘 echarts
watch(chartTheme, () => {
  if (tokenChart) {
    renderTokenChart()
  }
  renderPieChart()
})

onBeforeUnmount(() => {
  tokenChart?.dispose()
  pieChart?.dispose()
})
</script>

<template>
  <section class="card card--chart">
    <div class="flex-r mb10" style="flex-wrap: wrap; gap: 10px">
      <div>
        <h2>{{ t('dashboard.tokenTrend') }}</h2>
        <p class="fs11 muted mt4">
          {{ t('dashboard.tokenTotal') }}：
          <span class="mono" style="color: var(--wb-ink); font-weight: 600">{{ tokenTotalText }}</span>
        </p>
      </div>
      <span class="sp" />
      <div class="seg">
        <button :class="{ on: tokenScope === 'today' }" @click="onScope('today')">{{ t('dashboard.tokenScopeToday') }}</button>
        <button :class="{ on: tokenScope === 'week' }" @click="onScope('week')">{{ t('dashboard.tokenScopeWeek') }}</button>
        <button :class="{ on: tokenScope === 'month' }" @click="onScope('month')">{{ t('dashboard.tokenScopeMonth') }}</button>
        <button :class="{ on: tokenScope === 'custom' }" @click="onScope('custom')">{{ t('dashboard.tokenScopeCustom') }}</button>
      </div>
      <!-- 自定义范围：选中 custom 后展开日期区间 -->
      <el-date-picker
        v-if="tokenScope === 'custom'"
        v-model="tokenCustomRange"
        type="daterange"
        size="small"
        :clearable="false"
        :start-placeholder="t('dashboard.customStart')"
        :end-placeholder="t('dashboard.customEnd')"
        style="width: 230px"
        @change="applyTokenCustom"
      />
      <div class="legend">
        <span><i :style="{ background: 'var(--wb-primary)' }" />{{ t('dashboard.tokenInput') }}</span>
        <span><i :style="{ background: 'var(--wb-sky)' }" />{{ t('dashboard.tokenOutput') }}</span>
        <span><i :style="{ background: 'var(--wb-mint)' }" />{{ t('dashboard.tokenCache') }}</span>
      </div>
    </div>
    <div v-if="!dashboard.tokenTrend" class="flex h-56 items-center justify-center rounded-lg border border-dashed border-wb-border text-sm text-wb-muted">
      {{ t('dashboard.tokenNoData') }}
    </div>
    <div v-show="dashboard.tokenTrend" ref="tokenEl" style="width: 100%; height: 230px" />
  </section>
</template>
