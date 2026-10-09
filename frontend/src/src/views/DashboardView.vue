<script setup lang="ts">
// 用量仪表盘：一行读数 → 趋势折线图 → 按模型 / 会话两个分布卡。
// 节奏照「案例」走：先给四个关键读数（横排、各自独立），再给一张全宽大图，
// 最后才是两个分布小卡。竖着一路堆卡片会让人不知道该先看哪。
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
// 没有任何用量时不画空图表：一张空的折线图只会让人以为坏了
const hasUsage = computed(() => (totals.value?.total || 0) > 0)
const topSessions = computed(() => (data.value?.sessions || []).slice(0, 6))
// 上限 100%：老数据的输入量按「未命中部分」记，命中量可能大于输入量（后端已归一，这里兜显示）
const hitRate = computed(() => Math.min(100, (totals.value?.cache_hit_rate || 0) * 100))

// 四个读数卡：每张一个主读数 + 标签 + 一行注解。
// 抽成数据而不是把模板抄四遍——抄错一处就是四处分叉。
const kpis = computed(() => {
  const t = totals.value
  return [
    {
      key: 'total',
      icon: 'chart',
      value: fmtCount(t?.total || 0),
      label: '总 token',
      note: `模型调用 ${t?.calls || 0} 次`,
    },
    {
      key: 'io',
      icon: 'arrow-down-up',
      value: `${fmtCount(t?.input || 0)} / ${fmtCount(t?.output || 0)}`,
      label: '输入 / 输出',
      note: '你发的 / 助手回的',
    },
    {
      key: 'hit',
      icon: 'zap',
      value: `${hitRate.value.toFixed(hitRate.value >= 10 ? 0 : 1)}%`,
      label: '缓存命中率',
      note: `${fmtCount(t?.cached || 0)} 个输入 token 走了缓存`,
      hit: hitRate.value > 0,
    },
    {
      key: 'latency',
      icon: 'clock',
      value: fmtMs(t?.avg_latency_ms || 0),
      label: '平均耗时',
      note: `合计 ${fmtMs(t?.latency_ms || 0)}`,
    },
  ]
})

// 上下文水位与会话数不进读数卡：它们是背景信息，别和「花了多少」抢同一排格子
const subline = computed(() => {
  const t = totals.value
  if (!t) return ''
  return `平均上下文 ${fmtCount(t.avg_context)} · 峰值 ${fmtCount(t.peak_context)} · 有记录的会话 ${t.sessions} 个`
})

// 打开会话要等 deserialization：async 期间不给反馈，用户会以为点空了
const opening = ref('')
async function openSession(id: string) {
  if (opening.value) return
  opening.value = id
  try {
    await session.open(id)
    router.push('/')
  } finally {
    opening.value = ''
  }
}

