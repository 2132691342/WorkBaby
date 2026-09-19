<script setup lang="ts">
import { computed as computed2 } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { MessageSquare, BookOpen, GitBranch, Brain as BrainIcon } from '@/components/common/icons'
import { useDashboardStore } from '@/stores/dashboard'
import { formatDateTime } from '@/utils/time'
import { t } from '@/i18n'

/**
 * 仪表盘 · 三联（缓存命中率 / 工作流成功率 / 最近活动）+ 二联（快捷入口 / 执行审计）。
 * 数据全部来自真实 store（tokenTrend / stats），无占位硬编码。
 */
const dashboard = useDashboardStore()
const router = useRouter()
const { stats } = storeToRefs(dashboard)

// ===== 缓存命中率 =====
interface CacheSummary { input: number; output: number; cache: number; uncached: number; rate: number; costUSD: number }
const cacheSummary = computed2<CacheSummary | null>(() => {
  const s = dashboard.tokenTrend
  if (!s) return null
  const sum = (arr: number[]): number => arr.reduce((a, b) => a + (Number(b) || 0), 0)
  const input = sum(s.input)
  const output = sum(s.output)
  const cache = Math.min(sum(s.cache_read), input)
  if (input <= 0) return null
  return {
    input,
    output,
    cache,
    uncached: Math.max(input - cache, 0),
    rate: Math.round((cache / input) * 100),
    costUSD: s.cost_usd ?? 0
  }
})

// ===== 工作流执行聚合（按 workflow_id 分组成功率） =====
interface WfRow { id: string; total: number; success: number; rate: number }
const wfRows = computed2<WfRow[]>(() => {
  const rows = stats.value?.recent_executions ?? []
  const map = new Map<string, { total: number; success: number }>()
  for (const r of rows) {
    if (!r.workflow_id) continue
    const cur = map.get(r.workflow_id) ?? { total: 0, success: 0 }
    cur.total++
    if (r.status === 'success' || r.status === 'completed') cur.success++
    map.set(r.workflow_id, cur)
  }
  return Array.from(map.entries()).map(([id, v]) => ({
    id,
    total: v.total,
    success: v.success,
    rate: v.total === 0 ? 0 : Math.round((v.success / v.total) * 100)
  })).sort((a, b) => b.total - a.total).slice(0, 4)
})

// ===== 最近活动 =====
const recentExecutions = computed2(() => stats.value?.recent_executions ?? [])
const recentMessages = computed2(() => stats.value?.recent_messages ?? [])

function execType(status: string): 'success' | 'danger' | 'warning' | 'info' {
  if (status === 'success' || status === 'completed') return 'success'
  if (status === 'failed' || status === 'error') return 'danger'
  if (status === 'running' || status === 'pending') return 'warning'
  return 'info'
}

const bento = [
  { icon: MessageSquare, titleKey: 'dashboard.go.chat', descKey: 'dashboard.go.chatDesc', to: '/chat' },
  { icon: BookOpen, titleKey: 'dashboard.go.kdocs', descKey: 'dashboard.go.kdocsDesc', to: '/kdocs' },
  { icon: GitBranch, titleKey: 'dashboard.go.workflow', descKey: 'dashboard.go.workflowDesc', to: '/workflows' },
  { icon: BrainIcon, titleKey: 'dashboard.go.memory', descKey: 'dashboard.go.memoryDesc', to: '/memory' }
]

// 记忆中心入口展示真实三类记忆总数（此前是写死的「248 条」占位文案）。
const memoryTotal = computed2(() => {
  const s = stats.value
  if (!s) return null
  return (s.memory_episodes ?? 0) + (s.memory_facts ?? 0) + (s.memory_procedures ?? 0)
})

function descText(b: (typeof bento)[number]): string {
  if (b.to === '/memory') {
    return memoryTotal.value != null
      ? t('dashboard.go.memoryDesc', memoryTotal.value)
      : t('dashboard.go.memoryEmpty')
  }
  return t(b.descKey)
}

function go(to: string): void {
  void router.push(to)
}

function fmtTime(ts: number | null): string {
  if (!ts) return ''
  return formatDateTime(ts)
}

