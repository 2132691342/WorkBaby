<script setup lang="ts">
// 关于：版本号与两个引擎的版本。日志与失败记录在「诊断」那一节（唯一落点），
// 两处各放一份的话，一份改了另一份就成了假的。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import type { EngineInfo } from '@/types/api'

const props = defineProps<{
  engines: EngineInfo[]
}>()

const { t } = useI18n()
const app = useAppStore()

/** 已装引擎的「kind 版本」清单；都没装时给一句「未知」。 */
const engineLine = computed(() => {
  const list = props.engines.filter((e) => e.installed)
  if (!list.length) return t('common.unknown')
  return list.map((e) => `${e.kind} ${e.version}`).join(' · ')
})
</script>

<template>
  <section class="card-pad">
    <div class="wb-section-title">{{ t('settings.groups.about') }}</div>
    <div class="set-row">
      <span class="set-label">{{ t('settings.aboutVersion') }}</span>
      <span class="fact-value">{{ app.version }}</span>
    </div>
    <div class="set-row">
      <span class="set-label">{{ t('settings.aboutEngine') }}</span>
      <span class="fact-value">{{ engineLine }}</span>
    </div>
  </section>
</template>

<style scoped>
.set-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) 0;
}

.set-label {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}

.fact-value {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
</style>