// 建会话要走「建 + 拉列表 + 取快照」，期间按钮必须自己说明在忙。
const creating = ref(false)
const createErr = ref('')
async function newSession() {
  if (creating.value) return
  creating.value = true
  createErr.value = ''
  try {
    await session.create({})
    router.push('/')
  } catch (e) {
    createErr.value = (e as Error)?.message || '新建对话失败，请重试'
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="dash wb-ui">
    <AppSidebar :creating="creating" @new-session="newSession" />

    <div class="dash-main scroll">
      <div class="wrap">
        <header class="page-head">
          <div class="ph-txt">
            <h1>用量</h1>
            <p>最近 {{ days }} 天，助手替你花了多少</p>
          </div>
          <span class="sp" />
          <div class="seg" role="group" aria-label="统计区间">
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
        </header>

        <p v-if="createErr" class="alert is-bad">{{ createErr }}</p>

        <PageState :loading="loading" :error="error">
          <div v-if="!hasUsage" class="card blank">
            <div class="blank-in">
              <span class="bt"><AppIcon name="chart" size="ic-lg" /></span>
              <div>
                <b>还没有用量记录</b>
                <p>和助手聊两句，这里就会记下你花了多少。</p>
              </div>
              <button class="btn btn-primary" type="button" @click="router.push('/')">去聊两句</button>
            </div>
          </div>

          <template v-else>
            <!-- 四个关键读数：横排，各自独立成卡 -->
            <div class="kpis">
              <article v-for="k in kpis" :key="k.key" class="kpi">
                <span class="kt"><AppIcon :name="k.icon" size="ic-sm" /></span>
                <div class="kb">
                  <div class="kv" :class="{ 'is-hit': k.hit }">{{ k.value }}</div>
                  <div class="kl">{{ k.label }}</div>
                  <div class="kn">{{ k.note }}</div>
                </div>
              </article>
            </div>

            <!-- 趋势：一张全宽折线图 -->
            <section class="card chart-card">
              <div class="card-head">
                <h2>用量趋势</h2>
                <div class="legend">
                  <span><i class="sw in" /> 输入</span>
                  <span><i class="sw out" /> 输出</span>
                </div>
              </div>
              <UsageChart :daily="data?.daily || []" />
              <p v-if="subline" class="card-foot">{{ subline }}</p>
            </section>

            <!-- 两个分布卡 -->
            <div class="cols">
              <section class="card">
                <div class="card-head">
                  <h2>按模型</h2>
                </div>
                <ShareBars :items="data?.models || []" />
                <p v-if="!data?.models.length" class="none">还没有模型调用记录</p>
              </section>

              <section class="card">
                <div class="card-head">
                  <h2>用量最高的会话</h2>
                </div>
                <div v-if="topSessions.length" class="rowlist">
                  <button
                    v-for="s in topSessions"
                    :key="s.session_id"
                    class="rli ses"
                    type="button"
                    :disabled="opening !== ''"
                    :class="{ 'is-loading': opening === s.session_id }"
                    :title="`${s.title || '未命名对话'} · ${fmtCount(s.total)} token`"
                    @click="openSession(s.session_id)"
                  >
                    <AppIcon name="doc" size="ic-xs" />
                    <span class="nm">{{ s.title || '未命名对话' }}</span>
                    <span class="pct">{{ fmtCount(s.total) }}</span>
                  </button>
                </div>
                <p v-else class="none">还没有会话用量</p>
              </section>
            </div>
          </template>
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
  max-width: 1060px;
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
.page-head {
  display: flex;
  align-items: flex-end;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
  margin-bottom: var(--wb-sp-1);
}
.ph-txt {
  min-width: 0;
}
.page-head h1 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-2xl);
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
}
.page-head p {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.page-head .sp {
  flex: 1;
}
.page-head .seg {
  flex: none;
}

/* ---- 读数卡 ---- */
.kpis {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--wb-sp-3);
}
.kpi {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-3);
  min-width: 0;
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-surface);
  border: 1px solid var(--wb-line-2);
  transition: border-color var(--wb-dur) var(--wb-ease);
}
.kpi:hover {
  border-color: var(--wb-primary-line);
}
.kpi .kt {
  display: grid;
  place-items: center;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  flex: none;
  border-radius: var(--wb-radius-sm);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.kpi .kb {
  min-width: 0;
}
.kpi .kv {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 700;
  line-height: 1.15;
  color: var(--wb-ink);
  /* 百万级数字要有退路：不写 anywhere 会把卡片撑破 */
  overflow-wrap: anywhere;
}
/* 命中率 >0 才上色：常显的绿色等于没有绿色 */
.kpi .kv.is-hit {
  color: var(--wb-success);
}
.kpi .kl {
  margin-top: 1px;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.kpi .kn {
  margin-top: 1px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}

/* ---- 图表卡 ---- */
.card-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  margin-bottom: var(--wb-sp-4);
}
.card-head h2 {
  font-size: var(--wb-fs-md);
  font-weight: 600;
  color: var(--wb-ink);
}
.card-head .legend {
  margin-left: auto;
  display: flex;
  gap: var(--wb-sp-3);
}
.legend span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.sw {
  width: 14px;
  height: 3px;
  border-radius: var(--wb-radius-full);
}
.sw.in {
  background: var(--wb-primary);
}
.sw.out {
  background: var(--wb-ch-2);
}
/* 图表卡的悬浮提示要能探出卡片，否则首尾两天读不到 */
.chart-card {
  overflow: visible;
}
.card-foot {
  margin-top: var(--wb-sp-3);
  padding-top: var(--wb-sp-3);
  border-top: 1px solid var(--wb-line);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}

/* ---- 两个分布卡 ---- */
.cols {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--wb-sp-4);
  align-items: start;
}
.none {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.ses {
  width: 100%;
  text-align: left;
  cursor: pointer;
  gap: var(--wb-sp-2);
}
.ses:hover {
  background: var(--wb-tint);
}
.ses:active {
  transform: scale(0.99);
}
.ses:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 打开中：转圈顶掉图标位，文字与 token 数保留 */
.ses.is-loading {
  pointer-events: none;
  cursor: progress;
}
.ses.is-loading .nm {
  position: relative;
  padding-left: 18px;
}
.ses.is-loading .nm::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  width: 11px;
  height: 11px;
  margin-top: -5.5px;
  border-radius: var(--wb-radius-full);
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  animation: wb-ic-spin 0.8s linear infinite;
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

/* ---- 空态 ---- */
.blank {
  padding: var(--wb-sp-6);
}
.blank-in {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  color: var(--wb-muted);
}
.blank-in .bt {
  display: grid;
  place-items: center;
  width: var(--wb-tile);
  height: var(--wb-tile);
  flex: none;
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
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

/* 窄窗口先塌到两列，再塌到一列：读数卡挤成 4 列时会互相压字 */
@media (max-width: 1080px) {
  .cols {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 860px) {
  .kpis {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 720px) {
  .wrap {
    padding: var(--wb-sp-5) var(--wb-sp-4) var(--wb-sp-8);
  }
  .page-head h1 {
    font-size: var(--wb-fs-xl);
  }
}
</style>
