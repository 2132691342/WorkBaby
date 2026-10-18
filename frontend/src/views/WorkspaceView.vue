<script setup lang="ts">
// 工作区视图：助手改了哪些文件、改了什么、这个仓库发生过什么，以及把改动收成一次提交。
// 读全开放（status / diff / 历史），写受控六动作（specs/08）——提交这一下始终由用户点下去。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import { useSessionStore } from '../stores/session'
import { useToastStore } from '../stores/toast'
import type { GitDiffVO, GitFileVO, GitStatusVO } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'
import DiffView from '../components/workspace/DiffView.vue'
import GitBranchPanel from '../components/workspace/GitBranchPanel.vue'
import GitCommitPanel from '../components/workspace/GitCommitPanel.vue'
import GitHistoryPanel from '../components/workspace/GitHistoryPanel.vue'

const { t } = useI18n()
const toast = useToastStore()
const session = useSessionStore()

// 看的是当前会话的工作目录：会话自己选过目录时，全局目录不是改动发生的地方
const sessionId = computed(() => session.current?.id || '')

const status = ref<GitStatusVO | null>(null)
const diff = ref<GitDiffVO | null>(null)
const loading = ref(true)
const error = ref('')
const selected = ref('')
const loadingDiff = ref(false)

const files = computed<GitFileVO[]>(() => status.value?.files || [])
const readable = computed(() => !!status.value?.available && !!status.value?.is_repo)

const TAGS: Record<string, { text: string; tone: string }> = {
  M: { text: 'M', tone: 'is-mod' },
  A: { text: 'A', tone: 'is-add' },
  D: { text: 'D', tone: 'is-del' },
  R: { text: 'R', tone: 'is-mod' },
  '?': { text: 'U', tone: 'is-new' },
}

function shortPath(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    status.value = await api.git.status(sessionId.value)
    await openDiff('')
  } catch (e) {
    status.value = null
    error.value = (e as Error)?.message || ''
  } finally {
    loading.value = false
  }
}

async function openDiff(path: string) {
  selected.value = path
  loadingDiff.value = true
  try {
    diff.value = await api.git.diff(path, sessionId.value)
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
    diff.value = null
  } finally {
    loadingDiff.value = false
  }
}

onMounted(() => {
  if (!session.current) void session.loadList()
  void load()
})
watch(sessionId, load)

const TABS = [
  { key: 'changes', labelKey: 'workspace.changes' },
  { key: 'commit', labelKey: 'workspace.commit' },
  { key: 'history', labelKey: 'workspace.history' },
  { key: 'branch', labelKey: 'workspace.branches' },
]
const tab = ref('changes')

