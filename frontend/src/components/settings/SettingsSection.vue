<script setup lang="ts">
// 设置面板通用分节：标题 + 说明 + 右侧动作 + 内容。
// 七个面板共用同一套行距与描边，避免每个面板自己拼一遍「卡片头」。
import AppIcon from '../common/AppIcon.vue'

withDefaults(defineProps<{ title: string; desc?: string; icon?: string }>(), {
  desc: '',
  icon: '',
})
</script>

<template>
  <section class="sec">
    <header class="sec-head">
      <span v-if="icon" class="sec-tile"><AppIcon :name="icon" size="ic-sm" /></span>
      <div class="sec-tx">
        <h2 class="sec-title">{{ title }}</h2>
        <p v-if="desc" class="sec-desc">{{ desc }}</p>
      </div>
      <div class="sec-acts"><slot name="actions" /></div>
    </header>
    <div class="sec-body"><slot /></div>
  </section>
</template>

<style scoped>
.sec {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.sec-head {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-3);
}
.sec-tile {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  flex: none;
}
.sec-tx {
  min-width: 0;
  flex: 1;
}
.sec-title {
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
}
.sec-desc {
  margin-top: 2px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
  max-width: var(--wb-w-desc);
}
.sec-acts {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.sec-body {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
</style>
