<script setup lang="ts">
import { errorText } from '../utils/errors'
// 用量仪表盘：四张读数卡 → 全宽趋势图 → 模型占比。
// 数据来自统计接口的 resp_daily（天粒度）与 resp_by_model（按模型）；没有用量时整页停在空态，不画空图表。
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import type { StatsDailyItem, StatsRESP } from '../types/api'
import { fmtCount, fmtMoney } from '../utils/num'
import AppIcon from '../components/common/AppIcon.vue'
import KpiCard from '../components/dashboard/KpiCard.vue'
import ModelShare from '../components/dashboard/ModelShare.vue'
import UsageTrend from '../components/dashboard/UsageTrend.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const router = useRouter()

// span 只认 day7 / day30 / all；「全部」没有专属键，沿用 common.all
const SPANS = ['day7', 'day30', 'all'] as const
type Span = (typeof SPANS)[number]
const span = ref<Span>('day30')
const data = ref<StatsRESP | null>(null)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.stats(span.value)
  } catch (e) {
    error.value = errorText(e) || t('errors.generic')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})
watch(span, load)

const spanName = (s: Span) =>
  s === 'all' ? t('common.all') : t(s === 'day7' ? 'dashboard.range_7d' : 'dashboard.range_30d')
const spanLabel = computed(() => spanName(span.value))

// 后端不承诺顺序：图表与求和都按天升序
const daily = computed<StatsDailyItem[]>(() =>
  [...(data.value?.resp_daily ?? [])].sort((a, b) => a.daily_day.localeCompare(b.daily_day)),
)
const models = computed(() => data.value?.resp_by_model ?? [])

const sumTokens = (items: StatsDailyItem[]) =>
  items.reduce((s, d) => s + (d.daily_input || 0) + (d.daily_output || 0), 0)
const sumCost = (items: StatsDailyItem[]) => items.reduce((s, d) => s + (d.daily_cost || 0), 0)

const rangeInput = computed(() => daily.value.reduce((s, d) => s + (d.daily_input || 0), 0))
const rangeOutput = computed(() => daily.value.reduce((s, d) => s + (d.daily_output || 0), 0))
const hasUsage = computed(() => rangeInput.value + rangeOutput.value > 0)

// 「今日」按本地日期匹配 daily_day（YYYY-MM-DD）：统计服务与本机同一天
const todayKey = computed(() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
})
const today = computed(() => daily.value.find((d) => d.daily_day === todayKey.value))
const todayTokens = computed(() => (today.value ? sumTokens([today.value]) : 0))
const todayCost = computed(() => today.value?.daily_cost || 0)
const todayInput = computed(() => today.value?.daily_input || 0)
const todayOutput = computed(() => today.value?.daily_output || 0)

const kpis = computed(() => [
  {
    key: 'total',
    icon: 'chart',
    value: fmtCount(rangeInput.value + rangeOutput.value),
    label: t('dashboard.kpiTotal'),
    note: `${t('dashboard.input')} ${fmtCount(rangeInput.value)} / ${t('dashboard.output')} ${fmtCount(rangeOutput.value)}`,
  },
  {
    key: 'cost',
    icon: 'coin',
    value: fmtMoney(sumCost(daily.value)),
    label: t('dashboard.kpiCost'),
    note: t('dashboard.kpiCostNote'),
  },
  {
    key: 'today',
    icon: 'zap',
    value: fmtCount(todayTokens.value),
    label: t('dashboard.range_today'),
    note: `${t('dashboard.kpiCost')} ${fmtMoney(todayCost.value)}`,
  },
  {
    key: 'todayIo',
    icon: 'clock',
    value: `${fmtCount(todayInput.value)} / ${fmtCount(todayOutput.value)}`,
    label: t('dashboard.kpiIo'),
    note: t('dashboard.range_today'),
  },
])
</script>

<template>
  <div class="view">
    <PageHeader :title="t('dashboard.title')" :subtitle="t('dashboard.subtitle', { range: spanLabel })">
      <template #actions>
        <el-radio-group v-model="span" size="small">
          <el-radio-button v-for="s in SPANS" :key="s" :value="s">
            {{ spanName(s) }}
          </el-radio-button>
        </el-radio-group>
        <el-button size="small" :loading="loading" @click="load">
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('common.refresh') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <div v-if="error" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="alert" size="ic-xl" /></span>
        <b>{{ t('dashboard.empty') }}</b>
        <p>{{ error }}</p>
        <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
      </div>

      <template v-else-if="loading && !data">
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <el-skeleton v-for="i in 4" :key="i" animated :rows="2" class="wb-card p-5" />
        </div>
        <div class="wb-card p-5 mt-4"><el-skeleton animated :rows="6" /></div>
      </template>

      <div v-else-if="!hasUsage" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="chart" size="ic-xl" /></span>
        <b>{{ t('dashboard.empty') }}</b>
        <p>{{ t('dashboard.emptyHint') }}</p>
        <el-button size="small" type="primary" @click="router.push('/chat')">{{ t('chat.newSession') }}</el-button>
      </div>

      <template v-else>
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <KpiCard
            v-for="k in kpis"
            :key="k.key"
            :icon="k.icon"
            :label="k.label"
            :value="k.value"
            :note="k.note"
            :accent="k.key === 'total'"
          />
        </div>

        <section class="wb-card card-block mt-4">
          <header class="card-head">
            <div>
              <h3 class="card-title">{{ t('dashboard.trendTitle') }}</h3>
              <p class="card-sub">{{ t('dashboard.trendDay') }}</p>
            </div>
            <span class="card-chip">{{ spanLabel }}</span>
          </header>
          <div class="trend-box">
            <UsageTrend :daily="daily" />
          </div>
        </section>

        <section class="wb-card card-block mt-4">
          <header class="card-head">
            <div>
              <h3 class="card-title">{{ t('dashboard.modelTitle') }}</h3>
              <p class="card-sub">{{ t('dashboard.modelSub') }}</p>
            </div>
          </header>
          <ModelShare v-if="models.length" :models="models" />
          <p v-else class="card-empty">{{ t('dashboard.modelEmpty') }}</p>
        </section>
      </template>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
}
.card-block {
  padding: var(--wb-sp-5);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-3);
}
.card-title {
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
}
.card-sub {
  margin-top: 2px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.card-chip {
  flex: none;
  padding: 2px var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.trend-box {
  height: calc(260px * var(--wb-vw-scale));
}
.card-empty {
  padding: var(--wb-sp-6) 0;
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(420px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
