<script setup lang="ts">
// 工作区页页头：切工作区、仓库态、文件树开关、刷新。
// 抽成组件只为守住单文件行数上限：它读的全是外部传进来的状态，
// 自己不做任何请求，所以搬出去不会产生第二份数据来源。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { GitStatus, WorkspaceRow } from '@/types/api'

const props = defineProps<{
  /** 当前工作区路径 */
  workspace: string
  /** 工作区历史行（带分支与改动数） */
  currentRow: WorkspaceRow | null
  /** git 状态；未加载时为空 */
  status: GitStatus | null
  /** 正在切工作区 */
  switching: boolean
  /** 正在重拉状态 */
  loadingStatus: boolean
  /** 文件树是否展开 */
  filesOpen: boolean
}>()

const emit = defineEmits<{
  pick: []
  toggleFiles: []
  refresh: []
}>()

const { t } = useI18n()

/** 页头显示工作区名而不是整条路径：路径在副标题里已经有了。 */
const workspaceName = computed(() => {
  const path = props.workspace || ''
  if (!path) return t('workspace.pickWorkspace')
  return path.split(/[\\/]/).filter(Boolean).pop() || path
})
</script>

<template>
  <header class="page-head">
    <div>
      <h1 class="page-title">{{ t('workspace.title') }}</h1>
      <p class="page-sub mono selectable">{{ props.workspace }}</p>
    </div>
    <div class="head-actions">
      <!-- 工作区切换：这里是「我在哪个目录」的唯一入口，
           并列显示该目录的仓库态，不必跳到设置页去查。 -->
      <button
        class="wb-btn sm ws-pick"
        :class="{ 'is-loading': props.switching }"
        :title="props.workspace || t('workspace.pickWorkspace')"
        :disabled="props.switching"
        @click="emit('pick')"
      >
        <AppIcon name="folder" :size="13" />
        <span class="mono ws-name">{{ workspaceName }}</span>
      </button>
      <span v-if="props.currentRow?.workspace_is_repo" class="tag" :class="props.currentRow.workspace_dirty ? 'warn' : ''">
        <AppIcon name="branch" :size="11" />
        {{ props.currentRow.workspace_branch || t('workspace.noCommit') }}
        <span v-if="props.currentRow.workspace_dirty" class="dirty">+{{ props.currentRow.workspace_dirty }}</span>
      </span>
      <span v-if="props.status?.is_repo" class="tag" :class="props.status.has_conflict ? 'warn' : ''">
        <AppIcon name="branch" :size="11" />
        {{ props.status.branch_name || t('workspace.noCommit') }}
      </span>
      <span v-if="props.status?.is_repo && props.status.upstream_ref" class="readout">
        ↑{{ props.status.ahead }} ↓{{ props.status.behind }}
      </span>
      <button
        class="wb-btn icon ghost"
        :class="{ on: props.filesOpen }"
        :title="t('workspace.fileTree')"
        @click="emit('toggleFiles')"
      >
        <AppIcon name="folder" :size="15" />
      </button>
      <button
        class="wb-btn icon ghost"
        :class="{ 'is-loading': props.loadingStatus }"
        :title="t('common.refresh')"
        @click="emit('refresh')"
      >
        <AppIcon name="refresh" :size="15" />
      </button>
    </div>
  </header>
</template>

<style scoped>
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
</style>
