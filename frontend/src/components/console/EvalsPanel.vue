<script setup lang="ts">
// 评估页签：打分、存基线、看回归对比（specs/11）。
import { onMounted, ref } from 'vue'
import * as api from '../../api'
import type { BaselineVO, EvalVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'
import { useToastStore } from '../../stores/toast'
import { useDrawer } from '../../composables/useDrawer'

const toast = useToastStore()
const evals = ref<EvalVO[]>([])
const baselines = ref<BaselineVO[]>([])
const loading = ref(true)
const error = ref('')

const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}K` : String(n))
const when = (ms: number) => (ms ? new Date(ms).toLocaleDateString('zh-CN') : '')

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[evals.value, baselines.value] = await Promise.all([
      api.evals.list(),
      api.evals.baselines(),
    ])
  } catch (e) {
    error.value = (e as Error)?.message || '读不到评估记录'
  } finally {
    loading.value = false
  }
}
onMounted(load)

// 打分 1–5，点星即存；改分不动客观指标（后端保证）。
// scoring 记录正在打分的那条，await 期间禁用同行其他星，连点不会写坏。
const scoring = ref('')
const baselineSaving = ref('')
const baselineRemoving = ref('')

async function score(e: EvalVO, s: number) {
  if (scoring.value) return
  scoring.value = e.id
  try {
    await api.evals.register({ run_id: e.run_id, score: s, note: e.note })
    await load()
  } catch (err) {
    toast.bad(`打分失败：${(err as Error)?.message || '请重试'}`)
  } finally {
    scoring.value = ''
  }
}
async function saveBaseline(e: EvalVO) {
  if (baselineSaving.value) return
  baselineSaving.value = e.id
  try {
    await api.evals.saveBaseline(e.id, e.scope)
    toast.ok('已存为基线，之后的运行会跟它比')
    await load()
  } catch (err) {
    toast.bad(`存基线失败：${(err as Error)?.message || '请重试'}`)
  } finally {
    baselineSaving.value = ''
  }
}
async function removeBaseline(id: string) {
  if (baselineRemoving.value) return
  baselineRemoving.value = id
  try {
    await api.evals.removeBaseline(id)
    await load()
  } catch (err) {
    toast.bad(`删除失败：${(err as Error)?.message || '请重试'}`)
  } finally {
    baselineRemoving.value = ''
  }
}
// 对比：选一条基线与一条记录，并排看指标差与结果摘要。
const cmpOpen = ref(false)
const cmpBase = ref('')
const cmpRun = ref('')
const cmp = ref<Awaited<ReturnType<typeof api.evals.compare>> | null>(null)
const cmpBusy = ref(false)
const { drawerRef } = useDrawer(cmpOpen, () => (cmpOpen.value = false))

async function doCompare() {
  if (cmpBusy.value) return
  cmpBusy.value = true
  try {
    cmp.value = await api.evals.compare(cmpBase.value, cmpRun.value)
  } catch (e) {
    toast.bad(`对比失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    cmpBusy.value = false
  }
}
function openCompare(e: EvalVO) {
  const b = baselines.value.find((x) => x.scope === e.scope && x.scope_id === e.scope_id)
  if (!b) {
    toast.bad('这个范围还没有基线，先存一条')
    return
  }
  cmpBase.value = b.id
  cmpRun.value = e.run_id
  cmpOpen.value = true
  void doCompare()
}
</script>

