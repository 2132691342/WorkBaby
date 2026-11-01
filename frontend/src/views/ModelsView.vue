<script setup lang="ts">
// 模型页：一等入口，不再藏在设置的二级 tab 里。
// 加模型是「能不能开始对话」的前置动作，藏两层会让人以为程序坏了。
// 面板自带标题与操作区（SettingsSection），这里只铺滚动容器，不再叠一层页头。
import ProviderPanel from '../components/settings/ProviderPanel.vue'
import ToolPanel from '../components/settings/ToolPanel.vue'
</script>

<template>
  <div class="view">
    <div class="body">
      <ProviderPanel />
      <ToolPanel class="next" />
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
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
}
.next {
  margin-top: var(--wb-sp-5);
}
</style>
