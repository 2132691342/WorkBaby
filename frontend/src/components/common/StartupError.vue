<script setup lang="ts">
// 启动失败页：把真实原因与下一步都写出来，并给出「看日志」「重试」两个动作。
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

defineProps<{ message: string }>()
const app = useAppStore()
const { t } = useI18n()
</script>

<template>
  <div class="fail card-pad">
    <div class="fail-head">
      <span class="tag bad">{{ t('app.failTag') }}</span>
    </div>
    <p class="fail-msg selectable">{{ message }}</p>
    <p class="fail-hint">
      {{ t('app.failHint') }}
    </p>
    <div class="fail-actions">
      <button class="wb-btn primary" @click="app.load()">{{ t('common.retry') }}</button>
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
