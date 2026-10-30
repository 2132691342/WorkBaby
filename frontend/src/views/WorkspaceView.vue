<script setup lang="ts">
// 工作区视图：助手改了哪些文件、改了什么、这个仓库发生过什么，以及把改动收成一次提交。
// 读全开放（status / diff / 历史），写受控六动作（docs/WORKSPACE.md）——提交这一下始终由用户点下去。
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
import GitStateBanner from '../components/workspace/GitStateBanner.vue'

const { t } = useI18n()
const toast = useToastStore()
const session = useSessionStore()

// 看的是当前会话的工作目录：会话自己选过目录时，全局目录不是改动发生的地方
const sessionId = computed(() => session.current?.session_id || '')

const status = ref<GitStatusVO | null>(null)
const diff = ref<GitDiffVO | null>(null)
const loading = ref(true)
const error = ref('')
const selected = ref('')
const loadingDiff = ref(false)

const files = computed<GitFileVO[]>(() => status.value?.git_files || [])
const readable = computed(() => !!status.value?.git_available && !!status.value?.git_is_repo)

const TAGS: Record<string, { text: string; tone: string }> = {
  M: { text: 'M', tone: 'is-mod' },
  A: { text: 'A', tone: 'is-add' },
  D: { text: 'D', tone: 'is-del' },
  R: { text: 'R', tone: 'is-mod' },
  C: { text: 'C', tone: 'is-mod' },
  T: { text: 'T', tone: 'is-mod' },
  U: { text: '!', tone: 'is-conflict' },
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

// 不是仓库时给一条可执行的下一步：界面不能只告诉用户「这里用不了」
const initName = ref('main')
const initing = ref(false)
async function initRepo() {
  initing.value = true
  try {
    status.value = await api.git.init(initName.value.trim() || 'main', sessionId.value)
    toast.ok(t('workspace.initOk'))
    await load()
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    initing.value = false
  }
}

// 冲突文件逐个标记已解决：git 只在标记之后才接受提交
const resolving = ref('')
async function resolveOne(path: string) {
  resolving.value = path
  try {
    status.value = await api.git.resolve([path], sessionId.value)
    toast.ok(t('workspace.conflictResolveOk', { n: 1 }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    resolving.value = ''
  }
}

// 远端地址：没配远端时推送根本无处可去，这一步不能只留在命令行
const remoteOpen = ref(false)
const remoteName = ref('origin')
const remoteUrl = ref('')
const remoteBusy = ref(false)
async function addRemote() {
  remoteBusy.value = true
  try {
    status.value = await api.git.addRemote(
      remoteName.value.trim(),
      remoteUrl.value.trim(),
      sessionId.value,
    )
    toast.ok(t('workspace.remoteAddOk', { name: remoteName.value.trim() }))
    remoteOpen.value = false
    remoteUrl.value = ''
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    remoteBusy.value = false
  }
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
        <b>{{ status?.git_reason || t('workspace.notRepo') }}</b>
        <p>{{ t('workspace.initRepoHint') }}</p>
        <p class="wb-mono">{{ status?.git_workspace }}</p>
        <div v-if="status?.git_available" class="init-box">
          <el-input v-model="initName" size="small" class="init-name" :placeholder="t('workspace.initBranch')" />
          <el-button size="small" type="primary" :loading="initing" @click="initRepo">
            {{ t('workspace.initRepo') }}
          </el-button>
        </div>
      </div>

      <template v-else>
        <GitStateBanner :status="status" :session-id="sessionId" @done="afterWrite" />

        <div class="git-bar">
          <span class="gb-branch">
            <AppIcon name="git-branch" size="ic-xs" />
            {{ status?.git_branch || '—' }}
          </span>
          <span v-if="status?.git_upstream" class="gb-up wb-mono" :title="status.git_upstream">
            {{ status.git_upstream }}
          </span>
          <el-tag v-if="status?.git_ahead" size="small" type="primary">
            {{ t('workspace.ahead', { n: status.git_ahead }) }}
          </el-tag>
          <el-tag v-if="status?.git_behind" size="small" type="warning">
            {{ t('workspace.behind', { n: status.git_behind }) }}
          </el-tag>
          <span class="sp" />
          <el-button size="small" text @click="initName = ''; remoteOpen = !remoteOpen">
            {{ t('workspace.remoteAdd') }}
          </el-button>
          <span class="gb-path wb-mono" :title="status?.git_workspace || ''">{{ status?.git_workspace }}</span>
        </div>

        <div v-if="remoteOpen" class="wb-card pad remote-form">
          <span class="wb-label">{{ t('workspace.remoteAdd') }}</span>
          <el-input v-model="remoteName" size="small" class="rf-name" :placeholder="t('workspace.remoteName')" />
          <el-input v-model="remoteUrl" size="small" class="wb-mono" :placeholder="t('workspace.remoteUrl')" />
          <el-button size="small" type="primary" :loading="remoteBusy" @click="addRemote">
            {{ t('common.create') }}
          </el-button>
          <el-button size="small" @click="remoteOpen = false">{{ t('common.cancel') }}</el-button>
          <p class="rf-hint">{{ t('workspace.remoteUrlHint') }}</p>
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
          <GitBranchPanel :session-id="sessionId" :status="status" @done="afterWrite" />
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
              <li v-for="f in files" :key="f.git_path">
                <button
                  class="fc-row"
                  type="button"
                  :class="{ 'is-on': selected === f.git_path }"
                  :title="f.git_path"
                  @click="openDiff(f.git_path)"
                >
                  <span class="fc-tag" :class="TAGS[f.git_status]?.tone || 'is-mod'">
                    {{ TAGS[f.git_status]?.text || f.git_status }}
                  </span>
                  <span class="fc-path" :title="f.git_path">{{ shortPath(f.git_path) }}</span>
                  <el-button
                    v-if="f.git_conflict"
                    size="small"
                    text
                    class="fc-resolve"
                    :loading="resolving === f.git_path"
                    :title="t('workspace.conflictResolveHint')"
                    @click.stop="resolveOne(f.git_path)"
                  >
                    {{ t('workspace.conflictResolve') }}
                  </el-button>
                  <span v-else class="fc-stat">
                    <b class="is-add">+{{ f.git_additions }}</b>
                    <b class="is-del">−{{ f.git_deletions }}</b>
                  </span>
                </button>
              </li>
            </ul>
          </section>

          <DiffView
            :lines="diff?.git_lines || []"
            :path="diff?.git_diff_path || ''"
            :truncated="!!diff?.git_truncated"
            :dropped="diff?.git_dropped || 0"
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
  background: var(--wb-elev-raised);
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
.gb-up {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  max-width: calc(160px * var(--wb-vw-scale));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gb-path {
  max-width: 46%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.init-box {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-2);
  animation: wb-sink var(--wb-dur) var(--wb-ease);
}
.init-name {
  width: calc(150px * var(--wb-vw-scale));
}
.remote-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-2);
  animation: wb-sink var(--wb-dur) var(--wb-ease);
}
.rf-name {
  width: calc(120px * var(--wb-vw-scale));
}
.rf-hint {
  width: 100%;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.fc-resolve {
  flex: none;
}
.fc-tag.is-conflict {
  background: var(--wb-warning-soft);
  color: var(--wb-warning);
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
  background: var(--wb-elev-hover);
}
.fc-row.is-on {
  background: var(--wb-accent-tint);
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
  background: var(--wb-accent-tint);
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
