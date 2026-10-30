<script setup lang="ts">
// 技能面板（设置页入口）：与独立技能页共用同一份清单、开关与新建逻辑。
import { useI18n } from 'vue-i18n'
import SkillList from '../skills/SkillList.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="book" :title="t('settings.skills.title')" :desc="t('settings.skills.desc')">
      <SkillList />
    </SettingsSection>
  </div>
</template>

<style scoped>
.panel-wrap {
  display: flex;
  flex-direction: column;
}
</style>
