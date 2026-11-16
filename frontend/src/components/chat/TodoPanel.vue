<script setup lang="ts">
// 待办面板：由会话里的 todo 工具调用派生，只读展示。
// 面板管不到清单内容——改清单是模型的事，界面别抢；这里只把「做到哪了」摆出来。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon, { type IconName } from '@/components/common/AppIcon.vue'
import type { TodoItem } from '@/types/api'

const props = defineProps<{ todos: TodoItem[] }>()

const { t } = useI18n()

/** 默认展开：有任务在看，用户点标题收起。 */
const open = ref(true)

/** 完成数：cancelled 不存在（后端已滤掉），所以分母就是全部。 */
const done = computed(() => props.todos.filter((row) => row.todo_status === 'completed').length)

const current = computed(() => props.todos.find((row) => row.todo_status === 'in_progress'))

/** 进行中的那行主显 activeForm（现在进行时），没有退回 subject。 */
function subjectOf(row: TodoItem): string {
  if (row.todo_status === 'in_progress' && row.todo_active_form) return row.todo_active_form
  return row.todo_subject
}

function iconOf(row: TodoItem): IconName {
  if (row.todo_status === 'completed') return 'check'
  return 'clock'
}</script>

<template>
  <section v-if="todos.length" class="todo-panel">
    <button
      class="todo-head wb-row-click"
      type="button"
      :aria-expanded="open"
      :aria-label="t('chat.todoTitle')"
      @click="open = !open"
    >
      <AppIcon name="chevron" :size="10" class="todo-ic" :class="{ collapsed: !open }" />
      <span class="todo-title">{{ t('chat.todoTitle') }}</span>
      <span class="todo-progress wb-mono">{{ done }}/{{ todos.length }}</span>
      <span v-if="current" class="todo-current">{{ subjectOf(current) }}</span>
    </button>
    <!-- 展开/收起走高度过渡：一跳一合的面板会让底下的输入区跟着晃 -->
    <Transition name="todo-fold">
      <div v-if="open" class="todo-list">
        <div v-for="row in todos" :key="row.todo_id" class="todo-row" :class="`is-${row.todo_status}`">
          <AppIcon v-if="row.todo_status !== 'pending'" :name="iconOf(row)" :size="12" class="todo-mark" />
          <span v-else class="todo-dot" />
          <span class="todo-subject">{{ subjectOf(row) }}</span>
        </div>
      </div>
    </Transition>
  </section>
</template>

<style scoped>
.todo-panel {
  flex: none;
  margin: 0 var(--wb-sp-5);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-surface);
}

.todo-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-1) var(--wb-sp-3);
  border: 0;
  background: transparent;
  font: inherit;
  font-size: var(--wb-fs-2xs);
  cursor: pointer;
  border-radius: var(--wb-r);
  transition: background-color var(--wb-dur-fast) var(--wb-ease);
}

.todo-head:hover {
  background: var(--wb-hover);
}

.todo-head:active {
  background: var(--wb-selected);
}

.todo-ic {
  flex: none;
  transform: rotate(90deg);
  transition: transform var(--wb-dur-fast) var(--wb-ease);
}

.todo-ic.collapsed {
  transform: rotate(0deg);
}

.todo-title {
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--wb-faint);
  font-size: var(--wb-fs-3xs);
}

.todo-progress {
  color: var(--wb-brand);
  font-size: var(--wb-fs-3xs);
}

.todo-current {
  margin-left: auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink-2);
}

.todo-list {
  display: flex;
  flex-direction: column;
  padding: 0 var(--wb-sp-2) var(--wb-sp-2);
}

.todo-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 2px var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
}

.todo-row.is-completed {
  color: var(--wb-faint);
  text-decoration: line-through;
}

.todo-row.is-in_progress .todo-subject {
  color: var(--wb-brand);
  font-weight: 600;
}

.todo-mark {
  flex: none;
  color: var(--wb-faint);
}

.todo-row.is-in_progress .todo-mark {
  color: var(--wb-brand);
}

.todo-row.is-completed .todo-mark {
  color: var(--wb-ok);
}

.todo-dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin: 2px;
  border-radius: 50%;
  border: 1.5px solid var(--wb-faint);
}

/* 折叠过渡：透明 + 上边距收合，不做逐行动画——行数会变，逐行动画必然跳 */
.todo-fold-enter-active,
.todo-fold-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}

.todo-fold-enter-from,
.todo-fold-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
