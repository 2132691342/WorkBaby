<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'

/**
 * 设置 · 搜索 tab：DuckDuckGo 联网搜索配置。
 *
 * <p>内容只有「启用开关 + 说明」，紧凑单卡布局：开关与说明同行，保存按钮右侧，
 * 不再套 section 标题层层撑高。
 */
const settings = useSettingsStore()
const toast = useToast()
const { webSearchConfig, error } = storeToRefs(settings)
const { saveWebSearchConfig } = settings

async function handleSaveWebSearch(): Promise<void> {
  if (await saveWebSearchConfig()) {
    toast.success(t('settings.websearchSaved'))
  } else {
    toast.error(error.value ?? t('common.saveFailed'))
  }
}
</script>

<template>
  <section class="card p-4">
    <div class="flex items-center justify-between gap-4 rounded-lg border border-wb-border bg-wb-surface-2 px-3 py-2.5">
      <div class="flex min-w-0 items-center gap-3">
        <el-switch
          :model-value="webSearchConfig.enabled === 'true'"
          @update:model-value="(v: string | number | boolean) => (webSearchConfig.enabled = String(v))"
        />
        <div class="min-w-0">
          <span class="text-sm text-wb-ink">{{ t('settings.websearchEnabled') }}</span>
          <p class="wb-form-hint">{{ t('settings.websearchHint') }}</p>
        </div>
      </div>
      <el-button type="primary" @click="handleSaveWebSearch">{{ t('settings.saveWebSearch') }}</el-button>
    </div>
  </section>
</template>
