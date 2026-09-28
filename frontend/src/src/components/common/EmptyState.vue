<script setup lang="ts">
// 空态基元：图标 + 标题 + 副文案，全站唯一实现。
import AppIcon from './AppIcon.vue'

withDefaults(defineProps<{ title: string; sub?: string; icon?: string }>(), { icon: 'info' })
</script>

<template>
  <div class="empty">
    <div class="em-ic">
      <slot name="icon"><AppIcon :name="icon" size="ic-lg" /></slot>
    </div>
    <div class="em-t">{{ title }}</div>
    <div v-if="sub" class="em-s">{{ sub }}</div>
    <div class="em-slot"><slot /></div>
  </div>
</template>

<style scoped>
.em-ic {
  width: 52px;
  height: 52px;
  border-radius: var(--wb-radius-lg);
  display: grid;
  place-items: center;
  background: var(--wb-surface-2);
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
  margin: 0 auto var(--wb-sp-4);
}
.em-slot {
  margin-top: var(--wb-sp-4);
  display: flex;
  justify-content: center;
  gap: var(--wb-sp-2);
}
</style>
