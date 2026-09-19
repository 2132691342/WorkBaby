<script setup lang="ts">
/**
 * PinnedPlan：composer 上方的计划胶囊。
 *
 * <p>默认只显示 `3/7` + 迷你进度条，hover / focus 才展开清单——
 * 随时可见「还剩多少」但不喧宾夺主，也不占用消息流。
 * 全部完成后转绿勾；运行中有步骤未完成时进度条带呼吸动画。
 */
import { computed, ref } from 'vue'
import { useChatStore } from '@/stores/chat'
import { t } from '@/i18n'

const chat = useChatStore()
const items = computed(() => chat.todoState?.items ?? [])
const done = computed(() => items.value.filter((x) => x.done).length)
const total = computed(() => items.value.length)
const progress = computed(() => (total.value === 0 ? 0 : Math.round((done.value / total.value) * 100)))
const isDone = computed(() => total.value > 0 && done.value === total.value)
/** 进行中：有计划且未完成（呼吸动画的依据）。 */
const isActive = computed(() => total.value > 0 && !isDone.value)

const open = ref(false)

/** 正在提交勾选的条目 id（防连点；状态以服务端返回为准）。 */
const toggling = ref('')

async function toggle(itemID: string): Promise<void> {
  if (!itemID || toggling.value) return
  toggling.value = itemID
  try {
    await chat.toggleTodo(itemID)
  } finally {
    toggling.value = ''
  }
}
</script>

<template>
  <div
    v-if="total > 0"
    class="pinned-plan"
    @mouseenter="open = true"
    @mouseleave="open = false"
    @focusin="open = true"
    @focusout="open = false"
  >
    <button
      type="button"
      class="pp-capsule"
      :aria-expanded="open"
      :title="t('chat.todo.title')"
      @click="open = !open"
    >
      <svg class="h-3 w-3 shrink-0" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <path d="M3 4.5h10M3 8h10M3 11.5h6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
      </svg>
      <span class="pp-num tabular-nums">{{ done }}/{{ total }}</span>
      <span class="pp-bar">
        <span
          class="pp-fill"
          :class="{ 'pp-fill--active': isActive, 'pp-fill--done': isDone }"
          :style="{ width: progress + '%' }"
        />
      </span>
      <span v-if="isDone" class="pp-check">✓</span>
    </button>

    <Transition
      enter-active-class="transition-all duration-150 ease-out"
      enter-from-class="opacity-0 translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition-all duration-100 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="pp-pop">
        <p class="pp-title">{{ t('chat.todo.title') }} · {{ progress }}%</p>
        <ul class="pp-list">
          <li v-for="item in items" :key="item.id">
            <!-- 可勾选：用户能直接改进度，写回与 todo 工具同一份会话状态，
                 下一轮 system 注入即带上最新进度（模型据此调整下一步） -->
            <button
              type="button"
              class="pp-item"
              :class="item.done ? 'text-wb-muted' : 'text-wb-ink'"
              :disabled="toggling === item.id"
              :title="t('chat.todo.toggleHint')"
              @click="toggle(item.id)"
            >
              <span class="pp-dot" :class="item.done ? 'pp-dot--done' : 'pp-dot--open'" />
              <span :class="item.done ? 'line-through' : ''">{{ item.title }}</span>
            </button>
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.pinned-plan {
  position: relative;
  display: flex;
  justify-content: center;
  margin: 0 auto 4px;
  width: fit-content;
  z-index: 20;
}
.pp-capsule {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 999px;
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-size: 11px;
  font-weight: 500;
  box-shadow: var(--wb-shadow-sm);
  cursor: pointer;
  transition: border-color 0.15s ease;
}
.pp-capsule:hover {
  border-color: color-mix(in srgb, var(--wb-primary) 45%, transparent);
}
.pp-num {
  font-variant-numeric: tabular-nums;
}
.pp-bar {
  display: inline-block;
  width: 44px;
  height: 3px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--wb-surface-hover);
}
.pp-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--wb-primary);
  transition: width 0.3s ease-out;
}
/* 进行中呼吸；完成转绿 */
.pp-fill--active {
  animation: pp-breathe 1.6s ease-in-out infinite;
}
.pp-fill--done {
  background: var(--wb-mint);
  animation: none;
}
.pp-check {
  color: var(--wb-mint);
  font-weight: 700;
}
.pp-pop {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  width: 300px;
  max-width: 80vw;
  border-radius: 14px;
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  padding: 10px 12px;
  box-shadow: var(--wb-shadow-lg);
  z-index: 30;
}
.pp-title {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--wb-muted);
  margin-bottom: 6px;
}
.pp-list {
  max-height: 200px;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.pp-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
  padding: 2px 0;
  font-size: 11px;
  line-height: 1.45;
  text-align: left;
  cursor: pointer;
}
.pp-item:hover {
  color: var(--wb-primary);
}
.pp-item:disabled {
  opacity: 0.6;
  cursor: default;
}
.pp-dot {
  margin-top: 4px;
  display: inline-block;
  height: 6px;
  width: 6px;
  flex-shrink: 0;
  border-radius: 999px;
}
.pp-dot--done {
  background: var(--wb-mint);
}
.pp-dot--open {
  background: var(--wb-primary);
}
@keyframes pp-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}
</style>
