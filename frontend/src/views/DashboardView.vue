<script setup lang="ts">
// 用量仪表盘：四张读数卡 → 全宽趋势图 → 占比与榜单并排。
// 数据全部来自 PI 的会话文件；没有用量时整页停在空态，不画空图表。
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import { useSessionStore } from '../stores/session'
import { useToastStore } from '../stores/toast'
import type { StatsRESP } from '../types/api'
import { fmtCount, fmtInt, fmtMoney } from '../utils/num'
import AppIcon from '../components/common/AppIcon.vue'
import KpiCard from '../components/dashboard/KpiCard.vue'
import ModelShare from '../components/dashboard/ModelShare.vue'
import UsageTrend from '../components/dashboard/UsageTrend.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const router = useRouter()
const session = useSessionStore()
const toast = useToastStore()

const SPANS = [7, 14, 30]
const days = ref(14)
const data = ref<StatsRESP | null>(null)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.stats(days.value)
  } catch (e) {
    error.value = (e as Error)?.message || t('errors.generic')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
  void session.loadList()
})
watch(days, load)

const totals = computed(() => data.value?.totals)
const hasUsage = computed(() => (totals.value?.total || 0) > 0)
const topSessions = computed(() => (data.value?.sessions || []).slice(0, 6))

const hitRate = computed(() => {
  const tt = totals.value
  if (!tt?.input) return 0
  return Math.min(100, (tt.cached / tt.input) * 100)
})

const kpis = computed(() => {
  const tt = totals.value
  return [
    {
      key: 'total',
      icon: 'chart',
      value: fmtCount(tt?.total || 0),
      label: t('dashboard.kpiTotal'),
      note: `${t('dashboard.kpiCalls')} ${fmtInt(tt?.calls || 0)}`,
    },
    {
      key: 'io',
      icon: 'arrow-down-up',
      value: `${fmtCount(tt?.input || 0)} / ${fmtCount(tt?.output || 0)}`,
      label: t('dashboard.kpiIo'),
      note: t('dashboard.trendSub'),
    },
    {
      key: 'hit',
      icon: 'zap',
      value: `${hitRate.value.toFixed(hitRate.value >= 10 ? 0 : 1)}%`,
      label: t('dashboard.kpiHit'),
      note: `${fmtCount(tt?.cached || 0)} ${t('common.tokens')}`,
      hit: hitRate.value > 0,
    },
    {
      key: 'cost',
      icon: 'coin',
      value: fmtMoney(tt?.cost || 0),
      label: t('dashboard.kpiCost'),
      note: `${t('dashboard.kpiSessions')} ${fmtInt(tt?.sessions || 0)}`,
    },
  ]
})

const subline = computed(() => {
  const tt = totals.value
  if (!tt) return ''
  return `${fmtInt(tt.sessions)} · ${fmtInt(tt.messages)} · ${fmtInt(tt.calls)}`
})

const opening = ref('')
async function openSession(id: string) {
  if (opening.value) return
  opening.value = id
  try {
    await session.open(id)
    void router.push('/')
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    opening.value = ''
  }
}

function fmtDate(ms: number) {
  const d = new Date(ms)
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function onRowClick(row: { session_id: string }) {
  void openSession(row.session_id)
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('dashboard.title')" :subtitle="t('dashboard.subtitle', { days })">
      <template #actions>
        <el-radio-group v-model="days" size="small">
          <el-radio-button v-for="s in SPANS" :key="s" :value="s">
            {{ t(`dashboard.range${s}`) }}
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
        <el-button size="small" type="primary" @click="router.push('/')">{{ t('chat.newSession') }}</el-button>
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
            :hit="k.hit"
            :accent="k.key === 'total'"
          />
        </div>

        <section class="wb-card card-block mt-4">
          <header class="card-head">
            <div>
              <h3 class="card-title">{{ t('dashboard.trendTitle') }}</h3>
              <p class="card-sub">{{ t('dashboard.trendSub') }}</p>
            </div>
            <span class="card-chip">{{ subline }}</span>
          </header>
          <div class="trend-box">
            <UsageTrend :daily="data?.daily || []" />
          </div>
        </section>

        <div class="grid grid-cols-1 xl:grid-cols-2 gap-4 mt-4">
          <section class="wb-card card-block">
            <header class="card-head">
              <div>
                <h3 class="card-title">{{ t('dashboard.modelTitle') }}</h3>
                <p class="card-sub">{{ t('dashboard.modelSub') }}</p>
              </div>
            </header>
            <ModelShare v-if="(data?.models || []).length" :models="data?.models || []" />
            <p v-else class="card-empty">{{ t('dashboard.modelEmpty') }}</p>
          </section>

          <section class="wb-card card-block">
            <header class="card-head">
              <div>
                <h3 class="card-title">{{ t('dashboard.sessionTitle') }}</h3>
                <p class="card-sub">{{ t('dashboard.sessionSub') }}</p>
              </div>
            </header>
            <el-table
              v-if="topSessions.length"
              :data="topSessions"
              size="small"
              :show-header="false"
              class="sess-table"
              @row-click="onRowClick"
            >
              <el-table-column min-width="0">
                <template #default="{ row }">
                  <div class="sess-cell">
                    <span class="sess-name">{{ row.title || t('chat.newSession') }}</span>
                    <span class="sess-meta">{{ row.workspace }} · {{ fmtDate(row.updated_at) }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column width="96" align="right">
                <template #default="{ row }">
                  <span class="sess-tokens">{{ fmtCount(row.total) }}</span>
                </template>
              </el-table-column>
            </el-table>
            <p v-else class="card-empty">{{ t('dashboard.sessionEmpty') }}</p>
          </section>
        </div>
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
  background: var(--wb-surface-2);
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
.sess-table :deep(.el-table__row) {
  cursor: pointer;
}
.sess-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.sess-name {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sess-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sess-tokens {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-primary);
  font-weight: 700;
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
