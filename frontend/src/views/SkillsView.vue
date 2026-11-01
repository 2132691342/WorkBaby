<script setup lang="ts">
// 技能页：内置技能包与用户自建技能的清单台。内置技能只读（不可删改），
// 用户技能可开关、可删除；正文在抽屉里看。
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import SkillList from '../components/skills/SkillList.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()

const list = ref<InstanceType<typeof SkillList> | null>(null)
// 导入按钮自己也要有忙碌态：打开文件选择框那几百毫秒里没反馈，点下去就像失灵
const importing = ref(false)

async function importSkill() {
  if (importing.value) return
  importing.value = true
  try {
    await list.value?.importSkill()
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('skills.title')" :subtitle="t('skills.subtitle')">
      <template #actions>
        <el-button size="small" :loading="importing" :disabled="importing" @click="importSkill">
          {{ t('settings.skills.import') }}
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <SkillList ref="list" />
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
