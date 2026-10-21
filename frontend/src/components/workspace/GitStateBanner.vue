<script setup lang="ts">
// 中间状态横幅：合并 / 变基 / 拣选 / 回退没做完时顶在最上面。
// 这时 git 拒绝大多数写入，用户必须知道「继续」还是「中止」，否则页面看起来只是坏了。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ status: GitStatusVO | null; sessionId: string }>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

const { t } = useI18n()
const toast = useToastStore()
const busy = ref('')

// 状态名走 i18n：直接显示 state 会冒出英文 merge / rebase，与界面语言不一致
const stateLabel = computed(() => {
  switch (props.status?.state) {
    case 'merge':
      return t('workspace.stateMerge')
    case 'rebase':
      return t('workspace.stateRebase')
    case 'cherry-pick':
      return t('workspace.stateCherryPick')
    case 'revert':
      return t('workspace.stateRevert')
    default:
      return ''
  }
})

const conflicts = computed(() => (props.status?.files || []).filter((f) => f.conflict))
const blocked = computed(() => conflicts.value.length > 0)

async function act(action: 'continue' | 'abort') {
  if (busy.value) return
  busy.value = action
  try {
    emit('done', await api.git.state(action, props.sessionId))
    toast.ok(action === 'continue' ? t('workspace.stateContinueOk') : t('workspace.stateAbortOk'))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <div v-if="status && status.state && status.state !== 'none'" class="st">
    <span class="st-icon"><AppIcon name="alert" size="ic-sm" /></span>
    <div class="st-text">
      <b>{{ stateLabel }}</b>
      <span v-if="status.state_detail" class="st-detail wb-mono" :title="status.state_detail">
        {{ status.state_detail }}
      </span>
      <span v-if="blocked" class="st-count">
        {{ t('workspace.stateConflictCount', { n: conflicts.length }) }}
      </span>
    </div>
    <span class="sp" />
    <el-button
      size="small"
      :loading="busy === 'continue'"
      :disabled="!!busy || blocked"
      :title="blocked ? t('workspace.conflictResolveHint') : ''"
      @click="act('continue')"
    >
      {{ t('workspace.stateContinue') }}
    </el-button>
    <el-popconfirm
      :title="t('workspace.stateAbortConfirm')"
      :confirm-button-text="t('workspace.stateAbort')"
      :cancel-button-text="t('common.cancel')"
      width="260"
      @confirm="act('abort')"
    >
      <template #reference>
        <el-button size="small" type="danger" plain :loading="busy === 'abort'" :disabled="!!busy">
          {{ t('workspace.stateAbort') }}
        </el-button>
      </template>
    </el-popconfirm>
  </div>
</template>

<style scoped>
/* 现在是「有事情等你决定」，用中性描边 + 警示文字，不铺一层彩色底 */
.st {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-border-strong);
  border-left: 2px solid var(--wb-warning);
  border-radius: var(--wb-radius);
  background: var(--wb-elev-raised);
  animation: wb-sink var(--wb-dur) var(--wb-ease);
}
.st-icon {
  display: grid;
  place-items: center;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-warning-soft);
  color: var(--wb-warning);
  flex: none;
}
.st-text {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  min-width: 0;
  font-size: var(--wb-fs-sm);
}
.st-text b {
  color: var(--wb-ink);
  flex: none;
}
.st-detail {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: calc(300px * var(--wb-vw-scale));
}
.st-count {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  font-variant-numeric: tabular-nums;
  flex: none;
}
.sp {
  flex: 1;
}
@keyframes wb-sink {
  from {
    opacity: 0;
    transform: translateY(calc(-4px * var(--wb-vw-scale)));
  }
  to {
    opacity: 1;
    transform: none;
  }
}
</style>
