<script setup lang="ts">
// 对话内提示条：排队、已切换、压缩中这类「刚刚发生了什么」。
// 不复用 toast：那是全局动作的结果，这里是这一轮的过程，要贴着消息流。
// 每条 4 秒自动收、悬停暂停——用户可能正要复制里面的路径。
import { onBeforeUnmount, ref, watch } from 'vue'
import { useChatStore } from '@/stores/chat'

const chat = useChatStore()

// 每条提示各自的倒计时：一条共享定时器会让「后到的提示」把前面的寿命重置，
// 表现为提示偶尔赖着不走。hover 暂停也按条算。
const hovered = ref(0)
const timers = new Map<number, number>()

function armTimer(id: number, ms: number): void {
  clearTimer(id)
  timers.set(
    id,
    window.setTimeout(() => {
      timers.delete(id)
      chat.dismissNotice(id)
    }, ms),
  )
}

function clearTimer(id: number): void {
  const t = timers.get(id)
  if (t !== undefined) {
    window.clearTimeout(t)
    timers.delete(id)
  }
}

function hold(id: number): void {
  hovered.value = id
  clearTimer(id)
}

function resume(id: number): void {
  hovered.value = 0
  armTimer(id, 2000)
}

// 只为新增的提示起表；已有的不重置——「刚刚发生了什么」越新越要消失得准。
for (const n of chat.notices) armTimer(n.id, 4200)

watch(
  () => chat.notices.map((n) => n.id),
  (ids, prev) => {
    const known = new Set(prev ?? [])
    for (const id of ids) if (!known.has(id)) armTimer(id, 4200)
    if (prev) for (const id of prev) if (!ids.includes(id)) clearTimer(id)
  },
)

onBeforeUnmount(() => {
  for (const t of timers.values()) window.clearTimeout(t)
  timers.clear()
})
</script>

<template>
  <transition-group name="notice" tag="div" class="notice-stack">
    <button type="button"
      v-for="notice in chat.notices"
      :key="notice.id"
      class="notice-chip wb-pressable"
      :class="notice.level"
      @mouseenter="hold(notice.id)"
      @mouseleave="resume(notice.id)"
      @click="chat.dismissNotice(notice.id)"
    >
      <span class="dot" :class="notice.level === 'error' ? 'warn' : notice.level === 'warn' ? 'warn' : 'ok'" />
      <span class="notice-text selectable">{{ notice.message }}</span>
    </button>
  </transition-group>
</template>

<style scoped>
.notice-stack {
  position: absolute;
  left: 50%;
  /* 底部栈高度由 ChatView 实测写入 --wb-bottom-h：提示条贴着输入区上方走 */
  bottom: calc(var(--wb-bottom-h, 150px) + var(--wb-sp-2));
  transform: translateX(-50%);
  z-index: var(--wb-z-pop);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  align-items: center;
  pointer-events: none;
}

.notice-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  max-width: 520px;
  padding: var(--wb-sp-1) var(--wb-sp-3);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-full);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-5);
  pointer-events: auto;
}

/* 缩放交给 .wb-pressable；这里只管换色 */
.notice-chip:hover {
  border-color: var(--wb-brand);
}

.notice-chip.error {
  border-color: var(--wb-bad);
  color: var(--wb-bad);
}

.notice-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notice-enter-active,
.notice-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}

.notice-enter-from,
.notice-leave-to {
  opacity: 0;
  transform: translateY(6px) scale(0.96);
}
</style>
