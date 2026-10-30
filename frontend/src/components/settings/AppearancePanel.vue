<script setup lang="ts">
// 外观面板：只看「看得见的样子」——主题、字体字号、消息显示与背景。
// 语言与各项开关在「行为」面板；内置能力的开关在「功能」面板。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import {
  FONTS,
  FONT_SIZES,
  bgBlur,
  bgImage,
  bgStrength,
  currentFont,
  currentSize,
  syncFromSettings,
} from '../../composables/useAppearance'
import { useTheme } from '../../composables/useTheme'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'
import { pickFile } from '../../api/shell'

const { t } = useI18n()
const { theme, setTheme, THEMES } = useTheme()
const store = useSettingsStore()
const toast = useToastStore()

const FONT_KEYS: Record<string, string> = {
  system: 'fontSystem',
  song: 'fontSong',
  kai: 'fontKai',
  hei: 'fontHei',
  fang: 'fontFang',
}
const SIZE_KEYS: Record<string, string> = {
  sm: 'fontSizeSm',
  md: 'fontSizeMd',
  lg: 'fontSizeLg',
  xl: 'fontSizeXl',
}

async function setFont(key: string) {
  currentFont.value = key
  try {
    await store.setValue('font_family', key)
  } catch {
    /* setValue 已提示 */
  }
}

async function setSize(key: string) {
  currentSize.value = key
  try {
    await store.setValue('font_size', key)
  } catch {
    /* setValue 已提示 */
  }
}

const thinkingBusy = ref(false)
const showThinking = computed(() => store.values['show_thinking'] !== 'false')
async function toggleThinking() {
  if (thinkingBusy.value) return
  const next = !showThinking.value
  thinkingBusy.value = true
  try {
    await store.setValue('show_thinking', next ? 'true' : 'false')
    useChatStore().setShowThinking(next)
  } catch {
    /* setValue 已提示并回滚 */
  } finally {
    thinkingBusy.value = false
  }
}

// ---- 背景：浓度与模糊用离散档，这两项不需要连续取值 ----
const BG_STRENGTHS = [
  { key: 'soft', label: 'Soft', value: 0.25 },
  { key: 'mid', label: 'Mid', value: 0.5 },
  { key: 'bold', label: 'Bold', value: 0.75 },
]
const BG_BLURS = [
  { key: 'none', label: 'None', value: 0 },
  { key: 'light', label: 'Light', value: 6 },
  { key: 'mid', label: 'Mid', value: 14 },
]

const bgBusy = ref(false)
const hasBg = computed(() => bgImage.value !== '')

function applyBgVo(image: string, strength: number, blur: number) {
  bgImage.value = image
  bgStrength.value = strength
  bgBlur.value = blur
  store.values = {
    ...store.values,
    bg_image: image,
    bg_opacity: String(strength),
    bg_blur: String(blur),
  }
}

async function saveBg(path: string, strength: number, blur: number) {
  if (bgBusy.value) return
  bgBusy.value = true
  try {
    const vo = await api.settings.setBackground(path, strength, blur)
    applyBgVo(vo.resp_background_file, vo.resp_background_opacity, vo.resp_background_blur)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    bgBusy.value = false
  }
}

async function pickBackground() {
  if (bgBusy.value) return
  const p = await pickFile(t('settings.appearance.backgroundPick'), '*.png;*.jpg;*.jpeg;*.webp;*.bmp')
  if (!p) return
  await saveBg(p, bgStrength.value, bgBlur.value)
}

const strengthKey = computed(() => {
  const hit = BG_STRENGTHS.find((s) => Math.abs(s.value - bgStrength.value) < 0.01)
  return hit?.key || 'mid'
})
const blurKey = computed(() => {
  const hit = BG_BLURS.find((s) => Math.abs(s.value - bgBlur.value) < 0.5)
  return hit?.key || 'none'
})

async function loadBg() {
  try {
    const vo = await api.settings.background()
    applyBgVo(vo.resp_background_file, vo.resp_background_opacity, vo.resp_background_blur)
  } catch {
    /* 背景不是关键设置，读不到就保持上一次显示 */
  }
}

onMounted(async () => {
  syncFromSettings()
  await loadBg()
})
</script>

