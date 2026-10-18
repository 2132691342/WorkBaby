<script setup lang="ts">
// 面板统一页头：图标瓦片 + 标题 + 副题 + 右侧动作区。设置页七个面板共用。
import AppIcon from './AppIcon.vue'

withDefaults(defineProps<{ icon?: string; title: string; sub?: string }>(), { icon: 'info' })
</script>

<template>
  <header class="pn-head">
    <span class="pn-tile"><AppIcon :name="icon" size="ic" /></span>
    <div class="pn-txt">
      <h3>{{ title }}</h3>
      <p v-if="sub" class="pn-sub"><slot name="sub">{{ sub }}</slot></p>
    </div>
    <span class="sp" />
    <div class="pn-acts"><slot /></div>
  </header>
</template>

<style scoped>
.pn-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.pn-tile {
  width: var(--wb-tile);
  height: var(--wb-tile);
  border-radius: var(--wb-radius);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.pn-txt {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1;
}
.pn-txt h3 {
  font-size: var(--wb-fs-lg);
  font-weight: 600;
  color: var(--wb-ink);
  letter-spacing: -0.01em;
}
.pn-sub {
  margin: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  line-height: 1.6;
  max-width: var(--wb-w-desc);
}
.sp {
  flex: 1;
  min-width: 0;
}
.pn-acts {
  display: flex;
  gap: var(--wb-sp-2);
  align-items: center;
  flex: none;
}
</style>
