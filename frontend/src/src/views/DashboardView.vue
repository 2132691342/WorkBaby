<script setup lang="ts">
// 用量仪表盘：一行读数 + 近 N 天柱状图 + 按模型占比 + 用量最高的会话。
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../api'
import { useSessionStore } from '../stores/session'
import type { StatsRESP } from '../types/api'
import { fmtCount, fmtMs } from '../utils/num'
import AppIcon from '../components/common/AppIcon.vue'
import AppSidebar from '../components/common/AppSidebar.vue'
import PageState from '../components/common/PageState.vue'
import ShareBars from '../components/dashboard/ShareBars.vue'
import UsageChart from '../components/dashboard/UsageChart.vue'

const SPANS = [7, 14, 30]

const router = useRouter()
const session = useSessionStore()
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
    error.value = (e as Error)?.message || '读不到用量数据，稍后再试试'
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
// 没有任何用量时不画空图表：一张空的柱状图只会让人以为坏了
const hasUsage = computed(() => (totals.value?.total || 0) > 0)
const topSessions = computed(() => (data.value?.sessions || []).slice(0, 6))
const hitRate = computed(() => (totals.value?.cache_hit_rate || 0) * 100)

// 读数卡片：右栏一组小卡，每张一个主读数 + 一行注解。抽成数据而不是铺开六个块，
// 是不想让模板里出现「同一张卡抄六遍」——抄错一处就六处分叉。
const metrics = computed(() => {
  const t = totals.value
  return [
    { key: 'total', value: fmtCount(t?.total || 0), label: '总 token', note: `模型调用 ${t?.calls || 0} 次` },
    {
      key: 'io',
      value: `${fmtCount(t?.input || 0)} / ${fmtCount(t?.output || 0)}`,
      label: '输入 / 输出',
      note: '你发的 / 助手回的',
    },
    {
      key: 'hit',
      value: `${hitRate.value.toFixed(hitRate.value >= 10 ? 0 : 1)}%`,
      label: '缓存命中率',
      note: `${fmtCount(t?.cached || 0)} 个输入 token 走了缓存`,
      hit: hitRate.value > 0,
    },
    {
      key: 'ctx',
      value: fmtCount(t?.avg_context || 0),
      label: '平均上下文',
      note: `峰值 ${fmtCount(t?.peak_context || 0)} token`,
    },
    { key: 'sessions', value: String(t?.sessions || 0), label: '会话数', note: '有对话记录的会话' },
    {
      key: 'latency',
      value: fmtMs(t?.avg_latency_ms || 0),
      label: '平均耗时',
      note: `合计 ${fmtMs(t?.latency_ms || 0)}`,
    },
  ]
})

async function openSession(id: string) {
  await session.open(id)
  router.push('/')
}

async function newSession() {
  await session.create({})
  router.push('/')
}
</script>

