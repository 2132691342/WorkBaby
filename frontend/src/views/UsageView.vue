<script setup lang="ts">
// 用量页：数字全部来自引擎统计的会话文件（通过 sessions 列表聚合），不另存一份。
// 两张图交给 UsageCharts（概览页同一个组件）：echarts 实例的生命周期只在那里管一次，
// 这一页再自己 init 一遍就是两套画法、两处 dispose 机会。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import UsageCharts from '@/components/overview/UsageCharts.vue'
import WbSeg from '@/components/common/WbSeg.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import type { SegItem } from '@/types/ui'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { fmtCost, fmtCount } from '@/utils/format'
import type { SessionRow } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

const rows = ref<SessionRow[]>([])
const loading = ref(false)
// 首次取数是否已经落定：为假时不说「还没有用量」——那是一句断言。
const loaded = ref(false)
// 取数是否失败：失败时那三张图与那张表都不该摆成「0」。
const loadFailed = ref(false)
const span = ref<'today' | 'week' | 'month' | 'total'>('week')

/** 时间窗分段：文案走 i18n，值就是 span 本身。 */
const spanItems = computed<SegItem[]>(() =>
  (['today', 'week', 'month', 'total'] as const).map((key) => ({ value: key, label: t(`usage.${key}`) })),
)

const windowDays = computed(() =>
  span.value === 'today' ? 1 : span.value === 'week' ? 7 : span.value === 'month' ? 30 : 3650,
)

/** 落在时间窗内的会话：未带时间戳的也算进来（老会话没有 updated_at）。 */
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

/** 窗口内一条会话都没有：图上什么也画不出来，用空态替掉两张空画布。 */
const empty = computed(() => loaded.value && scoped.value.length === 0)

let loadSeq = 0

async function load(): Promise<void> {
  const mine = ++loadSeq
  loading.value = true
  try {
    // 用量来自全部会话文件的合计：账本只有一本，就是那些 jsonl。
    const data = await api.sessions()
    if (mine !== loadSeq) return
    rows.value = data.sessions ?? []
    loadFailed.value = false
  } catch (err) {
    if (mine !== loadSeq) return
    loadFailed.value = true
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === loadSeq) {
      loading.value = false
      loaded.value = true
    }
  }
}

async function refresh(): Promise<void> {
  await load()
}

onMounted(() => {
  void load()
})

// 离开页面后到货的响应没有意义：直接作废。
onUnmounted(() => {
  loadSeq += 1
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
        <button type="button"
          class="wb-btn icon ghost"
          :class="{ 'is-loading': loading }"
          :title="t('common.refresh')" :aria-label="t('common.refresh')"
          :disabled="loading"
          @click="refresh"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <WbSkeleton v-if="loading && !loaded" block />

      <!-- 取数失败：不能摆一屏 0，那会被当成「没花过钱」的结论 -->
      <section v-else-if="loadFailed" class="load-failed">
        <AppIcon name="alert" :size="20" class="failed-ic" />
        <p class="failed-title">{{ t('common.loadFailed') }}</p>
        <p class="failed-hint">{{ t('usage.loadFailedHint') }}</p>
        <button class="wb-btn sm primary" type="button" @click="refresh">{{ t('common.retry') }}</button>
      </section>

      <template v-else-if="empty">
        <WbEmpty icon="chart" :title="t('usage.emptyTitle')" :hint="t('usage.emptyHint')" />
      </template>

      <template v-else>
        <section class="stat-row">
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('usage.total') }}</span>
            <span class="wb-metric">{{ fmtCost(totals.cost) }}</span>
          </div>
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('usage.inputTokens') }} + {{ t('usage.outputTokens') }}</span>
            <span class="wb-metric">{{ fmtCount(totals.tokens) }}</span>
          </div>
          <div class="wb-card card-pad stat">
            <span class="wb-metric-label">{{ t('usage.sessionCount') }}</span>
            <span class="wb-metric">{{ totals.sessions }}</span>
          </div>
        </section>

        <UsageCharts :span="span" :rows="rows" />

        <section class="wb-card card-pad table-card">
          <div class="wb-sec-head"><span class="wb-sec-title">{{ t('usage.model') }}</span></div>
          <el-table :data="byModel" size="small" max-height="280">
            <el-table-column prop="model" :label="t('usage.model')" min-width="150">
              <template #default="{ row }">
                <span class="wb-mono">{{ row.model }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('usage.totalTokens')" width="110" align="right">
              <template #default="{ row }">
                <span class="wb-mono">{{ fmtCount(row.tokens) }}</span>
              </template>
            </el-table-column>
            <el-table-column :label="t('usage.cost')" width="110" align="right">
              <template #default="{ row }">
                <span class="wb-mono">{{ fmtCost(row.cost) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="sessions" :label="t('usage.sessionCount')" width="90" align="right" />
            <template #empty>
              <span class="wb-empty-title">{{ t('usage.emptyTitle') }}</span>
            </template>
          </el-table>
        </section>
      </template>
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
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

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
  gap: var(--wb-sp-1);
}

/* 取数失败：一块说明 + 一个重试，不是一屏 0 */
.load-failed {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-5);
  text-align: center;
}
.failed-ic {
  color: var(--wb-warn);
}
.failed-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink-2);
}
.failed-hint {
  margin: 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
}

.table-card {
  min-width: 0;
}
</style>
