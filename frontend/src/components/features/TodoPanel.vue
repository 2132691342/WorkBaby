<script setup lang="ts">
// 待办清单面板：模型维护的实时清单，带依赖关系与被依赖阻塞标记。
// 数据来自 GET /todo（结构 VO），不解析 widget 文本——依赖图是结构不是字符串。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { TodoItemVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import AppIcon from '../common/AppIcon.vue'
import EmptyState from '../common/EmptyState.vue'

const { t } = useI18n()

const items = ref<TodoItemVO[]>([])
const doneCount = ref(0)
const loading = ref(true)
const failed = ref('')

const total = computed(() => items.value.length)

/** 被依赖项里还有没完成的 → 这条现在不能开始 */
function isBlocked(item: TodoItemVO): boolean {
  if (!item.todo_blocked_by.length) return false
  const done = new Set(
    items.value.filter((i) => i.todo_status === 'completed').map((i) => i.todo_id),
  )
  return item.todo_blocked_by.some((id) => !done.has(id))
}

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    const resp = await api.todo.list()
    items.value = resp?.resp_items ?? []
    doneCount.value = resp?.resp_done_count ?? 0
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
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

    <EmptyState
      v-else-if="!total"
      icon="list"
      :title="t('featurePanel.todoEmpty')"
      :desc="t('featurePanel.todoEmptyHint')"
    />

    <template v-else>
      <div class="cap">
        <span class="wb-label">{{ t('featurePanel.todoProgress', { done: doneCount, total }) }}</span>
        <el-progress
          :percentage="total ? Math.round((doneCount / total) * 100) : 0"
          :stroke-width="6"
          :show-text="false"
          class="bar"
        />
      </div>

      <ul class="list">
        <li
          v-for="item in items"
          :key="item.todo_id"
          class="row"
          :class="{ 'is-done': item.todo_status === 'completed', 'is-blocked': isBlocked(item) }"
        >
          <span class="seq">{{ item.todo_seq }}</span>
          <span class="mark" :class="`is-${item.todo_status}`" aria-hidden="true">
            <AppIcon
              v-if="item.todo_status === 'completed'"
              name="check"
              size="ic-xs"
            />
            <span v-else-if="item.todo_status === 'in_progress'" class="dot" />
          </span>
          <span class="tx">{{ item.todo_text }}</span>
          <span v-if="isBlocked(item)" class="blocked" :title="t('featurePanel.todoBlockedHint')">
            <AppIcon name="lock" size="ic-xs" />
            {{ t('featurePanel.todoBlocked') }}
          </span>
        </li>
      </ul>
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
  padding: var(--wb-sp-2) var(--wb-sp-3);
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
.seq {
  flex: none;
  width: calc(18px * var(--wb-vw-scale));
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
  padding-top: 1px;
}
.mark {
  flex: none;
  width: calc(15px * var(--wb-vw-scale));
  height: calc(15px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  border: 1.5px solid var(--wb-border-strong);
  display: grid;
  place-items: center;
  margin-top: 1px;
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease);
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
