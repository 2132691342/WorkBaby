<script setup lang="ts">
// 知识库面板：添加文档 → 自动切分建索引 → 可随时检索或重建。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ReindexJobVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const store = useSettingsStore()
const toast = useToastStore()
// 忙碌标识带动作与目标：转圈只出现在真正在跑的那个按钮上
const busyTarget = ref('')
const busy = computed(() => busyTarget.value !== '')
const query = ref('')
const hits = ref<Array<{ title: string; content: string }>>([])
const error = ref('')

const statusName: Record<string, string> = {
  pending: '待处理',
  indexed: '已索引',
  failed: '失败',
}

// 一次选中就建索引：分两步走会让新手以为第一次点击没生效
async function pickAndAdd() {
  error.value = ''
  busyTarget.value = 'add'
  try {
    const { OpenFileDialog } = await import('../../../wailsjs/go/main/App')
    const path = await OpenFileDialog('选择要放进知识库的文档', '')
    if (!path) return
    const r = await api.knowledge.add([path])
    if (!r.added)
      error.value = '这个文件没法识别，支持：md / txt / csv / log / json / yml / yaml / html / pdf / docx / xlsx'
    else {
      await store.loadDocs()
      toast.ok('文档已添加，正在自动建立索引')
    }
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busyTarget.value = ''
  }
}

// 删除两段式确认：第一次点进入确认态，3 秒不点自动还原
const confirming = ref('')
let confirmTimer = 0
function askRemove(id: string) {
  if (confirming.value === id) {
    confirming.value = ''
    void removeDoc(id)
    return
  }
  confirming.value = id
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirming.value = ''), 3000)
}
onBeforeUnmount(() => clearTimeout(confirmTimer))

async function removeDoc(id: string) {
  busyTarget.value = `remove:${id}`
  error.value = ''
  try {
    await api.knowledge.remove(id)
    await store.loadDocs()
    toast.ok('文档已删除')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busyTarget.value = ''
  }
}

// 重建是后台任务：大库要跑几十秒，同步等一个请求就把这条连接占死了，
// 期间界面只有一个转圈，也没办法停。启动 → 轮询进度 → 可取消。
const job = ref<ReindexJobVO | null>(null)
let pollTimer = 0
const jobRunning = computed(() => job.value?.running === true)
const jobPercent = computed(() => {
  const j = job.value
  if (!j || j.total === 0) return 0
  return Math.min(100, Math.round(((j.done + j.failed) / j.total) * 100))
})

function stopPolling() {
  clearInterval(pollTimer)
  pollTimer = 0
}

onBeforeUnmount(stopPolling)