<template>
  <div class="panel-wrap wb-form">
    <SettingsSection icon="contrast" :title="t('settings.appearance.theme')">
      <div class="theme-grid">
        <button
          v-for="th in THEMES"
          :key="th.key"
          class="theme-card"
          type="button"
          :class="{ 'is-on': theme === th.key }"
          @click="setTheme(th.key)"
        >
          <span class="tc-preview" :class="`is-${th.key}`">
            <span class="tc-bar" />
            <span class="tc-body">
              <span class="tc-block" />
              <span class="tc-block is-sm" />
            </span>
          </span>
          <span class="tc-foot">
            <span class="tc-name">{{ t(`settings.appearance.theme${th.key === 'light' ? 'Light' : 'Dark'}`) }}</span>
            <span class="tc-dot" :style="{ background: th.swatch }" />
            <AppIcon v-if="theme === th.key" name="check" size="ic-xs" class="tc-check" />
          </span>
        </button>
      </div>
    </SettingsSection>

    <SettingsSection icon="type" :title="t('settings.appearance.textSection')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx"><b>{{ t('settings.appearance.fontFamily') }}</b></div>
        <el-radio-group :model-value="currentFont" @change="(v: string | number | boolean | undefined) => setFont(String(v))">
          <el-radio-button v-for="f in FONTS" :key="f.key" :value="f.key">
            {{ t(`settings.appearance.${FONT_KEYS[f.key]}`) }}
          </el-radio-button>
        </el-radio-group>
      </div>

      <div class="wb-set-row">
        <div class="wb-set-row-tx"><b>{{ t('settings.appearance.fontSize') }}</b></div>
        <el-radio-group :model-value="currentSize" @change="(v: string | number | boolean | undefined) => setSize(String(v))">
          <el-radio-button v-for="s in FONT_SIZES" :key="s.key" :value="s.key">
            {{ t(`settings.appearance.${SIZE_KEYS[s.key]}`) }}
          </el-radio-button>
        </el-radio-group>
      </div>
    </SettingsSection>

    <SettingsSection icon="brain" :title="t('settings.appearance.messageSection')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.appearance.showThinking') }}</b>
          <p>{{ t('settings.appearance.showThinkingHint') }}</p>
        </div>
        <el-switch :model-value="showThinking" :loading="thinkingBusy" :disabled="thinkingBusy" @change="toggleThinking" />
      </div>
    </SettingsSection>

    <SettingsSection icon="image" :title="t('settings.appearance.background')" :desc="t('settings.appearance.backgroundHint')" divided>
      <template #actions>
        <el-button size="small" :loading="bgBusy" @click="pickBackground">
          {{ t('settings.appearance.backgroundPick') }}
        </el-button>
        <el-button v-if="hasBg" size="small" :disabled="bgBusy" @click="saveBg('', bgStrength, bgBlur)">
          {{ t('settings.appearance.backgroundClear') }}
        </el-button>
      </template>

      <div class="wb-set-row">
        <div class="wb-set-row-tx"><b>{{ t('settings.appearance.opacity') }}</b></div>
        <el-radio-group
          :model-value="strengthKey"
          size="small"
          :disabled="!hasBg || bgBusy"
          @change="(v: string | number | boolean | undefined) => saveBg(bgImage, BG_STRENGTHS.find((x) => x.key === v)?.value ?? 0.5, bgBlur)"
        >
          <el-radio-button v-for="s in BG_STRENGTHS" :key="s.key" :value="s.key">
            {{ t(`settings.appearance.opacity${s.label}`) }}
          </el-radio-button>
        </el-radio-group>
      </div>

      <div class="wb-set-row">
        <div class="wb-set-row-tx"><b>{{ t('settings.appearance.blur') }}</b></div>
        <el-radio-group
          :model-value="blurKey"
          size="small"
          :disabled="!hasBg || bgBusy"
          @change="(v: string | number | boolean | undefined) => saveBg(bgImage, bgStrength, BG_BLURS.find((x) => x.key === v)?.value ?? 0)"
        >
          <el-radio-button v-for="b in BG_BLURS" :key="b.key" :value="b.key">
            {{ t(`settings.appearance.blur${b.label}`) }}
          </el-radio-button>
        </el-radio-group>
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
/* 表单类面板收窄：控件列与标签列都靠左，面板宽 920px 时右侧会拖出一片空地 */
.panel-wrap {
  --wb-panel-w: calc(720px * var(--wb-vw-scale));
}
.theme-grid {
  display: flex;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.theme-card {
  width: calc(200px * var(--wb-vw-scale));
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease);
}
.theme-card:hover {
  border-color: var(--wb-border-strong);
  box-shadow: var(--wb-card-shadow-hover);
}
.theme-card:active {
  transform: scale(0.98);
}
.theme-card.is-on {
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 2px var(--wb-accent-tint);
}
.tc-preview {
  display: flex;
  flex-direction: column;
  height: calc(84px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  overflow: hidden;
  border: 1px solid var(--wb-border);
}
/* 下面两张缩略图要同时展示浅色与深色两套主题，因此用两套主题的字面值，不跟当前主题走。 */
.tc-preview.is-light {
  background: #f8fafc;
}
.tc-preview.is-dark {
  background: #0f172a;
}
.tc-bar {
  height: calc(14px * var(--wb-vw-scale));
  background: #ffffff;
  border-bottom: 1px solid #e2e8f0;
}
.tc-preview.is-dark .tc-bar {
  background: #1e293b;
  border-color: rgba(148, 163, 184, 0.2);
}
.tc-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px;
}
.tc-block {
  height: 10px;
  border-radius: 3px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
}
.tc-preview.is-dark .tc-block {
  background: #1e293b;
  border-color: rgba(148, 163, 184, 0.2);
}
.tc-block.is-sm {
  width: 56%;
  background: #e0e7ff;
  border-color: #c7d2fe;
}
.tc-preview.is-dark .tc-block.is-sm {
  background: rgba(129, 140, 248, 0.3);
  border-color: rgba(129, 140, 248, 0.4);
}
.tc-foot {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.tc-name {
  flex: 1;
  font-size: var(--wb-fs-xs);
  font-weight: 700;
  color: var(--wb-ink);
  text-align: left;
}
.tc-dot {
  width: 10px;
  height: 10px;
  border-radius: var(--wb-radius-full);
}
.tc-check {
  color: var(--wb-primary);
}
</style>
