<script setup lang="ts">
// 功能页：内置能力的开关台。开一个功能，它带的工具就并入助手的工具表；
// 关一个功能，它带的工具整体撤出——这是本页与设置页里的旧入口共用同一份逻辑的原因。
// 第二段是记忆库：记忆明文落盘，用户必须能看见里面存了什么、并能删掉不该在的。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FeatureList from '../components/features/FeatureList.vue'
import MemoryPanel from '../components/features/MemoryPanel.vue'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()

/** 两段：功能开关 / 记忆库。记忆库单独一段而不是塞进清单，因为它是数据不是配置 */
const tab = ref<'features' | 'memory'>('features')

const list = ref<InstanceType<typeof FeatureList> | null>(null)
const memoryRef = ref<InstanceType<typeof MemoryPanel> | null>(null)
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
    await memoryRef.value?.reload()
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

    <div class="tabs">
      <button
        class="wb-row tab"
        :class="{ 'is-on': tab === 'features' }"
        type="button"
        @click="tab = 'features'"
      >
        <AppIcon name="zap" size="ic-xs" />
        {{ t('features.tabFeatures') }}
      </button>
      <button
        class="wb-row tab"
        :class="{ 'is-on': tab === 'memory' }"
        type="button"
        @click="tab = 'memory'"
      >
        <AppIcon name="brain" size="ic-xs" />
        {{ t('features.tabMemory') }}
      </button>
    </div>

    <div class="body">
      <FeatureList v-show="tab === 'features'" ref="list" />
      <MemoryPanel v-show="tab === 'memory'" ref="memoryRef" />
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
.tabs {
  display: flex;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-2) var(--wb-sp-5) 0;
  flex: none;
}
.tab {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-1) var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  border: 1px solid transparent;
  background: transparent;
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-muted);
  cursor: pointer;
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease);
}
.tab:hover {
  color: var(--wb-ink);
  background: var(--wb-elev-hover);
}
.tab.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary);
  color: var(--wb-primary);
}
</style>