// 写操作后由子面板回传重读的状态：整页用同一份数据刷新
async function afterWrite(st: GitStatusVO) {
  status.value = st
  if (tab.value === 'changes') await openDiff('')
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('workspace.title')" :subtitle="t('workspace.subtitle')">
      <template #actions>
        <el-button size="small" :loading="loading" @click="load">
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('workspace.reload') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <div v-if="loading && !status" class="wb-card pad">
        <el-skeleton animated :rows="6" />
      </div>

      <div v-else-if="error" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="alert" size="ic-xl" /></span>
        <b>{{ t('errors.generic') }}</b>
        <p>{{ error }}</p>
        <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
      </div>

      <div v-else-if="!readable" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="git-branch" size="ic-xl" /></span>
        <b>{{ status?.reason || t('workspace.notRepo') }}</b>
        <p>{{ t('workspace.notRepoHint') }}</p>
        <p class="wb-mono">{{ status?.workspace }}</p>
      </div>

      <template v-else>
        <div class="git-bar">
          <span class="gb-branch">
            <AppIcon name="git-branch" size="ic-xs" />
            {{ status?.branch || '—' }}
          </span>
          <el-tag v-if="status?.ahead" size="small" type="primary">
            {{ t('workspace.ahead', { n: status.ahead }) }}
          </el-tag>
          <el-tag v-if="status?.behind" size="small" type="warning">
            {{ t('workspace.behind', { n: status.behind }) }}
          </el-tag>
          <span class="sp" />
          <span class="gb-path wb-mono" :title="status?.workspace || ''">{{ status?.workspace }}</span>
        </div>

        <el-tabs v-model="tab" class="git-tabs">
          <el-tab-pane v-for="tt in TABS" :key="tt.key" :label="t(tt.labelKey)" :name="tt.key" />
        </el-tabs>

        <div v-if="tab === 'commit'" class="wb-card pad">
          <GitCommitPanel :status="status" :session-id="sessionId" @done="afterWrite" />
        </div>

        <div v-else-if="tab === 'history'" class="wb-card pad">
          <GitHistoryPanel :session-id="sessionId" />
        </div>

        <div v-else-if="tab === 'branch'" class="wb-card pad">
          <GitBranchPanel :session-id="sessionId" @done="afterWrite" />
        </div>

        <div v-else class="changes">
          <section class="wb-card file-card">
            <header class="fc-head">
              <span class="wb-label">{{ t('workspace.files') }}</span>
              <span class="fc-count">{{ files.length }}</span>
            </header>
            <div v-if="!files.length" class="fc-empty">
              <AppIcon name="check" size="ic-lg" />
              <b>{{ t('workspace.noChanges') }}</b>
              <p>{{ t('workspace.noChangesHint') }}</p>
            </div>
            <ul v-else class="fc-list">
              <li v-for="f in files" :key="f.path">
                <button
                  class="fc-row"
                  type="button"
                  :class="{ 'is-on': selected === f.path }"
                  :title="f.path"
                  @click="openDiff(f.path)"
                >
                  <span class="fc-tag" :class="TAGS[f.status]?.tone || 'is-mod'">
                    {{ TAGS[f.status]?.text || f.status }}
                  </span>
                  <span class="fc-path">{{ shortPath(f.path) }}</span>
                  <span class="fc-stat">
                    <b class="is-add">+{{ f.additions }}</b>
                    <b class="is-del">−{{ f.deletions }}</b>
                  </span>
                </button>
              </li>
            </ul>
          </section>

          <DiffView
            :lines="diff?.lines || []"
            :path="diff?.path || ''"
            :truncated="!!diff?.truncated"
            :dropped="diff?.dropped || 0"
            :loading="loadingDiff"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.pad {
  padding: var(--wb-sp-4);
}
.git-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  font-size: var(--wb-fs-xs);
}
.gb-branch {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-family: var(--font-mono);
  font-weight: 700;
  color: var(--wb-primary);
}
.gb-path {
  max-width: 46%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.git-tabs {
  margin-bottom: calc(var(--wb-sp-2) * -1);
}
.git-tabs :deep(.el-tabs__header) {
  margin-bottom: var(--wb-sp-2);
}
.changes {
  display: grid;
  grid-template-columns: minmax(0, 340px) minmax(0, 1fr);
  gap: var(--wb-sp-4);
  align-items: start;
  min-height: 0;
}
.file-card {
  overflow: hidden;
}
.fc-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-3) var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-border);
}
.fc-count {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.fc-list {
  max-height: calc(520px * var(--wb-vw-scale));
  overflow-y: auto;
  padding: var(--wb-sp-1);
}
.fc-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    box-shadow var(--wb-dur-fast) var(--wb-ease);
}
.fc-row:hover {
  background: var(--wb-surface-2);
}
.fc-row.is-on {
  background: var(--wb-primary-soft);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.fc-tag {
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  display: grid;
  place-items: center;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.fc-tag.is-add {
  background: var(--wb-success-soft);
  color: var(--wb-success);
}
.fc-tag.is-del {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
}
.fc-tag.is-mod {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.fc-tag.is-new {
  background: var(--wb-live-soft);
  color: var(--wb-live);
}
.fc-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fc-stat {
  display: flex;
  gap: var(--wb-sp-1);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  flex: none;
}
.fc-stat .is-add {
  color: var(--wb-success);
}
.fc-stat .is-del {
  color: var(--wb-danger);
}
.fc-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
  color: var(--wb-muted);
}
.fc-empty b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.fc-empty p {
  font-size: var(--wb-fs-2xs);
  line-height: var(--wb-lh-base);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(440px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
.sp {
  flex: 1;
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
@media (max-width: 1000px) {
  .changes {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
