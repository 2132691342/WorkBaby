<script setup lang="ts">
// 工作区页：文件树 + 改动清单 + 差异 + 提交/分支。
// 回答「助手到底改了什么，能不能收成一次提交」：读全开放，写只做白名单动作；
// 没有 git 或不是仓库时显示原因与「初始化」，不显示空表格。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import DiffView from '@/components/workspace/DiffView.vue'
import FilePanel from '@/components/workspace/FilePanel.vue'
import GitBar from '@/components/workspace/GitBar.vue'
import GitChangesPane from '@/components/workspace/GitChangesPane.vue'
import GitCommitPanel from '@/components/workspace/GitCommitPanel.vue'
import HistoryPanel from '@/components/workspace/HistoryPanel.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useRefreshStore } from '@/stores/refresh'
import { useToasts } from '@/composables/useToasts'
import { useGitActions } from '@/composables/useGitActions'
import type { GitBranch, GitCommit, GitDiff, GitFile, GitStatus, GitStash, WorkspaceRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const status = ref<GitStatus | null>(null)
const diff = ref<GitDiff | null>(null)
const branches = ref<GitBranch[]>([])
const history = ref<GitCommit[]>([])
const stashes = ref<GitStash[]>([])

const loadingStatus = ref(false)
const loadingDiff = ref(false)
const selected = ref('')
const committing = ref(false)
// 提交框自己持有正在写的说明，这里只需要在提交成功后叫它清空。
const commitPanel = ref<{ reset: () => void } | null>(null)
const tab = ref<'changes' | 'history'>('changes')
// 文件时间线：选中一个文件就看它的历史，空串表示看整个仓库。
// 它与「差异选中」是同一份 selected——用户点的就是一个文件，不该分两处。
const historyFile = ref('')
// 文件树默认展开：它是“我要看哪个文件”最快的入口。但窗口窄时三栏挤不下，
// 容器查询会把它整个收起来，需要时再用页头的按钮叫回来。
const filesOpen = ref(true)

const unstaged = computed(() => (status.value?.changed_files ?? []).filter((f) => !f.is_staged))
const staged = computed(() => (status.value?.changed_files ?? []).filter((f) => f.is_staged))
const conflicts = computed(() => (status.value?.changed_files ?? []).filter((f) => f.has_conflict))

// 工作区清单：每条带仓库分支、未提交改动与会话数。
// 取同名字段拼成一行，而不是再发一次 git 请求：切换工作区时这些数字
// 会跟着变，多一次往返就多一个“看到的是上一个库”的窗口。
const rows = ref<WorkspaceRow[]>([])
const switching = ref(false)
const currentRow = computed(() => rows.value.find((r) => r.workspace_path === app.workspace) ?? null)
const workspaceName = computed(() => {
  const path = app.workspace || ''
  if (!path) return t('workspace.pickWorkspace')
  return path.split(/[\\/]/).filter(Boolean).pop() || path
})

async function loadRows() {
  try {
    rows.value = (await api.workspaces()).workspace_rows ?? []
  } catch {
    // 清单是辅助信息：拉不到就不显示店铺，不影响主区逐项操作。
    rows.value = []
  }
}

async function loadStatus() {
  loadingStatus.value = true
  try {
    status.value = await api.gitStatus()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loadingStatus.value = false
  }
}

async function loadDiff(path = selected.value) {
  if (!status.value?.is_repo) return
  loadingDiff.value = true
  try {
    diff.value = await api.gitDiff(path)
  } catch {
    diff.value = null
  } finally {
    loadingDiff.value = false
  }
}

async function loadExtras() {
  if (!status.value?.is_repo) {
    branches.value = []
    history.value = []
    stashes.value = []
    return
  }
  try {
    branches.value = await api.gitBranches()
  } catch {
    branches.value = []
  }
  try {
    history.value = await (await api.gitHistory(40)).commits
  } catch {
    history.value = []
  }
  try {
    stashes.value = (await api.gitStashList()).stashes
  } catch {
    stashes.value = []
  }
}

async function refresh() {
  await loadStatus()
  await Promise.all([loadDiff(), loadExtras(), loadRows()])
}

function pick(file: GitFile) {
  selected.value = file.file_path
  void loadDiff(file.file_path)
}

/** 切到历史标签并只看这个文件。 */
function showFileHistory(filePath: string) {
  selected.value = filePath
  historyFile.value = filePath
  tab.value = 'history'
}

// 写动作全部收口在 useGitActions 里：它们共享同一套后果处理
// （挂 loading → 重拉 → 失败给原文），散着写迟早有哪个忘了重拉。
const git = useGitActions(() => status.value, refresh)

async function stageAll() {
  await git.stageAll(unstaged.value)
}

async function stage(file: GitFile) {
  await git.stage(file)
}

async function unstage(file: GitFile) {
  await git.unstage(file)
}

async function discard(file: GitFile) {
  await git.discard(file)
}

async function resolveConflicts() {
  await git.resolveConflicts(conflicts.value)
}

async function resolveOne(file: GitFile) {
  await git.resolve(file)
}

/** 按口径解决冲突：保留我方 / 保留对方 / 我改完了。 */
async function resolveAs(file: GitFile, side: 'ours' | 'theirs' | '') {
  await git.resolveAs(file, side)
}

async function commit(text: string, isAmend = false) {
  if (!text.trim() || (!isAmend && !staged.value.length) || committing.value) return
  committing.value = true
  try {
    const result = await api.gitCommit(text, isAmend)
    status.value = result.status
    commitPanel.value?.reset()
    toasts.push('ok', t('workspace.committed', { hash: result.commit_hash.slice(0, 7) }))
    await Promise.all([loadDiff(), loadExtras()])
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    committing.value = false
  }
}

// 引擎跑完一轮多半动过文件：事件路由 bump 一下，这里防抖重拉，
// 让「助手改了什么」不用手点刷新就能看到。
const refreshStore = useRefreshStore()
let bumpTimer = 0
watch(
  () => refreshStore.workspace,
  () => {
    window.clearTimeout(bumpTimer)
    bumpTimer = window.setTimeout(() => void refresh(), 600)
  },
)

// 切完工作区引擎会重建，因此整套状态都得重新拉一遍；
// 不能只等 workspace 事件后防抖，那个延迟里的旧数据会让人以为切失败了。
watch(
  () => app.workspace,
  async (next, prev) => {
    if (!prev || next === prev) return
    switching.value = true
    selected.value = ''
    diff.value = null
    try {
      await refresh()
    } finally {
      switching.value = false
    }
  },
)

onMounted(refresh)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('workspace.title') }}</h1>
        <p class="page-sub mono selectable">{{ app.workspace }}</p>
      </div>
      <div class="head-actions">
        <!-- 工作区切换：这里是「我在哪个目录」的唯一入口，
             并列显示该目录的仓库态与存储状况，不必跳到设置页去查。 -->
        <button
          class="wb-btn sm ws-pick"
          :class="{ 'is-loading': switching }"
          :title="app.workspace || t('workspace.pickWorkspace')"
          :disabled="switching"
          @click="app.chooseWorkspace()"
        >
          <AppIcon name="folder" :size="13" />
          <span class="mono ws-name">{{ workspaceName }}</span>
        </button>
        <span v-if="currentRow?.workspace_is_repo" class="tag" :class="currentRow.workspace_dirty ? 'warn' : ''">
          <AppIcon name="branch" :size="11" />
          {{ currentRow.workspace_branch || t('workspace.noCommit') }}
          <span v-if="currentRow.workspace_dirty" class="dirty">+{{ currentRow.workspace_dirty }}</span>
        </span>
        <span v-if="status?.is_repo" class="tag" :class="status.has_conflict ? 'warn' : ''">
          <AppIcon name="branch" :size="11" />
          {{ status.branch_name || t('workspace.noCommit') }}
        </span>
        <span v-if="status?.is_repo && status.upstream_ref" class="readout">
          ↑{{ status.ahead }} ↓{{ status.behind }}
        </span>
        <button
          class="wb-btn icon ghost"
          :class="{ on: filesOpen }"
          :title="t('workspace.fileTree')"
          @click="filesOpen = !filesOpen"
        >
          <AppIcon name="folder" :size="15" />
        </button>
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loadingStatus }" :title="t('common.refresh')" @click="refresh">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <!-- 不可用：说清原因与下一步 -->
    <div v-if="status && !status.git_available" class="notice">
      <span class="dot warn" />
      <span>{{ status.unavailable_reason }}</span>
    </div>

    <div v-else-if="status && !status.is_repo" class="notice">
      <span class="dot warn" />
      <span>{{ t('workspace.notRepo') }}</span>
      <button class="wb-btn sm" :class="{ 'is-loading': git.pending.value === 'init' }" :disabled="Boolean(git.pending.value)" @click="git.initRepo">
        {{ t('workspace.init') }}
      </button>
    </div>

    <template v-else-if="status">
      <!-- 中间态横幅：冲突时的处置入口 -->
      <div v-if="status.repo_state !== 'none' || status.has_conflict" class="notice warn">
        <span class="dot warn" />
        <span>{{ status.repo_state_detail || t('workspace.inProgress') }}</span>
        <button class="wb-btn sm" :class="{ 'is-loading': git.pending.value === 'resolve' }" :disabled="!conflicts.length || Boolean(git.pending.value)" @click="resolveConflicts">
          {{ t('workspace.markResolved') }}
        </button>
        <button class="wb-btn sm" :class="{ 'is-loading': git.pending.value === 'state:continue' }" :disabled="Boolean(git.pending.value)" @click="git.stateAction('continue')">
          {{ t('workspace.continue') }}
        </button>
        <button class="wb-btn sm danger" :class="{ 'is-loading': git.pending.value === 'state:abort' }" :disabled="Boolean(git.pending.value)" @click="git.stateAction('abort')">
          {{ t('workspace.abort') }}
        </button>
      </div>

      <!-- 版本栏：分支 / 上游 / 领先落后 / 同步动作。
           它放在三栏之上，因为「在哪个分支」决定了下面每一栏的语境。 -->
      <div class="bar-wrap">
        <GitBar
          :status="status"
          :pending="git.pending.value"
          @fetch="git.fetchRemote"
          @pull="git.pull"
          @push="git.push"
          @add-remote="git.addRemote"
        />
      </div>

      <div class="split" :class="{ 'no-files': !filesOpen }">
        <!-- 左：文件树。默认展开，但只是一个“找文件”的入口：
             真正看改动的地方是右边两栏。 -->
        <FilePanel v-if="filesOpen" class="pane-files" />

        <!-- 中：改动 + 提交 -->
        <div class="pane-mid">
          <div class="tabs seg">
            <button class="seg-btn" :class="{ active: tab === 'changes' }" @click="tab = 'changes'">
              {{ t('workspace.changes') }}
              <span v-if="status.changed_files.length" class="count">{{ status.changed_files.length }}</span>
            </button>
            <button class="seg-btn" :class="{ active: tab === 'history' }" @click="tab = 'history'">
              {{ t('workspace.history') }}
              <span v-if="history.length" class="count">{{ history.length }}</span>
            </button>          </div>

          <div v-if="tab === 'changes'" class="mid-body">
            <GitChangesPane
              :staged="staged"
              :unstaged="unstaged"
              :selected="selected"
              :pending="git.pending.value"
              :stashes="stashes"
              @pick="pick"
              @stage="stage"
              @unstage="unstage"
              @discard="discard"
              @resolve="resolveOne"
              @resolveAs="resolveAs"
              @stage-all="stageAll"
              @history="(file) => showFileHistory(file.file_path)"
              @stash-push="git.stashPush"
              @stash-pop="git.stashPop"
              @stash-drop="git.stashDrop"
            />

            <GitCommitPanel
              ref="commitPanel"
              :branches="branches"
              :staged-count="staged.length"
              :committing="committing"
              :pending="git.pending.value"
              @commit="commit"
              @create-branch="git.createBranch"
              @switch-branch="git.switchBranch"
              @delete-branch="git.deleteBranch"
              @merge-branch="git.mergeBranch"
            />
          </div>

          <div v-else class="mid-body history-body">
            <HistoryPanel :commits="history" :file="historyFile" @clear-file="historyFile = ''" />
          </div>
        </div>

        <!-- 右：差异 -->
        <DiffView class="pane-diff" :diff="diff" :path="selected" :loading="loadingDiff" />
      </div>
    </template>

    <div v-else class="empty">
      <span class="dot busy" />
      {{ t('common.loading') }}
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}
.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}
.page-sub {
  margin: 4px 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}
