<script setup lang="ts">
// 外观：主题、字号缩放、减弱动效。
// 三项都立刻生效：改完由 store 直接写进 CSS 变量，不重拉后端数据。
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

const emit = defineEmits<{
  save: [kv: Record<string, string>]
}>()

const { t } = useI18n()
const app = useAppStore()

/** 字号滑杆给的是 number 或 number[]（EP 的联合类型），统一取第一个。 */
function onFontScale(v: number | number[]): void {
  emit('save', { 'ui.font_scale': String(Array.isArray(v) ? v[0] : v) })
}
</script>

<template>
  <section class="card-pad">
    <div class="wb-section-title">{{ t('settings.groups.appearance') }}</div>

    <div class="set-row">
      <div class="set-label">{{ t('settings.theme') }}</div>
      <div class="seg">
        <button
          class="seg-btn"
          :class="{ active: app.theme === 'light' }"
          @click="emit('save', { 'ui.theme': 'light' })"
        >
          {{ t('settings.themeLight') }}
        </button>
        <button
          class="seg-btn"
          :class="{ active: app.theme === 'dark' }"
          @click="emit('save', { 'ui.theme': 'dark' })"
        >
          {{ t('settings.themeDark') }}
        </button>
      </div>
    </div>

    <div class="set-row">
      <div class="set-label">{{ t('settings.fontScale') }}</div>
      <el-slider
        :model-value="Number(app.settings['ui.font_scale'] ?? 1)"
        :min="0.85"
        :max="1.3"
        :step="0.05"
        :show-tooltip="false"
        style="max-width: 260px"
        @change="onFontScale"
      />
    </div>

    <div class="set-row">
      <div>
        <div class="set-label">{{ t('settings.reduceMotion') }}</div>
        <div class="set-hint">{{ t('settings.reduceMotionHint') }}</div>
      </div>
      <el-switch
        :model-value="app.settings['ui.reduce_motion'] === '1'"
        @change="(v: string | number | boolean) => emit('save', { 'ui.reduce_motion': v ? '1' : '0' })"
      />
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

.set-hint {
  margin: 2px 0 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  max-width: var(--wb-w-desc);
}
</style>