async function reindex() {
  if (jobRunning.value || busy) return
  error.value = ''
  try {
    const r = await api.knowledge.reindex()
    if (!r.started) {
      toast.info('已经有一趟重建在跑了')
      return
    }
    job.value = { running: true, total: 0, done: 0, failed: 0, started_at: 0, finished_at: 0 }
    stopPolling()
    pollTimer = window.setInterval(pollJob, 700)
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function pollJob() {
  try {
    const j = await api.knowledge.reindexStatus()
    job.value = j
    if (j.running) return
    stopPolling()
    await store.loadDocs()
    // 跑到一半被取消要如实说：文档列表里会留着上一版的块数，
    // 不提一句用户会以为全部重建完了。
    if (j.total > 0 && j.done + j.failed < j.total) {
      toast.info(`重建已中断（${j.done + j.failed}/${j.total}）`)
    } else if (j.failed > 0) {
      toast.ok(`索引已重建：成功 ${j.done} 个，失败 ${j.failed} 个`)
    } else {
      toast.ok(`索引已重建（${j.done} 个文档）`)
    }
  } catch (e) {
    stopPolling()
    error.value = (e as Error).message
  }
}

async function cancelReindex() {
  if (!jobRunning.value) return
  try {
    await api.knowledge.cancelReindex()
  } catch (e) {
    error.value = (e as Error).message
  }
}

const searching = ref(false)
async function search() {
  if (!query.value.trim()) return
  searching.value = true
  try {
    const r = await api.knowledge.search(query.value.trim(), 5)
    hits.value = r.hits
    if (!r.hits.length) toast.info('知识库里没找到相关内容')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    searching.value = false
  }
}

onMounted(() => store.loadDocs())
</script>

<template>
  <div class="kb">
    <div class="toolbar">
      <button
        class="btn btn-primary"
        type="button"
        :class="{ 'is-loading': busyTarget === 'add' }"
        :disabled="busy"
        @click="pickAndAdd"
      >
        <AppIcon name="plus" size="ic-xs" /> 添加文档
      </button>
      <button
        class="btn"
        type="button"
        :class="{ 'is-loading': jobRunning && !job?.total }"
        :disabled="jobRunning"
        @click="reindex"
      >
        <AppIcon v-if="!jobRunning" name="refresh" size="ic-xs" />
        {{ jobRunning ? '重建中…' : '重建索引' }}
      </button>
      <span class="spacer" />
      <span class="count">{{ store.docs.length }} 个文档</span>
    </div>

    <!-- 重建进度：有进度条才有「还能等多久」，只有转圈的话用户只能干等或乱点 -->
    <div v-if="jobRunning" class="job">
      <div class="job-bar" role="progressbar" :aria-valuenow="jobPercent" aria-valuemin="0" aria-valuemax="100">
        <span class="job-fill" :style="{ width: `${jobPercent}%` }" />
      </div>
      <span class="job-tx mono">
        {{ (job?.done || 0) + (job?.failed || 0) }}/{{ job?.total || 0 }}
        <template v-if="job?.failed"> · 失败 {{ job.failed }}</template>
      </span>
      <button class="btn btn-sm btn-ghost" type="button" @click="cancelReindex">停止</button>
    </div>

    <p v-if="error" class="alert a-warn">
      <AppIcon name="info" />
      <div>{{ error }}</div>
    </p>

    <div v-if="store.docs.length" class="search">
      <input
        v-model="query"
        class="input"
        placeholder="在知识库里搜点什么…"
        @keydown.enter="search"
      />
      <button
        class="btn"
        type="button"
        :class="{ 'is-loading': searching }"
        :disabled="searching"
        @click="search"
      >
        搜索
      </button>
    </div>

    <div v-if="hits.length" class="hits">
      <div v-for="(h, i) in hits" :key="i" class="hit">
        <div class="hit-t">{{ h.title }}</div>
        <div class="hit-c">{{ h.content }}</div>
      </div>
    </div>

    <PageState
      :loading="store.docsLoading"
      :error="store.error"
      :empty="!store.docs.length"
      empty-title="知识库还是空的"
      empty-sub="点上面的「添加文档」，把常用资料放进来，助手回答时就会去查"
    >
      <div class="rowlist">
        <div v-for="d in store.docs" :key="d.id" class="rli">
          <div class="grow">
            <h5>{{ d.title }}</h5>
            <p class="mono" :title="d.path">{{ d.path }}</p>
            <p v-if="d.error" class="err">{{ d.error }}</p>
          </div>
          <div class="flex-r nowrap">
            <span class="tag" :class="{ 'b-success': d.status === 'indexed', 'b-danger': d.status === 'failed' }">
              {{ statusName[d.status] || d.status }} · {{ d.chunks }} 块
            </span>
            <button
              class="btn btn-sm btn-danger-ghost"
              :class="{ 'is-loading': busyTarget === `remove:${d.id}` }"
              :disabled="busy"
              @click="askRemove(d.id)"
            >
              {{ confirming === d.id ? '确认删除' : '删除' }}
            </button>
          </div>
        </div>
      </div>
    </PageState>
  </div>
</template>

<style scoped>
.kb {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-width: 0;
}
/* 重建进度条：主色实条 + 中性轨道，不加装饰——它要说的是「还剩多少」 */
.job {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
}
.job-bar {
  flex: 1;
  min-width: 0;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-sunken);
  overflow: hidden;
}
.job-fill {
  display: block;
  height: 100%;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  transition: width var(--wb-dur) var(--wb-ease);
}
.job-tx {
  flex: none;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.toolbar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  min-width: 0;
}
.count {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  white-space: nowrap;
}
.search {
  display: flex;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.search .input {
  flex: 1;
  min-width: 0;
}
.err {
  color: var(--wb-danger);
}
.hits {
  display: grid;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.hit {
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-surface-2);
  border: 1px solid var(--wb-border);
  min-width: 0;
}
.hit-t {
  font-weight: 600;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.hit-c {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  margin-top: 2px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
