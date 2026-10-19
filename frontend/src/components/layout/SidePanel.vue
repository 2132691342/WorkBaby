<script setup lang="ts">
// 视图左侧面板外壳：固定宽度 + 头（标题 / 动作）+ 可滚动列表 + 可选底部。
// 对话的会话列表、设置的板块目录、帮助的文档目录共用同一套尺寸与分隔规则。
// sub 档给已经有一级 rail 的视图用——二级目录要比一级窄，否则读作两根侧栏。
withDefaults(defineProps<{ title?: string; width?: 'normal' | 'wide' | 'sub' }>(), {
  title: '',
  width: 'normal',
})
</script>

<template>
  <aside class="side-panel" :class="`is-${width}`">
    <div v-if="title || $slots.head" class="sp-head">
      <span v-if="title" class="wb-label">{{ title }}</span>
      <div class="sp-head-acts"><slot name="head" /></div>
    </div>
    <div class="sp-body">
      <slot />
    </div>
    <div v-if="$slots.foot" class="sp-foot">
      <slot name="foot" />
    </div>
  </aside>
</template>

<style scoped>
.side-panel {
  width: var(--wb-w-side);
  flex: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--wb-surface);
  border-right: 1px solid var(--wb-border);
}
.side-panel.is-wide {
  width: var(--wb-w-panel);
}
/* 二级目录：rail 已在左侧占位，这里收窄以示层次差异 */
.side-panel.is-sub {
  width: var(--wb-w-subside);
  background: var(--wb-elev-workspace);
}
.sp-head {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4) var(--wb-sp-3) var(--wb-sp-2);
}
.sp-head-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
}
.sp-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 var(--wb-sp-2) var(--wb-sp-3);
}
.sp-foot {
  flex: none;
  border-top: 1px solid var(--wb-border);
  padding: var(--wb-sp-3);
}
</style>
