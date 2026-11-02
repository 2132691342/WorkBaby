<script setup lang="ts">
// 待办面板：模型维护的清单，用户也能勾选 / 新增 / 删除。
// 数据来自 GET /todo（结构 VO），不解析 widget 文本——依赖图是结构不是字符串。
// 写入走 POST /todo/mutate，与模型用的 todo 工具同一套动作与校验规则。
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { TodoItemVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import AppIcon from '../common/AppIcon.vue'
import EmptyState from '../common/EmptyState.vue'

const { t } = useI18n()
const toast = useToastStore()

const items = ref<TodoItemVO[]>([])
const doneCount = ref(0)
const loading = ref(true)
const failed = ref('')
/** 正在写的条目 id（空串 = 没在写）；按钮据此进加载态 */
const busy = ref('')
const adding = ref(false)
const draft = ref('')
const draftInput = ref<{ focus: () => void } | null>(null)

const total = computed(() => items.value.length)

/** 被依赖项里还有没完成的 → 这条现在不能开始 */
function isBlocked(item: TodoItemVO): boolean {
  if (!item.todo_blocked_by.length) return false
  const done = new Set(
    items.value.filter((i) => i.todo_status === 'completed').map((i) => i.todo_id),
  )
  return item.todo_blocked_by.some((id) => !done.has(id))
}

function adopt(resp: { resp_items: TodoItemVO[]; resp_done_count: number }) {
  items.value = resp.resp_items ?? []
  doneCount.value = resp.resp_done_count ?? 0
}

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    adopt(await api.todo.list())
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

/** 勾选 / 取消勾选：completed ↔ pending 一个动作切完 */
async function toggle(item: TodoItemVO) {
  if (busy.value) return
  busy.value = item.todo_id
  try {
    adopt(
      await api.todo.mutate({
        op: 'status',
        id: item.todo_id,
        status: item.todo_status === 'completed' ? 'pending' : 'completed',
      }),
    )
  } catch (e) {
    toast.bad(errorText(e))
  } finally {
    busy.value = ''
  }
}

async function remove(item: TodoItemVO) {
  if (busy.value) return
  busy.value = item.todo_id
  try {
    adopt(await api.todo.mutate({ op: 'remove', id: item.todo_id }))
  } catch (e) {
    toast.bad(errorText(e))
  } finally {
    busy.value = ''
  }
}

async function submitDraft() {
  const text = draft.value.trim()
  if (!text || busy.value === '__add') return
  busy.value = '__add'
  try {
    adopt(await api.todo.mutate({ op: 'add', text }))
    draft.value = ''
    adding.value = false
  } catch (e) {
    toast.bad(errorText(e))
  } finally {
    busy.value = ''
  }
}

async function startAdding() {
  adding.value = true
  await nextTick()
  draftInput.value?.focus()
}

onMounted(() => {
  void reload()
})

defineExpose({ reload })
</script>

<template>
  <div class="todo-wrap">
    <div v-if="loading && !items.length" class="wb-card pad">
      <el-skeleton animated :rows="3" />
    </div>

    <div v-else-if="failed" class="wb-card pad err">
      <span class="err-tx">{{ t('common.loadFailed', { msg: failed }) }}</span>
      <el-button size="small" @click="reload">{{ t('common.retry') }}</el-button>
    </div>

    <template v-else>
      <div v-if="total" class="cap">
        <span class="wb-label">{{ t('featurePanel.todoProgress', { done: doneCount, total }) }}</span>
        <el-progress
          :percentage="total ? Math.round((doneCount / total) * 100) : 0"
          :stroke-width="6"
          :show-text="false"
          class="bar"
        />
      </div>

      <ul v-if="total" class="list">
        <li
          v-for="item in items"
          :key="item.todo_id"
          class="row"
          :class="{ 'is-done': item.todo_status === 'completed', 'is-blocked': isBlocked(item) }"
        >
          <button
            class="mark"
            :class="[`is-${item.todo_status}`, { 'is-busy': busy === item.todo_id }]"
            type="button"
            :title="t('featurePanel.todoToggle')"
            :disabled="!!busy"
            @click="toggle(item)"
          >
            <AppIcon v-if="item.todo_status === 'completed'" name="check" size="ic-xs" />
            <span v-else-if="item.todo_status === 'in_progress'" class="dot" />
          </button>
          <span class="seq">{{ item.todo_seq }}</span>
          <span class="tx">{{ item.todo_text }}</span>
          <span v-if="isBlocked(item)" class="blocked" :title="t('featurePanel.todoBlockedHint')">
            <AppIcon name="lock" size="ic-xs" />
            {{ t('featurePanel.todoBlocked') }}
          </span>
          <button
            class="del"
            type="button"
            :title="t('featurePanel.todoRemove')"
            :disabled="!!busy"
            @click="remove(item)"
          >
            <AppIcon name="close" size="ic-xs" />
          </button>
        </li>
      </ul>

      <EmptyState
        v-else
        icon="list"
        :title="t('featurePanel.todoEmpty')"
        :desc="t('featurePanel.todoEmptyHint')"
      />

      <div v-if="adding" class="add-row">
        <el-input
          ref="draftInput"
          v-model="draft"
          size="small"
          :placeholder="t('featurePanel.todoAddPlaceholder')"
          @keydown.enter="submitDraft"
          @keydown.esc="adding = false"
        />
        <el-button
          size="small"
          type="primary"
          :loading="busy === '__add'"
          :disabled="!draft.trim()"
          @click="submitDraft"
        >
          {{ t('common.confirm') }}
        </el-button>
        <el-button size="small" :disabled="busy === '__add'" @click="adding = false">
          {{ t('common.cancel') }}
        </el-button>
      </div>
      <el-button v-else size="small" class="add-btn" :disabled="!!busy" @click="startAdding">
        <AppIcon name="plus" size="ic-2xs" />
        <span class="ml-1">{{ t('featurePanel.todoAdd') }}</span>
      </el-button>
    </template>
  </div>
</template>

<style scoped>
.todo-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-height: 0;
}
.cap {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.bar {
  flex: 1;
  min-width: 0;
}
.list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  margin: 0;
  padding: 0;
  list-style: none;
}
.row {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-1);
  border-radius: var(--wb-radius-sm);
  border: 1px solid transparent;
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease);
}
.row:hover {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border);
}
/* 勾选盒就是按钮：勾选是这一屏最常做的动作，不该藏在菜单里 */
.mark {
  flex: none;
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  border: 1.5px solid var(--wb-border-strong);
  display: grid;
  place-items: center;
  margin-top: 1px;
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.mark:hover:not(:disabled) {
  border-color: var(--wb-primary);
  transform: scale(1.15);
}
.mark:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.mark:disabled {
  cursor: progress;
  opacity: 0.6;
}
.mark.is-completed {
  background: var(--wb-success);
  border-color: var(--wb-success);
  color: #fff;
}
.mark.is-in_progress {
  border-color: var(--wb-primary);
}
.dot {
  width: 5px;
  height: 5px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  animation: todo-breathe 1.5s var(--wb-ease) infinite;
}
@keyframes todo-breathe {
  0%,
  100% {
    opacity: 0.35;
  }
  50% {
    opacity: 1;
  }
}
.seq {
  flex: none;
  width: calc(16px * var(--wb-vw-scale));
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  padding-top: 1px;
}
.tx {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow-wrap: anywhere;
}
.row.is-done .tx {
  color: var(--wb-muted);
  text-decoration: line-through;
  text-decoration-color: var(--wb-border-strong);
}
.row.is-blocked .tx {
  color: var(--wb-muted);
}
.blocked {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-warning);
}
/* 删除只在行悬停时露出来：常驻的叉会在清单上制造一片噪音 */
.del {
  flex: none;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  display: grid;
  place-items: center;
  color: var(--wb-muted);
  opacity: 0;
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    background-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.row:hover .del,
.del:focus-visible {
  opacity: 1;
}
.del:hover:not(:disabled) {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  transform: scale(1.1);
}
.del:active:not(:disabled) {
  transform: scale(var(--wb-btn-scale-press));
}
.add-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.add-btn {
  align-self: flex-start;
}
.pad {
  padding: var(--wb-sp-4);
}
.err {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
}
.err-tx {
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
</style>
