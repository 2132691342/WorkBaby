<script setup lang="ts">
// 行为面板：工作目录、回车与托盘、默认思考档、压缩参数，以及 PI 的配置文件位置。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { AutoStartEnabled, OpenDirectoryDialog, SetAutoStart } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const settings = useSettingsStore()
const toast = useToastStore()

const LEVELS = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

const reserve = ref(0)
const keep = ref(0)
const level = ref('')
const busy = ref('')
const autoStart = ref(false)

const workspace = computed(() => settings.values['workspace'] || settings.boot?.workspace || '')
const paths = computed(() => settings.piSettings)
const sendOnEnter = computed(() => settings.values['send_on_enter'] !== 'false')
const tray = computed(() => settings.values['minimize_to_tray'] !== 'false')

async function pickWorkspace() {
  const dir = await OpenDirectoryDialog(t('settings.behavior.workspacePick'))
  if (!dir || dir === workspace.value) return
  try {
    await settings.setValue('workspace', dir)
    await settings.loadBoot()
    toast.ok(dir)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  }
}

async function toggleValue(key: string, next: boolean) {
  try {
    await settings.setValue(key, next ? 'true' : 'false')
  } catch {
    /* setValue 已提示并回滚 */
  }
}

async function saveThinking() {
  busy.value = 'thinking'
  try {
    await api.settings.setThinking(level.value)
    await settings.loadPISettings()
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function saveCompaction() {
  busy.value = 'compaction'
  try {
    await api.settings.setCompaction(Number(reserve.value) || 0, Number(keep.value) || 0)
    await settings.loadPISettings()
    toast.ok(t('common.save'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function toggleAutoStart() {
  const next = !autoStart.value
  try {
    await SetAutoStart(next)
    autoStart.value = next
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  }
}

async function copyPath(p: string) {
  if (!p) return
  try {
    await navigator.clipboard.writeText(p)
    toast.ok(t('common.copiedPath'))
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}

onMounted(async () => {
  if (!settings.piSettings) await settings.loadPISettings()
  level.value = settings.piSettings?.default_thinking_level || ''
  reserve.value = settings.piSettings?.compaction_reserve || 0
  keep.value = settings.piSettings?.compaction_keep || 0
  try {
    autoStart.value = await AutoStartEnabled()
  } catch {
    autoStart.value = false
  }
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="folder" :title="t('settings.behavior.workspace')" :desc="t('settings.behavior.workspaceHint')">
      <template #actions>
        <el-button size="small" @click="pickWorkspace">{{ t('settings.behavior.workspacePick') }}</el-button>
      </template>
      <p class="path-line" :title="workspace">{{ workspace || '—' }}</p>
    </SettingsSection>

    <SettingsSection icon="sliders" :title="t('settings.behavior.title')">
      <div class="row">
        <div class="row-tx">
          <b>{{ t('settings.behavior.sendOnEnter') }}</b>
          <p>{{ t('settings.behavior.sendOnEnterHint') }}</p>
        </div>
        <el-switch :model-value="sendOnEnter" @change="toggleValue('send_on_enter', !sendOnEnter)" />
      </div>
      <div class="row">
        <div class="row-tx">
          <b>{{ t('settings.behavior.minimizeToTray') }}</b>
          <p>{{ t('settings.behavior.minimizeToTrayHint') }}</p>
        </div>
        <el-switch :model-value="tray" @change="toggleValue('minimize_to_tray', !tray)" />
      </div>
      <div class="row">
        <div class="row-tx">
          <b>{{ t('settings.behavior.autostart') }}</b>
        </div>
        <el-switch :model-value="autoStart" @change="toggleAutoStart" />
      </div>
    </SettingsSection>

    <SettingsSection icon="brain" :title="t('settings.behavior.thinking')" :desc="t('settings.behavior.thinkingHint')">
      <template #actions>
        <el-button size="small" type="primary" :loading="busy === 'thinking'" :disabled="!!busy" @click="saveThinking">
          {{ t('common.save') }}
        </el-button>
      </template>
      <el-select v-model="level" class="sel">
        <el-option value="" :label="t('settings.behavior.thinkingDefault')" />
        <el-option v-for="l in LEVELS" :key="l" :value="l" :label="t(`thinking.${l}`)" />
      </el-select>
    </SettingsSection>

    <SettingsSection icon="arrow-down-up" :title="t('settings.behavior.compaction')">
      <template #actions>
        <el-button size="small" type="primary" :loading="busy === 'compaction'" :disabled="!!busy" @click="saveCompaction">
          {{ t('common.save') }}
        </el-button>
      </template>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div class="field">
          <label class="wb-label">{{ t('settings.behavior.compactionReserve') }}</label>
          <el-input-number v-model="reserve" :min="0" :step="1000" controls-position="right" />
          <p class="hint">{{ t('settings.behavior.compactionReserveHint') }}</p>
        </div>
        <div class="field">
          <label class="wb-label">{{ t('settings.behavior.compactionKeep') }}</label>
          <el-input-number v-model="keep" :min="0" :step="1000" controls-position="right" />
          <p class="hint">{{ t('settings.behavior.compactionKeepHint') }}</p>
        </div>
      </div>
    </SettingsSection>

    <SettingsSection icon="braces" :title="t('settings.behavior.piPaths')" :desc="t('settings.behavior.piPathsHint')">
      <div class="paths">
        <button
          v-for="p in [
            { key: 'agentDir', value: paths?.agent_dir },
            { key: 'settingsPath', value: paths?.settings_path },
            { key: 'modelsPath', value: paths?.models_path },
            { key: 'mcpPath', value: paths?.mcp_path },
            { key: 'sessionsDir', value: paths?.sessions_dir },
            { key: 'extensionsDir', value: paths?.extensions_dir },
            { key: 'skillsDir', value: paths?.skills_dir },
          ]"
          :key="p.key"
          class="path-row"
          type="button"
          :title="t('settings.behavior.copyPath')"
          @click="copyPath(p.value || '')"
        >
          <span class="path-k">{{ t(`settings.behavior.${p.key}`) }}</span>
          <span class="path-v wb-mono">{{ p.value || '—' }}</span>
          <AppIcon name="copy" size="ic-xs" class="path-copy" />
        </button>
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-2) 0;
}
.row-tx {
  flex: 1;
  min-width: 0;
}
.row-tx b {
  display: block;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.row-tx p {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.path-line {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.sel {
  max-width: calc(220px * var(--wb-vw-scale));
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.paths {
  display: flex;
  flex-direction: column;
  gap: 1px;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.path-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  text-align: left;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.path-row:hover {
  background: var(--wb-surface-2);
}
.path-k {
  width: calc(120px * var(--wb-vw-scale));
  flex: none;
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-muted);
}
.path-v {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.path-copy {
  color: var(--wb-muted);
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.path-row:hover .path-copy {
  opacity: 1;
}
</style>
