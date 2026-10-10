<script setup lang="ts">
// 全局 toast 容器：挂在 App 根部，任何页面的操作反馈都从这里出。
// 样式走 wb-ui.css 的 .toasts/.toast（半成品基础设施，这里补上消费端）。
import { useToastStore } from '../../stores/toast'
import AppIcon from './AppIcon.vue'

const toast = useToastStore()

const ICONS: Record<string, string> = { ok: 'check', bad: 'alert', info: 'info' }
</script>

<template>
  <!-- aria-live：读屏用户不该「看不到提示」；键盘可达则让不需要鼠标的人也能关掉 -->
  <div class="toasts" role="status" aria-live="polite">
    <button
      v-for="t in toast.items"
      :key="t.id"
      type="button"
      class="toast"
      :class="`is-${t.kind}`"
      @click="toast.dismiss(t.id)"
    >
      <AppIcon :name="ICONS[t.kind]" size="ic-sm" />
      <span>{{ t.text }}</span>
    </button>
  </div>
</template>

<style scoped>
/* 可点关闭的 toast 同样要有点击反馈：悬停提亮、按下回收，
   否则用户不知道它能点，只能等它自己消失 */
.toast {
  pointer-events: auto;
  cursor: pointer;
  /* 换 button 之后要按回 toast 的版式：button 自带居中与衬线默认，会歪 */
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  text-align: left;
  font: inherit;
  color: inherit;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.toast:hover {
  background: var(--wb-surface-hover);
}
.toast:active {
  transform: scale(0.98);
}
.toast.is-ok .ic {
  color: var(--wb-success);
}
.toast.is-bad {
  border-color: var(--wb-danger);
}
.toast.is-bad .ic {
  color: var(--wb-danger);
}
.toast.is-info .ic {
  color: var(--wb-primary);
}
</style>
