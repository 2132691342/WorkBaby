<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore, CODE_FONTS } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { THEMES } from '@/composables/useTheme'
import { t } from '@/i18n'

/**
 * 设置 · 外观 tab：语言 / 主题 / 背景图 / 代码字体。
 */
const settings = useSettingsStore()
const toast = useToast()
const { general, backgroundUrl, codeFont, theme, fontScale, error } = storeToRefs(settings)
const { saveGeneral, setBackground, setCodeFont, setTheme, setFontScale } = settings

/** 缩放档位展示（百分比）。 */
const fontScalePercent = computed(() => `${Math.round(fontScale.value * 100)}%`)

async function onFontScaleChange(val: number): Promise<void> {
  await setFontScale(val)
}

async function handleSaveGeneral(): Promise<void> {
  if (await saveGeneral()) {
    toast.success(t('settings.generalSaved'))
  } else {
    toast.error(error.value ?? t('common.saveFailed'))
  }
}

// 背景图上传：读文件 → 存 backgroundImageID → 换取预览 URL
async function onBgFile(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) await setBackground(file)
  input.value = ''
}

async function removeBackground(): Promise<void> {
  await setBackground(null)
}

function onCodeFontChange(val: string): void {
  setCodeFont(val)
}

async function onThemeChange(val: string): Promise<void> {
  await setTheme(val)
}
</script>

<template>
  <section class="card p-4">
    <h2 class="mb-3 font-display text-sm font-semibold text-wb-ink">
      {{ t('settings.tab.appearance') }}
    </h2>
    <div class="space-y-4">
      <div class="wb-form-section">
        <h3 class="wb-form-section__title">{{ t('settings.section.locale') }}</h3>
        <el-form class="wb-el-form wb-form-grid" label-position="top">
          <div class="wb-form-row wb-form-row--2">
            <el-form-item :label="t('settings.language')" class="wb-form-cell">
              <el-input v-model="(general.language as string)" placeholder="zh-CN" />
            </el-form-item>
            <el-form-item :label="t('settings.theme')" class="wb-form-cell">
              <el-select :model-value="theme" class="w-full" @change="onThemeChange">
                <el-option v-for="th in THEMES" :key="th.id" :value="th.id" :label="t(th.nameKey)" />
              </el-select>
            </el-form-item>
          </div>
          <div class="wb-form-actions">
            <el-button type="primary" @click="handleSaveGeneral">{{ t('settings.saveGeneral') }}</el-button>
          </div>
        </el-form>
      </div>

      <div class="wb-form-section">
        <h3 class="wb-form-section__title">{{ t('settings.section.backdrop') }}</h3>
        <div class="flex flex-wrap items-center gap-3">
          <label class="cursor-pointer">
            <el-button type="primary" plain>{{ t('settings.uploadBackground') }}</el-button>
            <input type="file" accept="image/*" class="hidden" @change="onBgFile" />
          </label>
          <el-button v-if="backgroundUrl" @click="removeBackground">{{ t('settings.removeBackground') }}</el-button>
          <span v-else class="text-xs text-wb-muted">{{ t('settings.noBackground') }}</span>
        </div>
        <el-image
          v-if="backgroundUrl"
          :src="backgroundUrl"
          fit="cover"
          class="mt-3 h-28 w-48 rounded-lg border border-wb-border"
        />
      </div>

      <div class="wb-form-section">
        <h3 class="wb-form-section__title">{{ t('settings.section.typography') }}</h3>
        <el-form class="wb-el-form wb-form-grid" label-position="top">
          <div class="wb-form-row wb-form-row--2">
            <el-form-item :label="t('settings.codeFont')" class="wb-form-cell">
              <el-select :model-value="codeFont" class="!w-64" @change="onCodeFontChange">
                <el-option v-for="f in CODE_FONTS" :key="f" :value="f" :label="f" />
              </el-select>
            </el-form-item>
            <el-form-item :label="`${t('settings.uiScale')}（${fontScalePercent}）`" class="wb-form-cell">
              <div class="flex w-64 items-center gap-3">
                <el-slider
                  v-model="fontScale"
                  :min="0.85"
                  :max="1.25"
                  :step="0.05"
                  :marks="{ 0.85: '85%', 1: '100%', 1.25: '125%' }"
                  class="flex-1"
                  @change="onFontScaleChange"
                />
              </div>
            </el-form-item>
          </div>
        </el-form>
        <p
          class="mt-3 rounded-lg border border-wb-border bg-wb-surface-2 px-3 py-2 text-sm text-wb-ink"
          :style="{ fontFamily: `'${codeFont}', ui-monospace, monospace` }"
        >
          {{ t('settings.codeFontPreview') }} — 1234567890 &lt;code&gt;{ x }&lt;/code&gt;
        </p>
      </div>
    </div>
  </section>
</template>
