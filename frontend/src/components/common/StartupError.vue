<script setup lang="ts">
// 启动失败页：把真实原因与下一步都写出来，并给出「看日志」「重试」两个动作。
import { useAppStore } from '@/stores/app'

defineProps<{ message: string }>()
const app = useAppStore()
</script>

<template>
  <div class="fail card-pad">
    <div class="fail-head">
      <span class="tag bad">启动失败</span>
    </div>
    <p class="fail-msg selectable">{{ message }}</p>
    <p class="fail-hint">
      后端没有起来。先看日志里最后几行；如果是端口或数据库被占用，重启本应用通常能解决。
    </p>
    <div class="fail-actions">
      <button class="wb-btn primary" @click="app.load()">重试</button>
    </div>
  </div>
</template>

<style scoped>
.fail {
  max-width: 520px;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.fail-msg {
  margin: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-sm);
  line-height: 1.6;
  color: var(--wb-bad);
  overflow-wrap: anywhere;
}

.fail-hint {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
  line-height: 1.65;
}

.fail-actions {
  display: flex;
  gap: var(--wb-sp-2);
}
</style>
