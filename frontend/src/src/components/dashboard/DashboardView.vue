<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { LayoutDashboard, RefreshCw } from '@/components/common/icons'
import { useDashboardStore } from '@/stores/dashboard'
import DashboardTokenCharts from '@/components/dashboard/DashboardTokenCharts.vue'
import DashboardTrendExec from '@/components/dashboard/DashboardTrendExec.vue'
import DashboardRuntime from '@/components/dashboard/DashboardRuntime.vue'
import { t } from '@/i18n'

/**
 * 仪表盘（照 prd/WorkBaby-UI-Prototype.html 07 屏）：
 * hero + 4 KPI（全部接真实 store，无数据时显 —）+
 * Token 三线折线（默认 today，可切 week / month / custom）+ 三联二联全部接真实数据 + 系统运行时折叠。
 */
const dashboard = useDashboardStore()
const { stats } = storeToRefs(dashboard)
const tokenChartsRef = ref<InstanceType<typeof DashboardTokenCharts> | null>(null)

async function refresh(): Promise<void> {
  await dashboard.load()
  await dashboard.loadTrend()
  await tokenChartsRef.value?.reload()
}

const todayMessages = computed(() => stats.value?.today_messages ?? null)
const todaySessions = computed(() => stats.value?.today_sessions ?? null)
const todayTokens = computed(() => stats.value?.today_tokens ?? null)
const aiTools = computed(() => stats.value?.ai_tools_total ?? null)

function fmtTokens(n: number | null): string {
  if (n == null) return '—'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`
  return String(n)
}

onMounted(() => {
  void dashboard.load()
  void dashboard.loadTrend()
  void dashboard.loadTokenTrend({ scope: 'today' })
})
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-lg">
      <!-- Hero -->
      <header class="hero">
        <div class="tile"><LayoutDashboard class="ic" /></div>
        <div>
          <h1>{{ t('dashboard.title') }}</h1>
          <p>{{ t('dashboard.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn" @click="refresh">
          <RefreshCw class="ic ic-sm" />
          {{ t('dashboard.refresh') }}
        </button>
      </header>

      <!-- 4 KPI（全部接真实数据；空态显 —） -->
      <div class="kpi">
        <div class="card c">
          <p class="l">{{ t('dashboard.today_messages') }}</p>
          <p class="v tnum">{{ todayMessages ?? '—' }}</p>
          <p class="d">{{ t('dashboard.todayHint') }}</p>
        </div>
        <div class="card c">
          <p class="l">{{ t('dashboard.today_sessions') }}</p>
          <p class="v tnum">{{ todaySessions ?? '—' }}</p>
          <p class="d">{{ t('dashboard.todayHint') }}</p>
        </div>
        <div class="card c">
          <p class="l">{{ t('dashboard.today_tokens') }}</p>
          <p class="v tnum">{{ fmtTokens(todayTokens) }}</p>
          <p class="d">{{ t('dashboard.todayHint') }}</p>
        </div>
        <div class="card c">
          <p class="l">{{ t('dashboard.aiTools') }}</p>
          <p class="v tnum">{{ aiTools ?? '—' }}</p>
          <p class="d">{{ t('dashboard.aiToolsHint') }}</p>
        </div>
      </div>

      <!-- Token 趋势 -->
      <DashboardTokenCharts ref="tokenChartsRef" />

      <!-- 三联 + 二联 -->
      <DashboardTrendExec />

      <!-- 系统运行时折叠 -->
      <DashboardRuntime />

      <div v-if="dashboard.error" class="alert a-danger">{{ dashboard.error }}</div>
    </div>
  </div>
</template>
