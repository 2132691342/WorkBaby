<script setup lang="ts">
// 全局 toast 容器：挂在 App 根部，任何页面的操作反馈都从这里出。
// 样式走 wb-ui.css 的 .toasts/.toast（半成品基础设施，这里补上消费端）。
import { useToastStore } from '../../stores/toast'
import AppIcon from './AppIcon.vue'

const toast = useToastStore()

const ICONS: Record<string, string> = { ok: 'check', bad: 'alert', info: 'info' }
</script>

<template>
  <div class="toasts">
    <div v-for="t in toast.items" :key="t.id" class="toast" :class="`is-${t.kind}`" @click="toast.dismiss(t.id)">
      <AppIcon :name="ICONS[t.kind]" size="ic-sm" />
      <span>{{ t.text }}</span>
    </div>
  </div>
</template>

<style scoped>
.toast {
  pointer-events: auto;
  cursor: pointer;
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
