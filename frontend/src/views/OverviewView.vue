<script setup lang="ts">
// 概览页：一屏回答「引擎什么状态、最近花了多少、有什么可继续的」。
// 是首屏：先看一眼全局再继续干活（最近会话就摆在同一屏）。
// 只做编排；数字全部来自引擎自己的落盘数据，不另存账本。
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/common/AppIcon.vue'
import EngineHealthCard from '@/components/overview/EngineHealthCard.vue'
import EngineProcesses from '@/components/overview/EngineProcesses.vue'
import NextStepBar from '@/components/overview/NextStepBar.vue'
import RecentSessions from '@/components/overview/RecentSessions.vue'
import UsageCharts from '@/components/overview/UsageCharts.vue'
import WbSeg from '@/components/common/WbSeg.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
import type { SegItem } from '@/types/ui'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import { fmtCost, fmtCount } from '@/utils/format'
import type { HealthData, SessionRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const router = useRouter()
const toasts = useToasts()

const health = ref<HealthData | null>(null)
const loading = ref(false)
const span = ref<'today' | 'week' | 'month'>('week')

/** 时间窗分段：文案走 i18n，值就是 span 本身。 */
const spanItems = computed<SegItem[]>(() =>
  (['today', 'week', 'month'] as const).map((key) => ({ value: key, label: t(`overview.${key}`) })),
)
const rows = ref<SessionRow[]>([])
// 正在停的那个实例（用 pid 做键：一个引擎一个进程）。
const stoppingRun = ref(0)

// 首屏要判断「下一步该干什么」：服务商与模型清单各拉一次。
// 这两件事不随 health 推送，只在进页面时拿到；失败按「没配」处理——
// 那正是用户该去修的东西，界面不该因为查不到就假装配好了。
const providerCount = ref(0)
const modelCount = ref(0)
const choosingWorkspace = ref(false)

async function loadSetupState(): Promise<void> {
  try {
    const view = await api.providers()
    providerCount.value = view.providers?.length ?? 0
    modelCount.value = (view.providers ?? []).reduce((n, p) => n + (p.models?.length ?? 0), 0)
  } catch {
    providerCount.value = 0
    modelCount.value = 0
  }
}

async function pickWorkspace(): Promise<void> {
  choosingWorkspace.value = true
  try {
    await app.chooseWorkspace()
    // 换完工作区这一屏的会话与引擎计数都变了，重新拉一遍。
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    choosingWorkspace.value = false
  }
}

const engines = computed(() => health.value?.health_engines ?? [])
const recent = computed(() => rows.value.slice(0, 6))
// 「能不能把数字当结论看」：后台装配跑完才算就绪。
// 只看装配标志不够——计数与最新版仍在后台补时界面已摆出一串 0，
// 而 0 是断言，那时它只是「还没数完」。
const ready = computed(() => Boolean(health.value) && !app.runtime?.detecting)
const detecting = computed(() => Boolean(app.runtime?.detecting))

/** 有前提可数：选了工作区、配了服务商与模型。否则这一屏的读数必然是 0 / —。 */
const setupReady = computed(() => Boolean(app.workspace) && providerCount.value > 0 && modelCount.value > 0)

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

// 三个「花了多少」的合计：「装了什么」那几个由后端在 health 上直接给合计，
// 界面不重算——那是引擎落盘的事实，多算一处就多一处可能漂移的真相。
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



// 竞态守卫：detecting / engine_ready 两个 watch 叠加手动刷新时，晚到的旧快照直接丢弃。
let loadSeq = 0

async function load() {
  const mine = ++loadSeq
  loading.value = true
  try {
    const [data, sessions] = await Promise.all([
      api.health(),
      api.sessions(app.workspace || undefined),
      loadSetupState(),
    ])
    if (mine !== loadSeq) return
    health.value = data
    rows.value = sessions.sessions ?? []
  } catch (err) {
    if (mine !== loadSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === loadSeq) loading.value = false
  }
}

async function openSession(path: string) {
  try {
    await router.push({ name: 'chat', query: { session: path } })
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

// 中止当前运行：中止的是这一轮不是引擎，会话、配置、凭据一个字不动，
// 下一句话随时可以再发，不需要二次确认。
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

watch(
  () => app.workspace,
  () => void load(),
)

onMounted(() => void load())

// 离开页面后到货的响应没有意义：直接作废，不需要网络层中止。
onUnmounted(() => {
  loadSeq += 1
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('overview.title') }}</h1>
        <p class="page-sub">{{ t('overview.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <WbSeg v-model="span" :items="spanItems" />
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <WbSkeleton v-if="!health && loading" block />
      <template v-else>
      <!-- 先答「我现在该干什么」：这一条在引擎卡与读数之上，
           没工作区 / 没服务商时它取代一屏没有结论的数字。 -->
      <NextStepBar
        :workspace="app.workspace"
        :provider-count="providerCount"
        :has-model="modelCount > 0"
        :engine-ready="app.hasEngine"
        :choosing="choosingWorkspace"
        @pick-workspace="pickWorkspace"
        @open-models="app.openModels()"
        @start-chat="router.push({ name: 'chat' })"
      />

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

      <!-- 引擎此刻在不在干活、上次为什么失败：紧贴引擎卡，属于「现在能不能用」 -->
      <EngineProcesses
        :instance="health?.health_engines[0]"
        :stopping="Boolean(stoppingRun)"
        :ready="ready"
        @restart="restartEngine"
        @refresh="load"
      />

      <!-- 「有东西可数」：没工作区或没服务商时，这一屏的读数必然全是 0 / —，
           那不是结论而是还没有前提——所以整块换成一句说明，先让人去补前提。 -->
      <section v-if="!setupReady" class="need-setup wb-card card-pad">
        <AppIcon name="gauge" :size="18" class="need-ic" />
        <div>
          <p class="need-title">{{ t('overview.noDataYetTitle') }}</p>
          <p class="need-hint">{{ t('overview.noDataYetHint') }}</p>
        </div>
      </section>

      <template v-else>
      <!-- 读数：三个是「花了多少」，两个是「装了什么」。
           全部读数在就绪前显示「—」：那一串 0 会被当成结论。 -->
      <section class="stat-row" :class="{ 'is-pending': !ready }">
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.spend') }}</span>
          <span class="wb-metric">{{ readout(fmtCost(usage.cost)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.tokens') }}</span>
          <span class="wb-metric">{{ readout(fmtCount(usage.tokens)) }}</span>
        </div>
        <div class="wb-card card-pad">
          <span class="wb-metric-label">{{ t('overview.sessions') }}</span>
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

      </template>

      <!-- 最近会话与读数无关：没有工作区时也照样能从这里接着上次的聊 -->
      <RecentSessions
        :rows="recent"
        :ready="ready"
        @open="openSession"
        @all="router.push({ name: 'sessions' })"
        @start="router.push({ name: 'chat' })"
      />
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

/* 未就绪横幅：一行说明 + 动作，淡底不抢主内容的视觉重量 */
.notice-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-raised);
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.need-setup {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-3);
  border-style: dashed;
}

.need-ic {
  flex: none;
  margin-top: 1px;
  color: var(--wb-faint);
}

.need-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink-2);
}

.need-hint {
  margin: 2px 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
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

/* 五个读数：三列两行，最后一行右格留空也照样对齐（auto-fit 会把 5 个
   摊成一行，每个只剩 130px，数字跟着换行）。 */
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
  gap: var(--wb-sp-1);
}

/* 未就绪时读数走弱色：一眼能看出「这些还不是结论」 */
.stat-row.is-pending .wb-metric {
  color: var(--wb-faint);
}
</style>