<template>
  <PageState :loading="loading" :error="error">
    <header class="pane-head">
      <div>
        <h2>评估</h2>
        <p>每次执行自动记下 token、耗时、工具成功率；你补一个 1–5 分。存成基线，退步了一眼看见。</p>
      </div>
    </header>

    <div v-if="!evals.length" class="card blank">
      <span class="bt"><AppIcon name="chart" size="ic-lg" /></span>
      <b>还没有评估记录</b>
      <p>在「任务」里跑几步，这里就会自动出现每次的客观指标。</p>
    </div>

    <table v-else class="tbl">
      <thead>
        <tr>
          <th>运行</th>
          <th>分数</th>
          <th>用量</th>
          <th>工具</th>
          <th>与基线</th>
          <th>时间</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in evals" :key="e.id">
          <td class="mono dim">
            {{ e.run_id.slice(-6) }}
            <!-- 来源（团队 / 任务）已删除：记录保留但无出处，标出来别让用户对着一行查不到来源的数据发懵 -->
            <span v-if="e.orphaned" class="tag muted" title="这个来源（团队 / 任务）已经被删除">来源已删</span>
          </td>
          <td>
            <span class="stars">
              <button
                v-for="s in 5"
                :key="s"
                class="star"
                :class="{ on: e.score >= s, 'is-loading': scoring === e.id }"
                type="button"
                :title="`${s} 分`"
                :disabled="!!scoring"
                @click="score(e, s)"
              >
                ★
              </button>
            </span>
          </td>
          <td>{{ fmt(e.metrics.input_tokens) }} / {{ fmt(e.metrics.output_tokens) }}</td>
          <td>{{ e.metrics.tool_calls }} 次{{ e.metrics.tool_failures ? `（失 ${e.metrics.tool_failures}）` : '' }}</td>
          <td>
            <span v-if="e.score_delta" :class="e.score_delta > 0 ? 'up' : 'down'">
              {{ e.score_delta > 0 ? '+' : '' }}{{ e.score_delta }}
            </span>
            <span v-else class="dim">—</span>
          </td>
          <td class="dim">{{ when(e.created_at) }}</td>
          <td class="ops">
            <button
              class="btn btn-sm btn-ghost"
              type="button"
              :class="{ 'is-loading': baselineSaving === e.id }"
              :disabled="!!baselineSaving || !!baselineRemoving"
              @click="saveBaseline(e)"
            >
              存基线
            </button>
            <button class="btn btn-sm btn-ghost" type="button" :disabled="!!baselineSaving || !!baselineRemoving || cmpOpen" @click="openCompare(e)">对比</button>
          </td>
        </tr>
      </tbody>
    </table>

    <section v-if="baselines.length" class="card base-list">
      <h3>当前基线</h3>
      <div v-for="b in baselines" :key="b.id" class="base-row">
        <span class="mono dim">{{ b.eval_id.slice(-6) }}</span>
        <span class="tag">{{ b.scope === 'task' ? '按任务' : '按团队' }}</span>
        <span>{{ b.score }} 分</span>
        <button
          class="btn btn-sm btn-ghost is-danger-text"
          type="button"
          :class="{ 'is-loading': baselineRemoving === b.id }"
          :disabled="!!baselineRemoving || !!baselineSaving"
          @click="removeBaseline(b.id)"
        >
          删除
        </button>
      </div>
    </section>

    <Transition name="scrim">
      <div v-if="cmpOpen" class="scrim-side no-scrim-anim" @click.self="cmpOpen = false">
        <Transition name="drawer" appear>
          <section v-if="cmpOpen" ref="drawerRef" class="drawer-side is-2xl" role="dialog" aria-modal="true" aria-label="回归对比">
            <header class="drawer-head">
              <h3>与基线对比</h3>
              <button class="btn btn-sm btn-ghost" type="button" @click="cmpOpen = false">关闭</button>
            </header>
            <div v-if="cmpBusy" class="dim">正在对比…</div>
            <template v-else-if="cmp">
              <div class="cmp-kpis">
                <div class="kpi">
                  <span class="k">分数差</span>
                  <b :class="cmp.score_delta >= 0 ? 'up' : 'down'">
                    {{ cmp.score_delta > 0 ? '+' : '' }}{{ cmp.score_delta }}
                  </b>
                </div>
                <div class="kpi">
                  <span class="k">总 token 差</span>
                  <b :class="cmp.total_delta <= 0 ? 'up' : 'down'">
                    {{ cmp.total_delta > 0 ? '+' : '' }}{{ fmt(cmp.total_delta) }}
                  </b>
                </div>
              </div>
              <div class="cmp-cols">
                <div>
                  <h4>基线那次</h4>
                  <p class="summary">{{ cmp.base_summary || '（没有留下正文）' }}</p>
                </div>
                <div>
                  <h4>这次</h4>
                  <p class="summary">{{ cmp.run_summary || '（没有留下正文）' }}</p>
                </div>
              </div>
            </template>
          </section>
        </Transition>
      </div>
    </Transition>
  </PageState>
