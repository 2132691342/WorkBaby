<script setup lang="ts">
// 运行页签：最近 run 的列表 → 详情（时间线 / 上下文快照 / 检查点）。
// 它回答的是「这一轮到底干了什么、花了多少、从哪能回去」，与对话页的运行态互补：
// 对话页看此刻，这里看历史。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../../api'
import type { RunDetailVO, RunVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import { useDrawer } from '../../composables/useDrawer'
import { useSessionStore } from '../../stores/session'
import { fmtCount, fmtMs } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'
import RunReplay from './RunReplay.vue'
import RunTimeline from './RunTimeline.vue'

const toast = useToastStore()
const route = useRoute()
const router = useRouter()
const session = useSessionStore()

const runs = ref<RunVO[]>([])
const loading = ref(true)
const error = ref('')

const STATUS: Record<string, { text: string; cls: string }> = {
  running: { text: '进行中', cls: 'b-primary' },
  done: { text: '已完成', cls: 'b-ok' },
  failed: { text: '失败', cls: 'b-bad' },
  aborted: { text: '已停止', cls: '' },
}
const LED: Record<string, string> = {
  running: 'is-live',
  done: 'is-ok',
  failed: 'is-bad',
  aborted: 'is-warn',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    runs.value = await api.runs.list('', 100)
  } catch (e) {
    error.value = (e as Error)?.message || '读不到运行记录'
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 详情 ----
const open = ref(false)
const detail = ref<RunDetailVO | null>(null)
const detailLoading = ref(false)
const { drawerRef } = useDrawer(open, () => (open.value = false))

async function show(id: string) {
  open.value = true
  detail.value = null
  detailLoading.value = true
  try {
    detail.value = await api.runs.detail(id)
  } catch (e) {
    toast.bad(`读不到这次运行：${(e as Error)?.message || '请重试'}`)
    open.value = false
  } finally {
    detailLoading.value = false
  }
}

// 深链：从仪表盘点进来时带 ?run=id，进页就展开那一条。
onMounted(() => {
  const id = String(route.query.run || '')
  if (id) void show(id)
})

// 进行中的记录要自己变：列表不会因为后台跑完而刷新。
let timer = 0
onMounted(() => {
  timer = window.setInterval(() => {
    if (runs.value.some((r) => r.status === 'running')) void load()
  }, 4000)
})
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})

