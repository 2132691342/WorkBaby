<!-- 行为面板：这个程序自己的习惯——默认工作区、界面语言与窗口 / 启动行为。
     视觉（主题、字体、背景）在外观面板；内置能力的开关在功能面板。 -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '../../stores/settings'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import { autostartGet, autostartSet, pickFolder } from '../../api/shell'
import * as api from '../../api'
import { LOCALES, setLocale, type Locale } from '../../i18n'
import SettingsSection from './SettingsSection.vue'
import { errorText } from '../../utils/errors'

const { t } = useI18n()
const settings = useSettingsStore()
const session = useSessionStore()
const toast = useToastStore()

const busy = ref('')
const autoStart = ref(false)

const workspace = computed(() => settings.boot?.boot_workspace || settings.values['workspace'] || '')
const sendOnEnter = computed(() => settings.values['send_on_enter'] !== 'false')
const tray = computed(() => settings.values['minimize_to_tray'] !== 'false')
const currentLocale = computed<Locale>(() => (settings.values['language'] === 'en' ? 'en' : 'zh-CN'))

async function pickWorkspace() {
  const dir = await pickFolder(t('settings.behavior.workspacePick'))
  if (!dir || dir === workspace.value) return
  busy.value = 'workspace'
  try {
    const resp = await api.workspace.switch(dir)
    // 切换在后端已完成（重扫技能 / 重建注册表）：前端只重读快照与清单
    await settings.loadBoot()
    await settings.warmup()
    session.draft()
    await session.loadList()
    toast.ok(t('settings.behavior.workspaceSwitched', { path: resp.resp_workspace }))
  } catch (e) {
    toast.bad(errorText(e) || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
  }
}

async function toggleValue(key: string, next: boolean) {
  try {
    await settings.setValue(key, next ? 'true' : 'false')
  } catch {
    /* setValue 已提示并回滚 */
  }
}

async function chooseLanguage(locale: Locale) {
  if (locale === currentLocale.value) return
  try {
    await settings.setValue('language', locale)
    setLocale(locale)
  } catch {
    /* setValue 已提示 */
  }
}

async function toggleAutoStart() {
  const next = !autoStart.value
  busy.value = 'autostart'
  try {
    await autostartSet(next)
    autoStart.value = next
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

onMounted(async () => {
  try {
    autoStart.value = await autostartGet()
  } catch {
    autoStart.value = false
  }
})
</script>

<template>
  <div class="panel-wrap wb-form">
    <SettingsSection icon="folder" :title="t('settings.behavior.workspace')" :desc="t('settings.behavior.workspaceHint')">
      <template #actions>
        <el-button size="small" :loading="busy === 'workspace'" :disabled="busy === 'workspace'" @click="pickWorkspace">
          {{ t('settings.behavior.workspacePick') }}
        </el-button>
      </template>
      <p class="path-line" :title="workspace">{{ workspace || '—' }}</p>
      <p class="hint-line">{{ t('settings.behavior.workspaceLive') }}</p>
    </SettingsSection>

    <SettingsSection icon="keyboard" :title="t('settings.behavior.input')" :desc="t('settings.behavior.inputHint')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.language') }}</b>
          <p>{{ t('settings.behavior.languageHint') }}</p>
        </div>
        <el-radio-group :model-value="currentLocale" @change="(v: string | number | boolean | undefined) => chooseLanguage(v as Locale)">
          <el-radio-button v-for="l in LOCALES" :key="l.key" :value="l.key">{{ l.name }}</el-radio-button>
        </el-radio-group>
      </div>

      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.sendOnEnter') }}</b>
          <p>{{ t('settings.behavior.sendOnEnterHint') }}</p>
        </div>
        <el-switch :model-value="sendOnEnter" @change="toggleValue('send_on_enter', !sendOnEnter)" />
      </div>
    </SettingsSection>

    <SettingsSection icon="monitor" :title="t('settings.behavior.window')" :desc="t('settings.behavior.windowHint')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.minimizeToTray') }}</b>
          <p>{{ t('settings.behavior.minimizeToTrayHint') }}</p>
        </div>
        <el-switch :model-value="tray" @change="toggleValue('minimize_to_tray', !tray)" />
      </div>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.autostart') }}</b>
          <p>{{ t('settings.behavior.autostartHint') }}</p>
        </div>
        <el-switch :model-value="autoStart" :loading="busy === 'autostart'" :disabled="busy === 'autostart'" @change="toggleAutoStart" />
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
/* 表单类面板收窄：控件列与标签列都靠左，面板宽 920px 时右侧会拖出一片空地 */
.panel-wrap {
  --wb-panel-w: calc(720px * var(--wb-vw-scale));
}
.path-line {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.hint-line {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
</style>
