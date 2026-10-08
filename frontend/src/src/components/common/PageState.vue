<script setup lang="ts">
// 列表页三态包装：加载 → 错误 → 空态，正常则渲染内容。
import AppIcon from './AppIcon.vue'

defineProps<{ loading: boolean; error?: string; empty?: boolean; emptyTitle?: string; emptySub?: string }>()
</script>

<template>
  <div v-if="loading" class="ps-skeleton">
    <div class="sk-line w60" />
    <div class="sk-line w90" />
    <div class="sk-line w80" />
    <div class="sk-line w40" />
  </div>
  <div v-else-if="error" class="alert a-danger">
    <AppIcon name="alert" />
    <div>
      <div class="a-t">加载失败</div>
      <div class="em-s">{{ error }}</div>
    </div>
  </div>
  <div v-else-if="empty" class="empty">
    <div class="em-t">{{ emptyTitle || '这里还什么都没有' }}</div>
    <div v-if="emptySub" class="em-s">{{ emptySub }}</div>
    <div class="em-slot"><slot name="empty-action" /></div>
  </div>
  <slot v-else />
</template>

<style scoped>
.ps-skeleton {
  display: grid;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-5) 0;
}
/* 骨架用不透明度做呼吸：渐变的流光会把注意力从「内容还没到」拉到背景自己身上 */
.sk-line {
  height: 14px;
  border-radius: var(--wb-radius-xs);
  background: var(--wb-raise);
  animation: sk 1.2s ease-in-out infinite;
}
.w40 { width: 40%; }
.w60 { width: 60%; }
.w80 { width: 80%; }
.w90 { width: 90%; }
@keyframes sk {
  0%,
  100% {
    opacity: 0.5;
  }
  50% {
    opacity: 1;
  }
}
</style>
