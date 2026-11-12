<script setup lang="ts">
// 概览页：一屏回答「引擎什么状态、最近花了多少、有什么可继续的」。
// 是首屏：先看一眼全局再继续干活（最近会话就摆在同一屏）。
// 只做编排；数字全部来自引擎自己的落盘数据，不另存账本。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/common/AppIcon.vue'
import EngineHealthCard from '@/components/overview/EngineHealthCard.vue'
import EngineProcesses from '@/components/overview/EngineProcesses.vue'
import RecentSessions from '@/components/overview/RecentSessions.vue'
import UsageCharts from '@/components/overview/UsageCharts.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { HealthData, SessionRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const router = useRouter()
const toasts = useToasts()

const health = ref<HealthData | null>(null)
const loading = ref(false)
const span = ref<'today' | 'week' | 'month'>('week')
const rows = ref<SessionRow[]>([])
// 正在停的那个实例（用 pid 做键：一个引擎一个进程）。
const stoppingRun = ref(0)

const engines = computed(() => health.value?.health_engines ?? [])
const recent = computed(() => rows.value.slice(0, 6))
// 「能不能把数字当结论看」：后台装配跑完才算就绪。
// 只看装配标志不够——计数与最新版仍在后台补时界面已摆出一串 0，
// 而 0 是断言，那时它只是「还没数完」。
const ready = computed(() => Boolean(health.value) && !app.runtime?.detecting)
const detecting = computed(() => Boolean(app.runtime?.detecting))

/** 还不可信时就话「—」：零值不等于「有 0 个」。 */
const pendingMark = '—'

/** 重新探测：失败要弹原话，静默失败会让用户以为点了个假按钮。 */
async function redetect(): Promise<void> {
  try {
    await app.detectEngines()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

/** 一个读数怎么显示：没就绪的那一格不把 0 抛出去。 */
function readout(formatted: string): string {
  if (!ready.value) return pendingMark
  return formatted === '' ? pendingMark : formatted
}

/**
 * 三个「花了多少」的合计。
 *
 * 「装了什么」那三个（技能 / 插件 / MCP）由后端在 health 上直接给合计，
 * 这里不重算：那是引擎落盘的事实，界面算一遍就多一处可能漂移的真相。
 */
const usage = computed(() => {
  const since = Date.now() - (span.value === 'today' ? 1 : span.value === 'week' ? 7 : 30) * 86_400_000
  let tokens = 0
  let cost = 0
  let sessions = 0
  for (const row of rows.value) {
    if (row.updated_at && row.updated_at < since) continue
    tokens += row.total_tokens ?? 0
    cost += row.cost_usd ?? 0
    sessions += 1
  }
  return { tokens, cost, sessions }
})

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

async function load() {
  loading.value = true
  try {
    const [data, sessions] = await Promise.all([api.health(), api.sessions(app.workspace || undefined)])
    health.value = data
    rows.value = sessions.sessions ?? []
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function openSession(path: string) {
  try {
    await router.push({ name: 'chat', query: { session: path } })
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

/**
 * 中止当前运行。
 *
 * 中止的是这一轮，不是引擎：会话、配置、凭据一个字不动，
 * 下一句话随时可以再发。所以不需要二次确认。
 */
async function restartEngine(): Promise<void> {
  stoppingRun.value = 1
  try {
    await api.restartEngine()
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    stoppingRun.value = 0
  }
}

// 装配完成时后端推 engine:status，runtime.detecting 翻转的这一刻重拉本屏。
watch(
  () => app.runtime?.detecting,
  (value, prev) => {
    if (prev && !value) void load()
  },
)

// 工具链就绪状态变化也要重拉：那是这一屏的核心事实。
watch(
  () => app.runtime?.engine_ready,
  () => void load(),
)

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('overview.title') }}</h1>
        <p class="page-sub">{{ t('overview.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <div class="seg">
          <button
            v-for="key in (['today', 'week', 'month'] as const)"
            :key="key"
            class="seg-btn"
            :class="{ active: span === key }"
            @click="span = key"
          >
            {{ t(`overview.${key}`) }}
          </button>
        </div>
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <!-- 未就绪的横幅：说清「屏幕上这些东西还在数」。
           缺了它，用户看到一屏「—」只能以为界面坏了。 -->
      <div v-if="!ready" class="notice-row">
        <span class="dot busy" />
        <span>{{ t('overview.collecting') }}</span>
        <button class="wb-btn text sm" :class="{ 'is-loading': app.reDetecting }" :disabled="loading || app.reDetecting" @click="redetect">
          {{ t('overview.redetect') }}
        </button>
      </div>

      <section class="engine-grid">
        <EngineHealthCard
          v-for="engine in engines"
          :key="engine.engine_kind"
          :engine="engine"
          :detecting="detecting"
          @install="app.openApps()"
        />
      </section>

      <!-- 六个读数：三个是「花了多少」，三个是「装了什么」。
           全部读数在就绪前显示「—」：那一串 0 会被当成结论。 -->
      <section class="stat-row" :class="{ 'is-pending': !ready }">
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.spend') }}</span>
          <span class="wb-metric">{{ readout(formatCost(usage.cost)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.tokens') }}</span>
          <span class="wb-metric">{{ readout(formatTokens(usage.tokens)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.turns') }}</span>
          <span class="wb-metric">{{ readout(String(usage.sessions)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.capSkills') }}</span>
          <span class="wb-metric">{{ readout(String(health?.total_skills ?? 0)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.capMcp') }}</span>
          <span class="wb-metric">{{ readout(String(health?.total_mcp ?? 0)) }}</span>
        </div>
      </section>

      <UsageCharts :span="span" :rows="rows" />

      <!-- 引擎进程：不解释的话用户只看到「机器变卡」 -->
      <EngineProcesses
        :instance="health?.health_engines[0]"
        :stopping="Boolean(stoppingRun)"
        :ready="ready"
        @restart="restartEngine"
        @refresh="load"
      />

      <RecentSessions
        :rows="recent"
        :ready="ready"
        @open="openSession"
        @all="router.push({ name: 'sessions' })"
        @start="router.push({ name: 'chat' })"
      />
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
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-sm);
  line-height: 1.65;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-12);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

.engine-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: var(--wb-sp-3);
}

/* 六个读数排成 3 × 2：用固定三列而不是 auto-fit，
   否则宽屏会排成一行六个（每个只剩 130px，数字换了行）。 */
.stat-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--wb-sp-3);
}

/* 窄窗口下三列会把数字挤成两行：降成两列 */
@container app (max-width: 900px) {
  .stat-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.stat-row > .wb-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* 未就绪时读数走弱色：一眼能看出「这些还不是结论」 */
.stat-row.is-pending .wb-metric {
  color: var(--wb-faint);
}
</style>
