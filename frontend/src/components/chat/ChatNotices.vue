<script setup lang="ts">
// 对话内提示条：排队、已切换、压缩中这类「刚刚发生了什么」。
// 不复用 toast：那是全局动作的结果，这里是这一轮的过程，要贴着消息流。
// 4 秒自动收、悬停暂停——用户可能正要复制里面的路径。
import { onBeforeUnmount, watch } from 'vue'
import { useChatStore } from '@/stores/chat'

const chat = useChatStore()

// 一条提示 4 秒后自己消失。用 setTimeout 而不是 CSS 动画，
// 因为「移除」要真的从数组里删掉，否则下一条会被它顶慢。
let timer: number | undefined
watch(
  () => chat.notices.length,
  (n) => {
    if (!n) return
    window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      const first = chat.notices[0]
      if (first) chat.dismissNotice(first.id)
    }, 4200)
  },
)

onBeforeUnmount(() => window.clearTimeout(timer))
</script>

<template>
  <transition-group name="notice" tag="div" class="notice-stack">
    <button
      v-for="notice in chat.notices"
      :key="notice.id"
      class="notice-chip wb-pressable"
      :class="notice.level"
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
