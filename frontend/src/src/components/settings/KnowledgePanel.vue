<script setup lang="ts">
// 知识库面板：添加文档 → 自动切分建索引 → 可随时检索或重建。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
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

async function reindex() {
  busyTarget.value = 'reindex'
  error.value = ''
  try {
    const r = await api.knowledge.reindex()
    await store.loadDocs()
    toast.ok(`索引已重建（${r.reindexed} 个文档）`)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busyTarget.value = ''
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
        :class="{ 'is-loading': busyTarget === 'reindex' }"
        :disabled="busy"
        @click="reindex"
      >
        <AppIcon name="refresh" size="ic-xs" /> 重建索引
      </button>
      <span class="spacer" />
      <span class="count">{{ store.docs.length }} 个文档</span>
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
