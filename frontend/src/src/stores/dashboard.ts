import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet } from '@/api/client'

/**
 * 仪表盘统计契约（双主机 Go 后端，camelCase）。
 *
 * <p>对应 {@code GET /api/v1/dashboard/stats}：累计计数 + 今日活动 +
 * 最近活动列表（消息/工作流执行）+ Go 运行时信息。
 */
export interface SystemInfo {
  os_name: string
  os_arch: string
  go_version: string
  user_home: string
  num_cpu: number
  goroutines: number
  heap_alloc_mb: number
  heap_sys_mb: number
}

/** 最近消息（活动流用；content 已截前 120 字符）。 */
export interface RecentMessage {
  id: string
  session_id: string
  role: string
  content: string
  created_at: number
}

/** 最近工作流执行。 */
export interface RecentExecution {
  id: string
  workflow_id: string
  status: string
  started_at: number | null
  finished_at: number | null
  error_msg: string | null
}

export interface Stats {
  // 累计
  memory_episodes: number
  memory_facts: number
  memory_procedures: number
  cron_jobs: number
  ai_tools_total: number
  // 今日
  today_sessions: number
  today_messages: number
  today_tokens: number
  // 最近活动
  recent_messages: RecentMessage[]
  recent_executions: RecentExecution[]
  // 运行时
  system: SystemInfo | null
}

/** 趋势数据（GET /api/v1/dashboard/trend?range=N）。 */
export interface TrendData {
  days: string[]
  sessions: number[]
  messages: number[]
  tokens: number[]
}

/** token 趋势聚合粒度：today=hour（24 桶），其余=day。 */
export type TokenTrendGranularity = 'hour' | 'day'

/** token 趋势时间范围。 */
export type TokenTrendScope = 'today' | 'week' | 'month' | 'custom'

/**
 * token 消耗趋势（GET /api/v1/dashboard/token-trend）。
 *
 * Labels 与三条折线等长对齐；今日 24 小时桶，其余按天递增。
 */
export interface TokenTrendData {
  scope: TokenTrendScope
  granularity: TokenTrendGranularity
  start_at: number
  end_at: number
  labels: string[]
  input: number[]
  output: number[]
  cache_read: number[]
  total: number
  /** 区间费用合计（USD；未配置模型单价时为 0）。 */
  cost_usd: number
}

/** 趋势查询参数；custom 时传 start_at/end_at（毫秒，精确到天）。 */
export interface TokenTrendQuery {
  scope: TokenTrendScope
  start_at?: number
  end_at?: number
}

/**
 * Dashboard store（Pinia 重构）。
 *
 * <p>职责：仪表盘各领域数据概览（只读）。
 */
export const useDashboardStore = defineStore('dashboard', () => {
  const stats = ref<Stats | null>(null)
  const trend = ref<TrendData | null>(null)
  const trendRange = ref(7)
  const tokenTrend = ref<TokenTrendData | null>(null)
  const tokenScope = ref<TokenTrendScope>('today')
  const error = ref<string | null>(null)

  /** 加载各领域数据概览。 */
  async function load(): Promise<void> {
    error.value = null
    try {
      stats.value = await apiGet<Stats>('/api/v1/dashboard/stats')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 加载最近 N 天趋势（失败静默，视图显示空图）。 */
  async function loadTrend(range: number = trendRange.value): Promise<void> {
    trendRange.value = range
    try {
      trend.value = await apiGet<TrendData>(`/api/v1/dashboard/trend?range=${range}`)
    } catch {
      trend.value = null
    }
  }

  /** 加载 token 消耗趋势（失败静默，视图显示空图）。 */
  async function loadTokenTrend(query: TokenTrendQuery = { scope: tokenScope.value }): Promise<void> {
    tokenScope.value = query.scope
    const params = new URLSearchParams({ scope: query.scope })
    if (query.scope === 'custom' && query.start_at && query.end_at) {
      params.set('start_at', String(query.start_at))
      params.set('end_at', String(query.end_at))
    }
    try {
      tokenTrend.value = await apiGet<TokenTrendData>(`/api/v1/dashboard/token-trend?${params.toString()}`)
    } catch {
      tokenTrend.value = null
    }
  }

  return {
    stats,
    trend,
    trendRange,
    tokenTrend,
    tokenScope,
    error,
    load,
    loadTrend,
    loadTokenTrend
  }
})
