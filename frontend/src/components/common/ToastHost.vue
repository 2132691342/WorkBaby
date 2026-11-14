<script setup lang="ts">
// 轻提示：右下角堆叠，可点选复制，悬停暂停计时；可带一个动作（如撤销）。
// 有原因才显示原因，没原因不硬编一句「操作成功」。
import { useI18n } from 'vue-i18n'
import { useToasts } from '@/composables/useToasts'

const { t } = useI18n()
const { items, dismiss, hold, runAction } = useToasts()
</script>

<template>
  <div class="toast-host">
    <transition-group name="wb-list" tag="div" class="toast-stack">
      <div
        v-for="item in items"
        :key="item.id"
        class="toast selectable"
        :class="item.level"
        @mouseenter="hold(item.id, true)"
        @mouseleave="hold(item.id, false)"
      >
        <span class="toast-dot" />
        <div class="toast-text">
          <p class="toast-msg">{{ item.message }}</p>
          <p v-if="item.detail" class="toast-detail">{{ item.detail }}</p>
        </div>
        <button v-if="item.action" class="toast-action" @click="runAction(item)">
          {{ item.action.label }}
        </button>
        <button class="toast-close" :title="t('common.close')" @click="dismiss(item.id)">
          <svg viewBox="0 0 12 12" width="10" height="10" stroke="currentColor" stroke-width="1.4"><path d="M3 3l6 6M9 3 3 9" /></svg>
        </button>
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.toast-host {
  position: fixed;
  right: var(--wb-sp-4);
  bottom: var(--wb-sp-4);
  z-index: var(--wb-z-toast);
  pointer-events: none;
}

.toast-stack {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  align-items: flex-end;
}

.toast {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  max-width: 380px;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-lg);
  background: var(--wb-raised);
  box-shadow: var(--wb-shadow-lift);
  pointer-events: auto;
  animation: toast-in var(--wb-dur-slow) var(--wb-ease-out);
}

@keyframes toast-in {
  from {
    opacity: 0;
    transform: translateY(8px) scale(0.97);
  }
}

.toast-dot {
  width: 6px;
  height: 6px;
  margin-top: 6px;
  border-radius: 50%;
  background: var(--wb-mute);
  flex: none;
}

.toast.ok .toast-dot {
  background: var(--wb-ok);
}
.toast.warn .toast-dot {
  background: var(--wb-warn);
}
.toast.error .toast-dot {
  background: var(--wb-bad);
}

.toast-text {
  min-width: 0;
}

.toast-msg {
  margin: 0;
  font-size: var(--wb-fs-sm);
  line-height: 1.5;
  color: var(--wb-ink);
  overflow-wrap: anywhere;
}

.toast-detail {
  margin: 3px 0 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  line-height: 1.45;
  color: var(--wb-faint);
  overflow-wrap: anywhere;
}

/* 动作按钮（如「撤销」）：比关闭钮显眼，但不能盖过提示正文 */
.toast-action {
  flex: none;
  align-self: center;
  padding: 2px 10px;
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-full);
  background: transparent;
  color: var(--wb-brand);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.toast-action:hover {
  background: var(--wb-brand-soft);
  border-color: var(--wb-brand-line);
  transform: scale(var(--wb-press-hover));
}

.toast-action:active {
  transform: scale(var(--wb-press-active));
}

.toast-close {
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  flex: none;
  border: none;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-faint);
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 小按钮也要按下反馈：缩放刻度复用全站令牌 */
.toast-close:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
  transform: scale(var(--wb-press-hover));
}
.toast-close:active {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}
</style>
