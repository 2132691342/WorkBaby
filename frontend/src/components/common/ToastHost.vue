<script setup lang="ts">
// 全局轻提示：挂在 App 根部，任何页面的操作反馈都从这里出。
// 正文必须可选可复制——报错原文与工具日志要靠它贴出去找人，读一半消失 + 选不中等于没有提示。
import { useI18n } from 'vue-i18n'
import { useToastStore } from '../../stores/toast'
import AppIcon from './AppIcon.vue'

const { t } = useI18n()
const toast = useToastStore()

const ICONS: Record<string, string> = { ok: 'check', bad: 'alert', info: 'info' }
</script>

<template>
  <!-- aria-live：读屏用户不会「看不到提示」；关闭键给不想等它自己消失的人 -->
  <TransitionGroup name="toast" tag="div" class="toasts" role="status" aria-live="polite">
    <div
      v-for="it in toast.items"
      :key="it.id"
      class="toast"
      :class="`is-${it.kind}`"
      @pointerenter="toast.hold(it.id, true)"
      @pointerleave="toast.hold(it.id, false)"
    >
      <span class="toast-ic"><AppIcon :name="ICONS[it.kind]" size="ic-sm" /></span>
      <p class="toast-text">{{ it.text }}</p>
      <button class="toast-close" type="button" :aria-label="t('common.close')" @click="toast.dismiss(it.id)">
        <AppIcon name="close" size="ic-xs" />
      </button>
    </div>
  </TransitionGroup>
</template>

<style scoped>
.toasts {
  position: fixed;
  right: var(--wb-sp-4);
  bottom: var(--wb-sp-4);
  z-index: var(--wb-z-toast);
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: var(--wb-sp-2);
  pointer-events: none;
}
.toast {
  pointer-events: auto;
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  max-width: calc(420px * var(--wb-vw-scale));
  padding: var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  box-shadow: var(--wb-shadow-3);
}
.toast-ic {
  display: grid;
  place-items: center;
  width: calc(20px * var(--wb-vw-scale));
  height: calc(20px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  flex: none;
  margin-top: 1px;
}
.toast.is-ok .toast-ic {
  background: var(--wb-success-soft);
  color: var(--wb-success);
}
.toast.is-bad .toast-ic {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
}
.toast.is-info .toast-ic {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.toast-text {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  line-height: var(--wb-lh-base);
  color: var(--wb-ink);
  overflow-wrap: anywhere;
  user-select: text;
  cursor: text;
}
.toast-close {
  flex: none;
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.toast-close:hover {
  background: var(--wb-surface-2);
  color: var(--wb-ink);
  transform: scale(1.1);
}
.toast-close:active {
  transform: scale(0.9);
}
.toast-enter-active,
.toast-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
.toast-leave-to {
  opacity: 0;
  transform: scale(0.96);
}
</style>