.head-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

/* 工作区名是一段路径：限宽 + 省略号，否则长路径会把右边的动作全挤出头部 */
.ws-name {
  display: inline-block;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

/* 未提交改动数：数字比字色更靠前，一眼扫得到 */
.dirty {
  margin-left: var(--wb-sp-1);
  font-variant-numeric: tabular-nums;
  opacity: 0.85;
}
.notice {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin: var(--wb-sp-4) var(--wb-sp-6) 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-surface);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
  flex-wrap: wrap;
  flex: none;
}
.notice.warn {
  border-color: var(--wb-warn);
  background: var(--wb-warn-soft);
}
.split {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 210px minmax(280px, 1fr) minmax(320px, 1.2fr);
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4) var(--wb-sp-6) var(--wb-sp-5);
  overflow: hidden;
}

/* 文件树收起来时把那一列让给改动与差异 */
.split.no-files {
  grid-template-columns: minmax(280px, 1fr) minmax(320px, 1.3fr);
}

/* 版本栏的位置：贴着页头，横跨整个内容区 */
.bar-wrap {
  padding: var(--wb-sp-4) var(--wb-sp-6) 0;
  flex: none;
}

/* 窄窗口下三栏放不下：文件树让出宽度，diff 保住最小可用宽度。
   低于 980px 时左栏只剩 176px（只够看目录名），再窄就完全交给用户自己收。 */
@container app (max-width: 980px) {
  .split {
    grid-template-columns: 168px minmax(200px, 1fr) minmax(240px, 1fr);
    padding: var(--wb-sp-3) var(--wb-sp-4);
  }
  .split.no-files {
    grid-template-columns: minmax(200px, 1fr) minmax(240px, 1fr);
  }
}
.pane-files,
.pane-diff {
  min-height: 0;
}
.pane-mid {
  display: flex;
  flex-direction: column;
  min-height: 0;
  gap: var(--wb-sp-2);
}
.mid-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
/* 历史面板自己管滚动：提交详情展开后高度会变，外层再套一个滚动条会双重跳动 */
.mid-body.history-body {
  overflow: hidden;
}
/* 改动清单与提交 / 分支各自的样式都搬进 GitChangesPane 与 GitCommitPanel 了：
   这一页只管三栏的排布与刷新。 */
.empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  color: var(--wb-faint);
}
</style>
