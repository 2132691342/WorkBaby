<script setup lang="ts">
// 内容区页头：标题 + 副标题 + 右侧动作区。所有视图共用一套间距与字重。
defineProps<{ title: string; subtitle?: string }>()
</script>

<template>
  <div class="page-head">
    <div class="ph-tx">
      <h1 class="ph-title">{{ title }}</h1>
      <p v-if="subtitle" class="ph-sub">{{ subtitle }}</p>
    </div>
    <div class="ph-acts">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.page-head {
  flex: none;
  min-height: var(--wb-h-pagehead);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-4) var(--wb-sp-5);
  border-bottom: 1px solid var(--wb-border);
  background: var(--wb-bg);
  position: sticky;
  top: 0;
  z-index: 2;
}
.ph-tx {
  min-width: 0;
}
.ph-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 700;
  letter-spacing: -0.015em;
  line-height: var(--wb-lh-tight);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ph-sub {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ph-acts {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
</style>
