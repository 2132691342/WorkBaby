<script setup lang="ts">
// 设置页：单页滚动，一节一件事。
// 只有外观、行为、诊断三节——模型 / 工具 / 技能 / 功能 / MCP 已升格为一级页，
// 把它们的开关再在这里列一遍就会出现「两处都能改、各自认为是权威」。
import { useI18n } from 'vue-i18n'
import PageHeader from '../components/layout/PageHeader.vue'
import AppearancePanel from '../components/settings/AppearancePanel.vue'
import BehaviorPanel from '../components/settings/BehaviorPanel.vue'
import InspectPanel from '../components/settings/InspectPanel.vue'

const { t } = useI18n()
</script>

<template>
  <div class="view">
    <PageHeader :title="t('settings.title')" :subtitle="t('settings.subtitle')" />

    <div class="body">
      <AppearancePanel />
      <BehaviorPanel />
      <InspectPanel />
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-5);
}
/* 面板自己决定内容宽度：表单类要收窄，清单类要宽 */
.body :deep(.panel-wrap) {
  max-width: var(--wb-panel-w, calc(920px * var(--wb-vw-scale)));
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
</style>
