<script setup lang="ts">
// 功能开关面板（设置页入口）：与独立功能页共用同一份清单与开关逻辑。
import { useI18n } from 'vue-i18n'
import FeatureList from '../features/FeatureList.vue'
import SettingsSection from './SettingsSection.vue'

const { t, te } = useI18n()

const title = te('settings.features.title') ? t('settings.features.title') : '内置功能'
const desc = te('settings.features.desc')
  ? t('settings.features.desc')
  : '内置能力的开关：关掉后它带的工具不再进入助手的工具表。'
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="zap" :title="title" :desc="desc">
      <FeatureList />
    </SettingsSection>
  </div>
</template>

<style scoped>
.panel-wrap {
  display: flex;
  flex-direction: column;
}
</style>