/** token 数人类可读：132600 → 129.5K（与 UsageBadge 同 1024 进制口径）。 */
function fmtTokens(n: number): string {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)}M`
  if (n >= 1024) return `${(n / 1024).toFixed(1)}K`
  return String(n)
}
</script>

<template>
  <!-- 三联：缓存命中率 / 工作流成功率 / 最近活动。
       三卡等高（dash-card 固定高度），超高内容卡内滚动——
       最近活动不再把左侧两张卡「挤扁」，窄屏时自动降为单列。 -->
  <div class="dash-trio">
    <!-- 缓存命中率（真实数据：token-trend 聚合） -->
    <div class="card p-sm dash-card">
      <h3 class="mb10">{{ t('dashboard.cacheRate') }}</h3>
      <p v-if="cacheSummary" class="fs11 muted" style="margin: -6px 0 8px">{{ t('dashboard.cacheScopeHint') }}</p>
      <div v-if="cacheSummary" class="dash-card__body">
        <div class="ring" :style="{
          width: '66px',
          height: '66px',
          background: `conic-gradient(var(--wb-primary) ${cacheSummary.rate}%, var(--wb-surface-hover) 0)`
        }">
          <i style="width: 50px; height: 50px; font-size: 15px">{{ cacheSummary.rate }}%</i>
        </div>
        <div class="fs11 muted">
          <p>{{ t('dashboard.cacheRead', fmtTokens(cacheSummary.cache)) }}</p>
          <p>{{ t('dashboard.cacheCharged', fmtTokens(cacheSummary.uncached)) }}</p>
          <p>{{ t('dashboard.cacheOutput', fmtTokens(cacheSummary.output)) }}</p>
          <!-- 费用为估算值：按 pricing.<model> 单价 KV 计算；未配置单价的模型不计入 -->
          <p v-if="cacheSummary.costUSD > 0">{{ t('dashboard.costUSD', cacheSummary.costUSD.toFixed(4)) }}</p>
        </div>
      </div>
      <div v-else class="empty fs11" style="flex: 1; display: flex; align-items: center; justify-content: center; padding: 12px 0">{{ t('dashboard.noTokenData') }}</div>
    </div>

    <!-- 工作流执行 · 近期聚合 -->
    <div class="card p-sm dash-card">
      <h3 class="mb10">{{ t('dashboard.wfAggTitle') }}</h3>
      <div v-if="wfRows.length === 0" class="empty fs11" style="flex: 1; display: flex; align-items: center; justify-content: center; padding: 12px 0">{{ t('dashboard.noExecutions') }}</div>
      <div v-else class="fs11 dash-card__scroll">
        <div v-for="r in wfRows" :key="r.id" class="flex-r mb8">
          <span class="mono" style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ r.id.slice(0, 18) }}</span>
          <span class="mono">{{ r.total }} {{ t('dashboard.times') }}</span>
          <span class="badge" :class="r.rate >= 80 ? 'b-success' : r.rate >= 50 ? 'b-warning' : 'b-danger'">{{ r.rate }}%</span>
        </div>
      </div>
    </div>

    <!-- 最近活动（混合：消息 + 执行） -->
    <div class="card p-sm dash-card">
      <h3 class="mb10">{{ t('dashboard.activity') }}</h3>
      <div v-if="recentMessages.length === 0 && recentExecutions.length === 0" class="empty fs11" style="flex: 1; display: flex; align-items: center; justify-content: center; padding: 12px 0">{{ t('dashboard.noActivity') }}</div>
      <div v-else class="rowlist dash-card__scroll" style="font-size: 11px">
        <div v-for="m in recentMessages.slice(0, 8)" :key="m.id" class="rli">
          <span class="led" :class="m.role === 'assistant' ? 'g' : 'w'" />
          <div class="grow" style="min-width: 0">
            <h5 class="truncate">{{ m.content }}</h5>
            <p>{{ fmtTime(m.created_at) }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- 快捷入口 -->
  <div class="grid2">
    <div class="card p-sm">
      <h3 class="mb10">{{ t('dashboard.shortcuts') }}</h3>
      <div class="bento" style="grid-template-columns: repeat(2, 1fr)">
        <button v-for="b in bento" :key="b.titleKey" @click="go(b.to)">
          <div class="mini-tile"><component :is="b.icon" class="ic" /></div>
          <div>
            <h5>{{ t(b.titleKey) }}</h5>
            <p>{{ descText(b) }}</p>
          </div>
        </button>
      </div>
    </div>

    <!-- 执行审计 · 替换掉「最近产出」「最近活动」冗余卡（原 07 屏右下），保留可执行列表 -->
    <div class="card p-sm">
      <h3 class="mb10">{{ t('dashboard.recent_executions') }}</h3>
      <div v-if="recentExecutions.length === 0" class="empty fs11" style="padding: 12px 0">{{ t('dashboard.noExecutions') }}</div>
      <div v-else class="rowlist">
        <div v-for="e in recentExecutions.slice(0, 5)" :key="e.id" class="rli">
          <span class="led" :class="execType(e.status) === 'success' ? 'g' : execType(e.status) === 'danger' ? 'r' : execType(e.status) === 'warning' ? 'w' : 'n'" />
          <div class="grow">
            <h5 class="mono">{{ e.workflow_id.slice(0, 18) }}…</h5>
            <p>{{ fmtTime(e.started_at) }}</p>
          </div>
          <span class="badge" :class="`b-${execType(e.status) === 'success' ? 'success' : execType(e.status) === 'danger' ? 'danger' : execType(e.status) === 'warning' ? 'warning' : 'neutral'}`">
            <span class="dot" />{{ e.status }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 三联等高：固定行高 + 卡内滚动，最近活动条数再多也不挤压左邻卡片 */
.dash-trio {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 12px;
  align-items: stretch;
}
.dash-card {
  display: flex;
  flex-direction: column;
  height: 208px;
  min-width: 0;
  overflow: hidden;
}
.dash-card__body {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 0;
}
.dash-card__scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
</style>
