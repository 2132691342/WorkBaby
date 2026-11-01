<script setup lang="ts">
// MCP 页：MCP 服务器的配置与在线状态。面板自带标题、重载按钮与空态，
// 这里只做一等入口的容器，与模型页同一处理。
import MCPPanel from '../components/settings/MCPPanel.vue'
</script>

<template>
  <div class="view">
    <div class="body">
      <MCPPanel />
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
</style>
