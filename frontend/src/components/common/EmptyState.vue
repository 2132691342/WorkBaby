<script setup lang="ts">
// 空态：一块图标 + 一句结论 + 一句指引。功能面板与列表页共用，
// 避免每处各写一遍居中排版（间距一旦漂移，全站空态就不整齐）。
import AppIcon from './AppIcon.vue'

withDefaults(
  defineProps<{
    /** AppIcon 图标名 */
    icon?: string
    /** 主文案（一句结论） */
    title: string
    /** 副文案（下一步做什么） */
    desc?: string
    /** 紧凑档：嵌在面板里时不撑满高度 */
    compact?: boolean
  }>(),
  { icon: 'info', desc: '', compact: false },
)
</script>

<template>
  <div class="empty" :class="{ 'is-compact': compact }">
    <span class="empty-tile"><AppIcon :name="icon" size="ic-lg" /></span>
    <p class="empty-title">{{ title }}</p>
    <p v-if="desc" class="empty-desc">{{ desc }}</p>
    <div v-if="$slots.default" class="empty-acts"><slot /></div>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
}
.empty.is-compact {
  padding: var(--wb-sp-5) var(--wb-sp-3);
}
.empty-tile {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-lg);
  display: grid;
  place-items: center;
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  margin-bottom: var(--wb-sp-1);
}
.empty-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.empty-desc {
  margin: 0;
  max-width: 46ch;
  font-size: var(--wb-fs-2xs);
  line-height: 1.55;
  color: var(--wb-muted);
}
.empty-acts {
  margin-top: var(--wb-sp-2);
  display: flex;
  gap: var(--wb-sp-2);
}
</style>