<template>
  <div class="dash wb-ui">
    <AppSidebar @new-session="newSession" />

    <div class="dash-main scroll">
      <div class="wrap">
        <div class="page-head">
          <div>
            <h1>用量</h1>
            <p>最近这段时间，助手替你花了多少</p>
          </div>
          <span class="sp" />
          <div class="seg">
            <button
              v-for="d in SPANS"
              :key="d"
              type="button"
              :class="{ on: days === d }"
              @click="days = d"
            >
              {{ d }} 天
            </button>
          </div>
        </div>

        <PageState :loading="loading" :error="error">
          <div v-if="!hasUsage" class="card blank">
            <div class="blank-in">
              <AppIcon name="chart" size="ic-lg" />
              <div>
                <b>还没有用量记录</b>
                <p>和助手聊两句，这里就会记下你花了多少。</p>
              </div>
              <button class="btn btn-primary" type="button" @click="router.push('/')">去聊两句</button>
            </div>
          </div>

          <div v-else class="cols">
            <!-- 左栏（主）：趋势与分布，占宽多一点是为了让图表读得清 -->
            <div class="col col-main">
              <div class="card chart-card">
                <div class="card-head">
                  <h2>每天用了多少</h2>
                  <div class="legend">
                    <span><i class="sw in" /> 输入</span>
                    <span><i class="sw out" /> 输出</span>
                  </div>
                </div>
                <UsageChart :daily="data?.daily || []" />
              </div>

              <div class="card">
                <div class="card-head">
                  <h2>按模型</h2>
                </div>
                <ShareBars :items="data?.models || []" />
                <p v-if="!data?.models.length" class="t-sub none">还没有模型调用记录</p>
              </div>
            </div>

            <!-- 右栏（次）：读数小卡 + 会话排行 -->
            <div class="col col-side">
              <div class="metrics">
                <article v-for="m in metrics" :key="m.key" class="metric">
                  <div class="mv" :class="{ 'is-hit': m.hit }">{{ m.value }}</div>
                  <div class="ml">{{ m.label }}</div>
                  <div class="md">{{ m.note }}</div>
                </article>
              </div>

              <div class="card">
                <div class="card-head">
                  <h2>用量最高的会话</h2>
                </div>
                <div v-if="topSessions.length" class="rowlist">
                  <button
                    v-for="s in topSessions"
                    :key="s.session_id"
                    class="rli ses"
                    type="button"
                    :title="`${s.title || '未命名对话'} · ${fmtCount(s.total)} token`"
                    @click="openSession(s.session_id)"
                  >
                    <AppIcon name="doc" size="ic-xs" />
                    <span class="nm">{{ s.title || '未命名对话' }}</span>
                    <span class="pct">{{ fmtCount(s.total) }}</span>
                  </button>
                </div>
                <p v-else class="t-sub none">还没有会话用量</p>
              </div>
            </div>
          </div>
        </PageState>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dash {
  display: flex;
  height: 100%;
  min-height: 0;
  /* 同 ChatView：flex 子项不写 flex:1 会塌成内容宽度 */
  flex: 1;
  min-width: 0;
}
.dash-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}
.wrap {
  /* width:100% 与 max-width 同时给：只给 max-width 时 flex 子项会退回内容宽度 */
  width: 100%;
  max-width: 1080px;
  margin: 0 auto;
  padding: var(--wb-sp-8) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.wrap > * {
  min-width: 0;
}
/* 页面头窄屏换行：标题在上，时间段切换在下，不再挤成一行 */
.page-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
  margin-bottom: var(--wb-sp-2);
}
.page-head > div:first-child {
  min-width: 0;
}
.page-head .seg {
  flex: none;
}
@media (max-width: 720px) {
  .wrap {
    padding: var(--wb-sp-5) var(--wb-sp-4) var(--wb-sp-8);
  }
}
/* 没有数据时也要留住页面结构：一个居中的大空白会让人以为坏了 */
.blank {
  padding: var(--wb-sp-6);
}
.blank-in {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  color: var(--wb-muted);
}
.blank-in .grow,
.blank-in b {
  color: var(--wb-ink);
  font-size: var(--wb-fs-md);
  font-weight: 600;
}
.blank-in p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.blank-in .btn {
  margin-left: auto;
}
.page-head .sp {
  flex: 1;
}
.card-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  margin-bottom: var(--wb-sp-4);
}
/* 命中率 >0 才上色：常显的绿色等于没有绿色 */
.metric .mv.is-hit {
  color: var(--wb-success);
}
.card-head h2 {
  font-size: var(--wb-fs-md);
  font-weight: 600;
  color: var(--wb-ink);
}
.card-head .legend {
  margin-left: auto;
}
.legend .sw.in {
  background: var(--wb-primary);
}
.legend .sw.out {
  background: var(--wb-ch-2);
}
/* 卡片式两栏：左主（图表 / 分布）右次（读数 / 排行）。
   用 fr 而不是固定像素：窄窗口按比例收缩，永远不出横向滚动条。 */
.cols {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
  gap: var(--wb-sp-4);
  align-items: start;
}
.col {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-3);
}
.metric {
  min-width: 0;
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line-2);
}
.mv {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 600;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
  /* 长数字（百万级）要有退路：anywhere 只在必要时断行，不断就让卡片被撑破 */
  overflow-wrap: anywhere;
}
.ml {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.md {
  margin-top: 2px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
/* 图表卡的悬浮提示要能探出卡片，否则首尾两天读不到 */
.chart-card {
  overflow: visible;
}
.none {
  color: var(--wb-muted);
}
.ses {
  width: 100%;
  text-align: left;
  cursor: pointer;
  gap: var(--wb-sp-2);
}
.ses .nm {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.ses .pct {
  flex: none;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
/* 窄到放不下两栏就叠成一栏：先塌布局，再塌读数网格，避免「两栏各剩 200px」 */
@media (max-width: 1000px) {
  .cols {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 520px) {
  .metrics {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
