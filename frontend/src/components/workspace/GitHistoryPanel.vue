<script setup lang="ts">
// 历史面板：提交列表 → 单次提交差异抽屉 → 文件历史抽屉。
// 历史不缓存：实时 git log 是唯一真相源，换分支后历史整块变正是缓存不该存在的原因。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitCommitDetailVO, GitCommitVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import DiffView from './DiffView.vue'

const props = defineProps<{ sessionId: string }>()
const { t } = useI18n()
const toast = useToastStore()

const commits = ref<GitCommitVO[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    commits.value = await api.git.history(props.sessionId, 100)
  } catch (e) {
    commits.value = []
    error.value = (e as Error)?.message || ''
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fmtWhen(ts: number): string {
  if (!ts) return '—'
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// ---- 单次提交 ----
const open = ref(false)
const detail = ref<GitCommitDetailVO | null>(null)
const detailLoading = ref(false)

async function show(hash: string) {
  open.value = true
  detail.value = null
  detailLoading.value = true
  try {
    detail.value = await api.git.commit(hash, props.sessionId)
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
    open.value = false
  } finally {
    detailLoading.value = false
  }
}

// ---- 文件历史 ----
const fileOpen = ref(false)
const fileCommits = ref<GitCommitVO[]>([])
const fileLoading = ref(false)
const filePath = ref('')

async function showFileHistory(path: string) {
  filePath.value = path
  fileOpen.value = true
  fileCommits.value = []
  fileLoading.value = true
  try {
    fileCommits.value = await api.git.fileHistory(path, props.sessionId, 50)
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
    fileOpen.value = false
  } finally {
    fileLoading.value = false
  }
}

async function openFromFile(hash: string) {
  fileOpen.value = false
  await show(hash)
}

function shortHash(h: string, len = 7) {
  return (h || '').slice(0, len)
}

const detailDiff = computed(() => detail.value?.diff)
</script>

<template>
  <div class="hist">
    <div v-if="loading" class="hist-state"><span class="spin" />{{ t('common.loading') }}</div>
    <div v-else-if="error" class="hist-state">
      <AppIcon name="alert" size="ic-lg" />
      {{ error }}
      <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
    </div>
    <div v-else-if="!commits.length" class="hist-state">
      <AppIcon name="git-branch" size="ic-lg" />
      {{ t('workspace.historyEmpty') }}
    </div>

    <ul v-else class="hist-list">
      <li v-for="c in commits" :key="c.hash">
        <button class="hist-row" type="button" @click="show(c.hash)">
          <span class="hist-dot" />
          <span class="hist-tx">
            <b class="hist-subject">{{ c.subject }}</b>
            <span class="hist-meta">
              <span class="hist-hash">{{ shortHash(c.hash) }}</span>
              <span>{{ c.author }}</span>
              <span>{{ fmtWhen(c.when) }}</span>
              <span>{{ t('workspace.commitCount', { n: c.files }) }}</span>
            </span>
          </span>
          <span class="hist-stat">
            <b class="is-add">+{{ c.additions }}</b>
            <b class="is-del">−{{ c.deletions }}</b>
          </span>
          <AppIcon name="chevron-right" size="ic-xs" class="hist-go" />
        </button>
      </li>
    </ul>

    <el-drawer v-model="open" :title="t('workspace.historyDetail')" size="560px">
      <div v-if="detailLoading" class="hist-state"><span class="spin" />{{ t('common.loading') }}</div>
      <div v-else-if="detail" class="det">
        <header class="det-head">
          <b class="det-subject">{{ detail.subject }}</b>
          <pre v-if="detail.body" class="det-body">{{ detail.body }}</pre>
          <dl class="det-kv">
            <div class="wb-kv"><dt>hash</dt><dd class="wb-mono">{{ detail.hash }}</dd></div>
            <div class="wb-kv"><dt>{{ t('common.name') }}</dt><dd>{{ detail.author }}</dd></div>
            <div class="wb-kv"><dt>{{ t('common.status') }}</dt><dd>{{ fmtWhen(detail.when) }}</dd></div>
          </dl>
        </header>

        <div class="det-files">
          <button
            v-for="f in (detail.files || [])"
            :key="f.path"
            class="det-file"
            type="button"
            @click="showFileHistory(f.path)"
          >
            <AppIcon name="doc" size="ic-xs" />
            <span class="det-path">{{ f.path }}</span>
            <span class="hist-stat">
              <b class="is-add">+{{ f.additions }}</b>
              <b class="is-del">−{{ f.deletions }}</b>
            </span>
          </button>
        </div>

        <DiffView
          v-if="detailDiff"
          :lines="detailDiff.lines || []"
          :path="detailDiff.path || ''"
          :truncated="!!detailDiff.truncated"
          :dropped="detailDiff.dropped || 0"
          :loading="false"
        />
      </div>
    </el-drawer>

    <el-drawer v-model="fileOpen" :title="t('workspace.fileHistory')" size="440px">
      <p class="det-path file-title">{{ filePath }}</p>
      <div v-if="fileLoading" class="hist-state"><span class="spin" />{{ t('common.loading') }}</div>
      <ul v-else class="hist-list">
        <li v-for="c in fileCommits" :key="c.hash">
          <button class="hist-row" type="button" @click="openFromFile(c.hash)">
            <span class="hist-dot" />
            <span class="hist-tx">
              <b class="hist-subject">{{ c.subject }}</b>
              <span class="hist-meta">
                <span class="hist-hash">{{ shortHash(c.hash) }}</span>
                <span>{{ fmtWhen(c.when) }}</span>
              </span>
            </span>
          </button>
        </li>
      </ul>
    </el-drawer>
  </div>
</template>

<style scoped>
.hist {
  min-height: 0;
}
.hist-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-5);
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
}
.spin {
  width: calc(13px * var(--wb-vw-scale));
  height: calc(13px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.8px solid var(--wb-border-strong);
  border-top-color: var(--wb-primary);
  animation: wb-spin 0.8s linear infinite;
}
.hist-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.hist-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.hist-row:hover {
  background: var(--wb-elev-hover);
}
.hist-row:active {
  transform: scale(0.995);
}
.hist-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  flex: none;
}
.hist-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.hist-subject {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hist-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.hist-hash {
  font-family: var(--font-mono);
  color: var(--wb-primary);
}
.hist-stat {
  display: flex;
  gap: var(--wb-sp-2);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.hist-stat .is-add {
  color: var(--wb-success);
}
.hist-stat .is-del {
  color: var(--wb-danger);
}
.hist-go {
  color: var(--wb-muted);
  flex: none;
}
.det {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.det-head {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.det-subject {
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
}
.det-body {
  margin: 0;
  font-family: var(--font-sans);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  line-height: var(--wb-lh-base);
}
.det-kv {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-elev-hover);
}
.det-kv dd {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
}
.det-files {
  display: flex;
  flex-direction: column;
  gap: 1px;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.det-file {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  text-align: left;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.det-file:hover {
  background: var(--wb-elev-hover);
}
.det-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-title {
  margin-bottom: var(--wb-sp-3);
}
</style>
