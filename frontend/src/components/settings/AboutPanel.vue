<script setup lang="ts">
// 关于：版本号与运行时版本。日志与失败记录在「诊断」那一节（唯一落点），
// 两处各放一份的话，一份改了另一份就成了假的。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const app = useAppStore()

/** 运行时版本清单：「node v24 · git 2.47 · python 3.13 · pi 0.42」。 */
const runtimeLine = computed(() => {
  const rt = app.runtime
  if (!rt) return t('common.unknown')
  const parts = [
    rt.node_version && `node ${rt.node_version}`,
    rt.git_version && `git ${rt.git_version.replace(/^git version /, '')}`,
    rt.python_version && `python ${rt.python_version.replace(/^Python /, '')}`,
    rt.pi_version && `pi ${rt.pi_version}`,
  ].filter(Boolean)
  return parts.length ? parts.join(' · ') : t('common.unknown')
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
      <span class="set-label">{{ t('settings.aboutRuntime') }}</span>
      <span class="fact-value">{{ runtimeLine }}</span>
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
