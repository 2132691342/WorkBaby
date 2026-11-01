<script setup lang="ts">
// 功能页：内置能力的开关台。开一个功能，它带的工具就并入助手的工具表；
// 关一个功能，它带的工具整体撤出——这是本页与设置页里的旧入口共用同一份逻辑的原因。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FeatureList from '../components/features/FeatureList.vue'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()

const list = ref<InstanceType<typeof FeatureList> | null>(null)
// 刷新自身的忙碌态。
// 之前绑的是 :loading="!!list"，list 是组件 ref，挂载后恒为真——这个按钮会永久转圈。
const refreshing = ref(false)

const counts = computed(() => ({
  enabled: list.value?.enabledCount ?? 0,
  total: list.value?.items.length ?? 0,
}))

async function reload() {
  if (refreshing.value) return
  refreshing.value = true
  try {
    await list.value?.reload()
  } finally {
    refreshing.value = false
  }
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('features.title')" :subtitle="t('features.subtitle')">
      <template #actions>
        <span class="wb-label">{{ counts.enabled }} / {{ counts.total }}</span>
        <el-button size="small" :loading="refreshing" :disabled="refreshing" @click="reload">
          <AppIcon name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('common.refresh') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <FeatureList ref="list" />
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
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
