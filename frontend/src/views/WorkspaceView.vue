<script setup lang="ts">
// 工作区页：文件树 + 文件预览（一直可用）+ git 的改动 / 提交 / 差异（仅仓库）。
// 回答「助手到底改了什么，能不能收成一次提交」：读全开放，写只做白名单动作；
// 文件浏览与预览不依赖 git——不是仓库的目录照样能看文件；
// 没有 git 或不是仓库时，那一栏说清原因并给唯一相关的动作，不留空白页。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DiffView from '@/components/workspace/DiffView.vue'
import FilePanel from '@/components/workspace/FilePanel.vue'
import FileViewPane from '@/components/workspace/FileViewPane.vue'
import GitBar from '@/components/workspace/GitBar.vue'
import GitMiddlePane from '@/components/workspace/GitMiddlePane.vue'
import WorkspaceHead from '@/components/workspace/WorkspaceHead.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
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
// 差异侧选中的文件（git 改动清单里点的那一个）。
const selected = ref('')
// 文件树里选中的文件（右侧「文件」标签显示的就是它）。与 selected 分开：
// 一个是 git 的改动，一个是磁盘上的文件——普通目录里没有 git 改动也有文件要看。
const previewPath = ref('')
// 右侧那栏看什么：git 差异还是文件内容。
const rightTab = ref<'diff' | 'file'>('diff')
const committing = ref(false)
// 竞态守卫：事件 bump 的自动重拉、切工作区与手动刷新并发时，晚到的旧状态直接丢弃。
let statusSeq = 0
let diffSeq = 0
// 提交框自己持有正在写的说明，这里只需要在提交成功后叫它清空。
const gitPane = ref<{ resetCommit: () => void } | null>(null)
// 文件树那一份抽屉：编辑抽屉只在文件树里有一份，右侧「编辑」按钮叫它打开。
const filePanel = ref<{ openFile: (path: string) => Promise<void> } | null>(null)
// 文件树默认展开：它是"我要看哪个文件"最快的入口。但窗口窄时三栏挤不下，
// 容器查询会把它整个收起来，需要时再用页头的按钮叫回来。
const filesOpen = ref(true)

const unstaged = computed(() => (status.value?.changed_files ?? []).filter((f) => !f.is_staged))
const staged = computed(() => (status.value?.changed_files ?? []).filter((f) => f.is_staged))
const conflicts = computed(() => (status.value?.changed_files ?? []).filter((f) => f.has_conflict))

/** 是不是一个 git 仓库：决定改动 / 提交 / 历史那一栏在不在。 */
const isRepo = computed(() => Boolean(status.value?.is_repo))
/** git 本身在不在：不在时要说后端给的原因，不能自己编一句。 */
const gitMissing = computed(() => Boolean(status.value && !status.value.git_available))
/** 没有工作目录时这一页无事可做：先让人选目录。 */
const noWorkspace = computed(() => !app.workspace)

// 工作区清单：每条带仓库分支、未提交改动与会话数。
// 取同名字段拼成一行，而不是再发一次 git 请求：切换工作区时这些数字
// 会跟着变，多一次往返就多一个"看到的是上一个库"的窗口。
const rows = ref<WorkspaceRow[]>([])
const switching = ref(false)
const currentRow = computed(() => rows.value.find((r) => r.workspace_path === app.workspace) ?? null)

async function loadRows() {
  try {
    rows.value = (await api.workspaces()).workspace_rows ?? []
  } catch {
    // 清单是辅助信息：拉不到就不显示店铺，不影响主区逐项操作。
    rows.value = []
  }
}

async function loadStatus() {
  const mine = ++statusSeq
  loadingStatus.value = true
  try {
    const next = await api.gitStatus()
    if (mine !== statusSeq) return
    status.value = next
  } catch (err) {
    if (mine !== statusSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === statusSeq) loadingStatus.value = false
  }
}

async function loadDiff(path = selected.value) {
  if (!status.value?.is_repo) return
  const mine = ++diffSeq
  loadingDiff.value = true
  try {
    const next = await api.gitDiff(path)
    if (mine !== diffSeq) return
    diff.value = next
  } catch (err) {
    if (mine !== diffSeq) return
    diff.value = null
    // 差异拉不到要说出来：右栏空着却什么都不说，用户以为「没有改动」。
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === diffSeq) loadingDiff.value = false
  }
}

