<script setup lang="ts">
// 工作区中间那一栏：git 的改动清单 + 提交框 + 历史，或「不是仓库」的理由。
// 拆成组件只为守住单文件行数上限：它只渲染，不发请求——
// 写动作由父组件经 useGitActions 统一处理。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GitChangesPane from '@/components/workspace/GitChangesPane.vue'
import GitCommitPanel from '@/components/workspace/GitCommitPanel.vue'
import HistoryPanel from '@/components/workspace/HistoryPanel.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import type { GitBranch, GitCommit, GitFile, GitStash } from '@/types/api'

const props = defineProps<{
  /** 是不是一个 git 仓库 */
  isRepo: boolean
  /** git 本身在不在：不在时用后端给的原因说明 */
  gitMissing: boolean
  /** 后端给的一句原因（git 不可用时用） */
  unavailableReason: string
  /** 已暂存的改动 */
  staged: GitFile[]
  /** 未暂存的改动 */
  unstaged: GitFile[]
  /** 当前选中的改动文件 */
  selected: string
  /** 分支清单 */
  branches: GitBranch[]
  /** 提交历史 */
  history: GitCommit[]
  /** 暂存架清单 */
  stashes: GitStash[]
  /** 进行中的 git 动作名（空表示空闲） */
  pending: string
  /** 提交进行中 */
  committing: boolean
}>()

const emit = defineEmits<{
  pick: [file: GitFile]
  stage: [file: GitFile]
  unstage: [file: GitFile]
  discard: [file: GitFile]
  resolve: [file: GitFile]
  resolveAs: [file: GitFile, side: 'ours' | 'theirs' | '']
  stageAll: []
  /** 切到历史标签并只看这个文件（父组件据此对齐差异侧） */
  history: [file: GitFile]
  commit: [message: string, isAmend: boolean]
  createBranch: []
  switchBranch: [name: string]
  deleteBranch: [name: string]
  mergeBranch: [name: string]
  stashPush: []
  stashPop: []
  stashDrop: [row: GitStash]
  initRepo: []
}>()

const { t } = useI18n()

const tab = ref<'changes' | 'history'>('changes')
// 文件时间线：选中一个文件就看它的历史，空串表示看整个仓库。
const historyFile = ref('')

/** 提交框自己持有正在写的说明：父组件在提交成功后叫它清空。 */
const commitPanel = ref<{ reset: () => void } | null>(null)

/** 切到历史标签并只看这个文件。 */
function showFileHistory(file: GitFile): void {
  historyFile.value = file.file_path
  tab.value = 'history'
  emit('history', file)
}

defineExpose({ resetCommit: () => commitPanel.value?.reset() })

/** 提交成功的提示由父组件负责（那边有 hash 要报）。这里只暴露清空动作。 */
const stagedCount = computed(() => props.staged.length)
</script>

<template>
  <template v-if="props.isRepo">
    <div class="tabs seg">
      <button class="seg-btn" type="button" :class="{ active: tab === 'changes' }" @click="tab = 'changes'">
        {{ t('workspace.changes') }}
        <span v-if="props.staged.length + props.unstaged.length" class="count">
          {{ props.staged.length + props.unstaged.length }}
        </span>
      </button>
      <button class="seg-btn" type="button" :class="{ active: tab === 'history' }" @click="tab = 'history'">
        {{ t('workspace.history') }}
        <span v-if="props.history.length" class="count">{{ props.history.length }}</span>
      </button>
    </div>

    <div v-if="tab === 'changes'" class="mid-body">
      <GitChangesPane
        :staged="props.staged"
        :unstaged="props.unstaged"
        :selected="props.selected"
        :pending="props.pending"
        :stashes="props.stashes"
        @pick="emit('pick', $event)"
        @stage="emit('stage', $event)"
        @unstage="emit('unstage', $event)"
        @discard="emit('discard', $event)"
        @resolve="emit('resolve', $event)"
        @resolveAs="(file, side) => emit('resolveAs', file, side)"
        @stage-all="emit('stageAll')"
        @history="showFileHistory"
        @stash-push="emit('stashPush')"
        @stash-pop="emit('stashPop')"
        @stash-drop="emit('stashDrop', $event)"
      />

      <GitCommitPanel
        ref="commitPanel"
        :branches="props.branches"
        :staged-count="stagedCount"
        :committing="props.committing"
        :pending="props.pending"
        @commit="(message, isAmend) => emit('commit', message, isAmend)"
        @create-branch="emit('createBranch')"
        @switch-branch="emit('switchBranch', $event)"
        @delete-branch="emit('deleteBranch', $event)"
        @merge-branch="emit('mergeBranch', $event)"
      />
    </div>

    <div v-else class="mid-body history-body">
      <HistoryPanel :commits="props.history" :file="historyFile" @clear-file="historyFile = ''" />
    </div>
  </template>

  <!-- git 不可用 / 不是仓库：说清原因，给唯一相关的那个动作 -->
  <div v-else class="git-gate">
    <WbEmpty
      :icon="props.gitMissing ? 'alert' : 'git'"
      :title="props.gitMissing ? t('workspace.gitUnavailableTitle') : t('workspace.notRepo')"
      :hint="props.gitMissing ? t('workspace.gitUnavailableHint') : t('workspace.notRepoHint')"
    >
      <template #action>
        <button
          v-if="!props.gitMissing"
          class="wb-btn primary"
          type="button"
          :class="{ 'is-loading': props.pending === 'init' }"
          :disabled="Boolean(props.pending)"
          @click="emit('initRepo')"
        >
          {{ t('workspace.init') }}
        </button>
      </template>
    </WbEmpty>
    <!-- git 不在时后端会给一句原因：原话比我们自己编的更准 -->
    <p v-if="props.gitMissing && props.unavailableReason" class="git-reason selectable">
      {{ props.unavailableReason }}
    </p>
  </div>
</template>

<style scoped>
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
/* git 不可用 / 不是仓库：那一栏说清原因，不留空白 */
.git-gate {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-6) var(--wb-sp-4);
  border: 1px dashed var(--wb-line);
  border-radius: var(--wb-r-lg);
}
.git-reason {
  margin: 0;
  max-width: 420px;
  text-align: center;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  overflow-wrap: anywhere;
}
</style>