const busy = ref('')
// 两个动作落到同一处：PI 没有「把会话叶子挪回去」的协议面，
// 回到某一轮 = 从那一刻分叉出一条新会话，原会话完整保留。
// 分叉完必须跳去对话页并打开新会话——留在指挥台等于什么都没发生。
async function forkFrom(cpId: string) {
  const d = detail.value
  if (!d || busy.value) return
  busy.value = cpId
  try {
    const sess = await api.checkpoints.fork(d.session_id, cpId)
    await session.open(sess.id)
    open.value = false
    await router.push('/')
    toast.ok('已从这一刻开出新会话，原对话还在')
  } catch (e) {
    toast.bad(`分叉失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

function fmtWhen(ts: number): string {
  if (!ts) return '—'
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// 上下文快照画成一根水位条：第几轮涨到多少，压过几次一眼看见。
const peakContext = computed(() => {
  const snaps = detail.value?.snapshots || []
  return snaps.reduce((n, s) => Math.max(n, s.used), 0)
})
</script>

<template>
  <div class="runs wb-ui">
    <header class="page-head">
      <div class="ph-txt">
        <h1>运行记录</h1>
        <p>每一轮对话干了什么、花了多少、从哪能回去</p>
      </div>
      <span class="sp" />
      <button class="btn btn-sm" type="button" :disabled="loading" @click="load()">
        <AppIcon name="refresh" size="ic-xs" /> 刷新
      </button>
    </header>

    <PageState
      :loading="loading"
      :error="error"
      :empty="!loading && !error && !runs.length"
      empty-title="还没有运行记录"
      empty-hint="回对话页说句话，这里就会留下那一次的账"
      @retry="load"
    />

    <ul v-if="!loading && !error && runs.length" class="run-list">
      <li v-for="r in runs" :key="r.id">
        <button class="run-row" type="button" @click="show(r.id)">
          <span class="led" :class="LED[r.status]" />
          <span class="rr-name">{{ r.session_title || '已归档会话' }}</span>
          <span class="rr-model dim">{{ r.model }}</span>
          <span class="tag" :class="STATUS[r.status]?.cls">{{ STATUS[r.status]?.text || r.status }}</span>
          <span class="rr-turns dim">{{ r.turns }} 轮</span>
          <span class="rr-tok mono">{{ fmtCount(r.tokens) }}</span>
          <span class="rr-time dim">{{ fmtWhen(r.started_at) }}</span>
        </button>
      </li>
    </ul>

    <!-- 详情抽屉：点一行就展开，不跳页 -->
    <div v-if="open" ref="drawerRef" class="drawer-mask" @click.self="open = false">
      <aside class="drawer" role="dialog" aria-label="运行详情">
        <header class="drawer-hd">
          <b>{{ detail?.session_title || '已归档会话' }}</b>
          <span class="sp" />
          <button class="btn btn-sm btn-ghost" type="button" @click="open = false">关闭</button>
        </header>
        <div v-if="detailLoading" class="drawer-bd">
          <p class="none">正在读取…</p>
        </div>
        <div v-else-if="detail" class="drawer-bd">
          <div class="kv">
            <div><span>状态</span><b>{{ STATUS[detail.status]?.text || detail.status }}</b></div>
            <div><span>收尾原因</span><b class="mono">{{ detail.stop_reason || '—' }}</b></div>
            <div><span>模型</span><b class="mono">{{ detail.model }}</b></div>
            <div><span>轮次</span><b>{{ detail.turns }}</b></div>
            <div><span>耗时</span><b>{{ detail.duration_ms ? fmtMs(detail.duration_ms) : '还在跑' }}</b></div>
            <div><span>token</span><b class="mono">{{ fmtCount(detail.tokens) }}</b></div>
            <div><span>其中缓存命中</span><b class="mono">{{ fmtCount(detail.cached) }}</b></div>
            <div><span>输出预算</span><b class="mono">{{ fmtCount(detail.max_tokens) }}</b></div>
            <div><span>压缩次数</span><b>{{ detail.compressed }}</b></div>
            <div><span>峰值上下文</span><b class="mono">{{ fmtCount(peakContext) }}</b></div>
          </div>

          <section class="sec">
            <h3>干了什么</h3>
            <p v-if="!detail.timeline?.steps.length && !detail.timeline?.chars" class="none">
              这一轮没调工具，只有正文。
            </p>
            <template v-else>
              <!-- 回放：同一份时间线的另一种看法，按真实时间轴走一遍 -->
              <RunReplay v-if="detail.timeline?.steps.length" :timeline="detail.timeline" />
              <!-- 静态瀑布：一眼看谁拖了后腿，与回放共用同一批 step -->
              <RunTimeline
                v-if="detail.timeline?.steps.length || detail.timeline?.chars"
                :timeline="detail.timeline"
              />
            </template>
          </section>

          <section v-if="detail.snapshots.length" class="sec">
            <h3>上下文水位</h3>
            <ul class="snaps">
              <li v-for="s in detail.snapshots" :key="s.id">
                <span class="sn-turn">第 {{ s.turn }} 轮</span>
                <span class="sn-track">
                  <span
                    class="sn-bar"
                    :style="{ width: `${s.window ? Math.min(100, Math.round((s.used / s.window) * 100)) : 0}%` }"
                  />
                </span>
                <span class="sn-num mono">{{ fmtCount(s.used) }} / {{ fmtCount(s.window) }}</span>
                <span v-if="s.compressed" class="tag b-warn">压过 {{ s.compressed }} 次</span>
              </li>
            </ul>
          </section>

          <section v-if="detail.checkpoints.length" class="sec">
            <h3>回到某一轮</h3>
            <p class="hint">回到那一刻不会删除任何内容；想保留原对话就选「开分支」。</p>
            <ul class="cps">
              <li v-for="cp in detail.checkpoints" :key="cp.id">
                <div class="cp-txt">
                  <b>第 {{ cp.turn + 1 }} 轮前</b>
                  <span class="dim">{{ cp.prompt || '（没有记下当时的提问）' }}</span>
                </div>
                <button
                  class="btn btn-sm"
                  type="button"
                  :disabled="!!busy"
                  @click="forkFrom(cp.id)"
                >
                  {{ busy === cp.id ? '处理中…' : '从这一刻重开' }}
                </button>
              </li>
            </ul>
          </section>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.runs {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.run-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.run-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid transparent;
  border-radius: var(--wb-radius);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.run-row:hover {
  background: var(--wb-tint);
  border-color: var(--wb-line);
  transform: translateX(2px);
}
.run-row:active {
  transform: translateX(2px) scale(0.995);
}
.rr-name {
  flex: none;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.rr-model {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
}
.rr-turns,
.rr-tok,
.rr-time {
  flex: none;
  font-size: var(--wb-fs-xs);
}
.rr-tok {
  width: 70px;
  text-align: right;
}
.rr-time {
  width: 84px;
  text-align: right;
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
.dim {
  color: var(--wb-muted);
}
.none {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}

/* 抽屉：右侧滑入，遮罩点击关闭 */
.drawer-mask {
  position: fixed;
  inset: 0;
  z-index: 40;
  background: rgb(15 23 42 / 24%);
  display: flex;
  justify-content: flex-end;
}
.drawer {
  width: min(560px, 92vw);
  display: flex;
  flex-direction: column;
  background: var(--wb-surface);
  border-left: 1px solid var(--wb-line);
  box-shadow: var(--wb-shadow-2);
  animation: drawer-in var(--wb-dur) var(--wb-ease);
}
@keyframes drawer-in {
  from {
    transform: translateX(24px);
    opacity: 0;
  }
}
.drawer-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
}
.drawer-hd b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.drawer-bd {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.kv {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: var(--wb-sp-2) var(--wb-sp-3);
}
.kv > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.kv span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.kv b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sec h3 {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.hint {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.snaps,
.cps {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.snaps li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
}
.sn-turn {
  flex: none;
  width: 64px;
  color: var(--wb-ink-2);
}
.sn-track {
  position: relative;
  flex: 1;
  min-width: 40px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface-2);
  overflow: hidden;
}
.sn-bar {
  position: absolute;
  inset: 0 auto 0 0;
  background: var(--wb-primary);
  border-radius: var(--wb-radius-full);
}
.sn-num {
  flex: none;
  width: 120px;
  text-align: right;
  color: var(--wb-muted);
}
.cps li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
}
.cp-txt {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: var(--wb-fs-xs);
}
.cp-txt span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