async function loadExtras() {
  if (!status.value?.is_repo) {
    branches.value = []
    history.value = []
    stashes.value = []
    return
  }
  // 分支 / 历史 / 暂存架三样各自报一次失败：静默清空会让「提交历史为空」
  // 看起来像真话，而它其实只是这一次没拉到。
  try {
    branches.value = await api.gitBranches()
  } catch (err) {
    branches.value = []
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
  try {
    history.value = (await (await api.gitHistory(40)).commits)
  } catch (err) {
    history.value = []
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
  try {
    stashes.value = (await api.gitStashList()).stashes
  } catch (err) {
    stashes.value = []
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function refresh() {
  await loadStatus()
  await Promise.all([loadDiff(), loadExtras(), loadRows()])
}

/** git 改动清单里点了一个文件：右侧切到差异，看这一处的改了什么。 */
function pick(file: GitFile) {
  selected.value = file.file_path
  rightTab.value = 'diff'
  void loadDiff(file.file_path)
}

/** 文件树里点了一个文件：右侧切到文件内容。普通目录里没有 git 改动，这是唯一出路。 */
function pickFile(path: string) {
  previewPath.value = path
  rightTab.value = 'file'
}

/** 文件被保存：右侧那一栏重读一次，正文才是最新的。 */
const fileReloadKey = ref(0)
function onFileSaved(): void {
  fileReloadKey.value += 1
}

/** 右侧那一栏点「编辑」：叫文件树那份抽屉打开——抽屉只有一份，不另造。 */
function openEditor(path: string): void {
  void filePanel.value?.openFile(path)
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
    gitPane.value?.resetCommit()
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
    previewPath.value = ''
    diff.value = null
    try {
      await refresh()
    } finally {
      switching.value = false
    }
  },
)

onMounted(refresh)

/** 直接叫起「选目录」：dialog 与随后的引擎重建都在这一段里，按钮挂 loading，失败出声。 */
const choosingWorkspace = ref(false)
async function pickWorkspace(): Promise<void> {
  if (choosingWorkspace.value) return
  choosingWorkspace.value = true
  try {
    const prev = app.workspace
    await app.chooseWorkspace()
    if (app.workspace !== prev) toasts.push('ok', t('workspace.switched', { path: app.workspace }))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    choosingWorkspace.value = false
  }
}
</script>

<template>
  <div class="page">
    <!-- 页头由 WorkspaceHead 自己渲染：它已经带 .page-head 那一层，
         这里再套一层只会多一次重排。 -->
    <WorkspaceHead
      :workspace="app.workspace"
      :current-row="currentRow"
      :status="status"
      :switching="switching || choosingWorkspace"
      :loading-status="loadingStatus"
      :files-open="filesOpen"
      @pick="pickWorkspace()"
      @toggle-files="filesOpen = !filesOpen"
      @refresh="refresh"
    />

    <!-- 没选目录：这一页无事可做，先给一个出口 -->
    <WbEmpty
      v-if="noWorkspace"
      class="ws-empty"
      icon="folder"
      :title="t('workspace.noWorkspaceTitle')"
      :hint="t('workspace.noWorkspaceHint')"
    >
      <template #action>
        <button class="wb-btn primary" type="button" :class="{ 'is-loading': choosingWorkspace }" :disabled="choosingWorkspace" @click="pickWorkspace()">
          {{ t('workspace.pickWorkspace') }}
        </button>
      </template>
    </WbEmpty>

    <!-- 引擎状态还没到货：骨架块留出与内容一致的呼吸空间 -->
    <WbSkeleton v-else-if="!status" block class="ws-boot" />

    <template v-else>
      <!-- 中间态横幅：冲突时的处置入口。
           repo_state 的正常值是 "normal"（后端契约），中间态是
           merging / rebasing / cherry-picking——拿 "none" 去比会永远成立，
           于是在干净的仓库上也挂出一条「未完成的合并 / 变基」。 -->
      <div v-if="isRepo && (status.repo_state !== 'normal' || status.has_conflict)" class="notice warn">
        <span class="dot warn" />
        <span>{{ status.repo_state_detail || t('workspace.inProgress') }}</span>
        <button type="button" class="wb-btn sm" :class="{ 'is-loading': git.pending.value === 'resolve' }" :disabled="!conflicts.length || Boolean(git.pending.value)" @click="resolveConflicts">
          {{ t('workspace.markResolved') }}
        </button>
        <button type="button" class="wb-btn sm" :class="{ 'is-loading': git.pending.value === 'state:continue' }" :disabled="Boolean(git.pending.value)" @click="git.stateAction('continue')">
          {{ t('workspace.continue') }}
        </button>
        <button type="button" class="wb-btn sm danger" :class="{ 'is-loading': git.pending.value === 'state:abort' }" :disabled="Boolean(git.pending.value)" @click="git.stateAction('abort')">
          {{ t('workspace.abort') }}
        </button>
      </div>

      <!-- 版本栏：分支 / 上游 / 领先落后 / 同步动作。
           它放在三栏之上，因为「在哪个分支」决定了下面每一栏的语境。
           不是仓库时没有版本栏——那一栏会自己说清原因。 -->
      <div v-if="isRepo" class="bar-wrap">
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
        <!-- 左：文件树。浏览文件与预览不依赖 git，普通目录里照样能用。 -->
        <FilePanel v-if="filesOpen" ref="filePanel" class="pane-files" :selected="previewPath" @pick="pickFile" @saved="onFileSaved" />

        <!-- 中：改动 + 提交（仅仓库）。不是仓库时这一栏说清原因与出路。 -->
        <div class="pane-mid">
          <GitMiddlePane
            ref="gitPane"
            :is-repo="isRepo"
            :git-missing="gitMissing"
            :unavailable-reason="status.unavailable_reason"
            :staged="staged"
            :unstaged="unstaged"
            :selected="selected"
            :branches="branches"
            :history="history"
            :stashes="stashes"
            :pending="git.pending.value"
            :committing="committing"
            @pick="pick"
            @stage="stage"
            @unstage="unstage"
            @discard="discard"
            @resolve="resolveOne"
            @resolveAs="resolveAs"
            @stage-all="stageAll"
            @history="(file) => { selected = file.file_path }"
            @commit="commit"
            @create-branch="git.createBranch"
            @switch-branch="git.switchBranch"
            @delete-branch="git.deleteBranch"
            @merge-branch="git.mergeBranch"
            @stash-push="git.stashPush"
            @stash-pop="git.stashPop"
            @stash-drop="git.stashDrop"
            @init-repo="git.initRepo"
          />
        </div>

        <!-- 右：差异（仓库）或文件内容（一直可用） -->
        <div class="pane-right">
          <div class="tabs seg right-tabs">
            <button
              v-if="isRepo"
              class="seg-btn"
              type="button"
              :class="{ active: rightTab === 'diff' }"
              @click="rightTab = 'diff'"
            >
              {{ t('workspace.diffTab') }}
            </button>
            <button class="seg-btn" type="button" :class="{ active: rightTab === 'file' }" @click="rightTab = 'file'">
              {{ t('workspace.fileTab') }}
            </button>
          </div>
          <DiffView
            v-if="isRepo && rightTab === 'diff'"
            class="pane-diff"
            :diff="diff"
            :path="selected"
            :loading="loadingDiff"
          />
          <FileViewPane v-else class="pane-diff" :path="previewPath" :reload-key="fileReloadKey" @edit="openEditor" />
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
/* 页头（.page-head / .page-title / .page-sub / .head-actions / .ws-name / .dirty）
   全部在 WorkspaceHead 里：那一层自己渲染页头，样式也跟它住在一起。 */

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
.pane-right {
  display: flex;
  flex-direction: column;
  min-height: 0;
  gap: var(--wb-sp-2);
}
/* 右栏的标签只占一行：它决定这一栏看差异还是看文件内容 */
.right-tabs {
  flex: none;
}
/* 改动清单与提交 / 分支各自的样式都搬进 GitChangesPane 与 GitCommitPanel 了：
   这一页只管三栏的排布与刷新。 */
/* 引擎状态还没到货：骨架块留出与内容一致的呼吸空间 */
.ws-boot {
  flex: 1;
  padding: var(--wb-sp-5) var(--wb-sp-6);
}
/* 没选工作目录：整页给一句说明与一个出口，不再是半空的一页 */
.ws-empty {
  flex: 1;
  justify-content: center;
}
</style>
