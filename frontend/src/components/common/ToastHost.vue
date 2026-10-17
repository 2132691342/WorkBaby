<script setup lang="ts">
// 全局 toast 容器：挂在 App 根部，任何页面的操作反馈都从这里出；样式在 wb-ui.css 的 .toasts/.toast。
// 正文必须可选可复制：报错原文与工具日志要靠它贴出去找人，读一半消失 + 选不中等于没有提示。
import { useToastStore } from '../../stores/toast'
import AppIcon from './AppIcon.vue'

const toast = useToastStore()

const ICONS: Record<string, string> = { ok: 'check', bad: 'alert', info: 'info' }
</script>

<template>
  <!-- aria-live：读屏用户不该「看不到提示」；关闭钮给不想等它自己消失的人 -->
  <TransitionGroup name="toast" tag="div" class="toasts" role="status" aria-live="polite">
    <div
      v-for="t in toast.items"
      :key="t.id"
      class="toast"
      :class="`is-${t.kind}`"
      @pointerenter="toast.hold(t.id, true)"
      @pointerleave="toast.hold(t.id, false)"
    >
      <AppIcon :name="ICONS[t.kind]" size="ic-sm" />
      <p class="toast-text">{{ t.text }}</p>
      <button
        class="toast-close"
        type="button"
        aria-label="关闭提示"
        @click="toast.dismiss(t.id)"
      >
        <AppIcon name="close" size="ic-xs" />
      </button>
    </div>
  </TransitionGroup>
</template>

<style scoped>
/* 关闭钮五态：默认不抢戏、悬停放大显形、按下回收、聚焦有环（键盘可达的等价物）。
   toast 本身不再整块可点——那会让「想选中文字」变成「不小心关掉」。 */
.toast-close {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin-left: auto;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--wb-radius-full);
  background: transparent;
  color: var(--wb-muted);
  cursor: pointer;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.toast-close:hover {
  background: var(--wb-tint);
  border-color: var(--wb-line-2);
  color: var(--wb-ink);
  transform: scale(1.12);
}
.toast-close:active {
  background: var(--wb-tint-lg);
  transform: scale(0.92);
}
.toast-close:focus-visible {
  outline: 2px solid var(--wb-primary);
  outline-offset: 1px;
}
</style>