</template>

<style scoped>
.pane-head { margin-bottom: var(--wb-sp-5); }
.pane-head h2 { font-size: var(--wb-fs-xl); color: var(--wb-ink); }
.pane-head p { font-size: var(--wb-fs-sm); color: var(--wb-muted); margin-top: var(--wb-sp-1); max-width: var(--wb-w-desc); }
.blank {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
  color: var(--wb-muted);
}
.blank b { color: var(--wb-ink); }
.stars { display: inline-flex; gap: 2px; }
.star {
  background: none;
  border: 0;
  cursor: pointer;
  font-size: var(--wb-fs-md);
  color: var(--wb-line-2);
  padding: 0 1px;
  transition: transform var(--wb-dur-fast) var(--wb-ease), color var(--wb-dur-fast) var(--wb-ease);
}
/* 悬停 1.06 / 按下 0.88：星级是小目标，比主按钮的 1.03/0.96 幅度更大才看得见。
   一排五颗星在 1.15 下会明显左右晃，1.06 是「邀请」与「不打断」的平衡点 */
.star:hover:not(:disabled) { transform: scale(1.06); color: var(--wb-primary); }
.star:active:not(:disabled) { transform: scale(0.88); }
.star:disabled { opacity: 0.45; cursor: not-allowed; }
/* 打分进行中：整排星呼吸，不是转圈——星星转圈读起来像「正在换星」 */
.star.is-loading { animation: star-busy 1.1s ease-in-out infinite; cursor: progress; }
@keyframes star-busy {
  50% { opacity: 0.45; }
}
.star.on { color: var(--wb-primary); }
.ops { display: flex; gap: var(--wb-sp-2); }
.up { color: var(--wb-success); font-weight: 700; }
.down { color: var(--wb-danger); font-weight: 700; }
.base-list { margin-top: var(--wb-sp-5); padding: var(--wb-sp-4); }
.base-list h3 { font-size: var(--wb-fs-md); color: var(--wb-ink); margin-bottom: var(--wb-sp-3); }
.base-row { display: flex; align-items: center; gap: var(--wb-sp-3); padding: var(--wb-sp-2) 0; font-size: var(--wb-fs-sm); }
.drawer-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--wb-sp-4); }
.drawer-head h3 { font-size: var(--wb-fs-lg); color: var(--wb-ink); }
.cmp-kpis { display: flex; gap: var(--wb-sp-4); margin-bottom: var(--wb-sp-5); }
.kpi {
  flex: 1;
  padding: var(--wb-sp-4);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.kpi .k { font-size: var(--wb-fs-label); color: var(--wb-muted); font-weight: 600; }
.kpi b { font-size: var(--wb-fs-xl); }
.cmp-cols { display: grid; grid-template-columns: 1fr 1fr; gap: var(--wb-sp-4); }
.cmp-cols h4 { font-size: var(--wb-fs-label); color: var(--wb-ink-2); margin-bottom: var(--wb-sp-2); }
.summary {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius-sm);
  padding: var(--wb-sp-3);
  max-height: 260px;
  overflow: auto;
  white-space: pre-wrap;
}
.dim { color: var(--wb-muted); }
.is-danger-text { color: var(--wb-danger); }
</style>
