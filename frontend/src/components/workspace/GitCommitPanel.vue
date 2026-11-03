<script setup lang="ts">
// 提交框与分支列表：同一块位置（改动清单下方）、都只在「改动」标签下出现。
// 拆出来让 WorkspaceView 只管三栏排布；自己的状态只有正在写的提交说明。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { GitBranch } from '@/types/api'

const props = defineProps<{
  branches: GitBranch[]
  /** 暂存区里的文件数（提交按钮上显示） */
  stagedCount: number
  committing: boolean
  /** 进行中的动作名（空表示空闲） */
  pending: string
}>()

const emit = defineEmits<{
  commit: [message: string]
  createBranch: []
  switchBranch: [name: string]
  deleteBranch: [name: string]
  mergeBranch: [name: string]
}>()

const { t } = useI18n()

const message = ref('')

const canCommit = computed(
  () => Boolean(message.value.trim()) && props.stagedCount > 0 && !props.committing,
)

async function submit(): Promise<void> {
  if (!canCommit.value) return
  emit('commit', message.value.trim())
  message.value = ''
}

/** 父组件提交成功后调用：清空输入框。 */
function reset(): void {
  message.value = ''
}

defineExpose({ reset })
</script>

<template>
  <div class="commit-box">
    <textarea
      v-model="message"
      class="wb-input"
      rows="3"
      :placeholder="t('workspace.commitPlaceholder')"
    />
    <div class="commit-actions">
      <button
        class="wb-btn primary"
        :class="{ 'is-loading': props.committing }"
        :disabled="!canCommit"
        @click="submit"
      >
        {{ t('workspace.commit', { n: props.stagedCount }) }}
      </button>
    </div>
  </div>

  <div class="branch-row">
    <span class="plate">{{ t('workspace.branches') }}</span>
    <button class="wb-btn text sm" :disabled="Boolean(props.pending)" @click="emit('createBranch')">
      {{ t('workspace.newBranch') }}
    </button>
  </div>
  <div class="branch-list">
    <button
      v-for="branch in props.branches"
      :key="`${branch.remote_name}:${branch.branch_name}`"
      class="branch-line wb-row-click"
      :class="{ active: branch.current }"
      :disabled="branch.current || Boolean(props.pending)"
      :title="branch.current ? t('workspace.currentBranch') : t('workspace.switchHere')"
      @click="emit('switchBranch', branch.branch_name)"
    >
      <AppIcon name="branch" :size="12" />
      <span class="branch-name">{{ branch.branch_name }}</span>
      <span v-if="branch.remote_name" class="tag">{{ branch.remote_name }}</span>
      <span v-if="branch.ahead || branch.behind" class="readout">
        ↑{{ branch.ahead }} ↓{{ branch.behind }}
      </span>
      <!-- 行内动作：悬停才出现。一列分支每行都挂两个按钮就是一片按钮墙。 -->
      <span v-if="!branch.current" class="branch-acts">
        <button class="act" :disabled="Boolean(props.pending)" @click.stop="emit('mergeBranch', branch.branch_name)">
          {{ t('workspace.merge') }}
        </button>
        <button class="act danger" :disabled="Boolean(props.pending)" @click.stop="emit('deleteBranch', branch.branch_name)">
          {{ t('common.delete') }}
        </button>
      </span>
    </button>
  </div>
</template>

<style scoped>
.commit-box {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  flex: none;
}

.commit-actions {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.branch-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}

.plate {
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: var(--wb-ls-plate);
  text-transform: uppercase;
  color: var(--wb-faint);
}

.branch-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  flex: none;
}

.branch-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 3px var(--wb-sp-2);
  border: none;
  background: transparent;
  color: var(--wb-mute);
  font-size: var(--wb-fs-xs);
  font-family: var(--wb-font-mono);
  text-align: left;
  border-radius: var(--wb-r-sm);
  cursor: pointer;
}

.branch-line span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.branch-name {
  flex: 1;
  min-width: 0;
}

/* 行内动作：默认不占视觉重量，悬停才出现 */
.branch-acts {
  display: flex;
  gap: 2px;
  flex: none;
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}

.branch-line:hover .branch-acts,
.branch-line:focus-within .branch-acts {
  opacity: 1;
}

.act {
  padding: 0 5px;
  height: calc(18px * var(--wb-vw-scale));
  border: none;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-mute);
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  white-space: nowrap;
  transition: color var(--wb-dur-fast) var(--wb-ease);
}

.act:hover:not(:disabled) {
  color: var(--wb-brand);
}

.act.danger:hover:not(:disabled) {
  color: var(--wb-bad);
}

.act:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.branch-line.active {
  color: var(--wb-brand);
  font-weight: 600;
}

.branch-line:disabled {
  cursor: default;
}

.readout {
  margin-left: auto;
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
</style>
