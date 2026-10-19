<script setup lang="ts">
// 行为面板：工作目录、回车与托盘、默认思考档、压缩参数，以及 PI 的配置文件位置。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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

// 数字框每点一下就存一次会连着写盘、连着弹提示：按一下步进按钮五次就是五条「已保存」。
// 合流成一次写，提示也只说一句。
let compactionTimer = 0
function queueCompaction() {
  if (compactionTimer) clearTimeout(compactionTimer)
  compactionTimer = window.setTimeout(() => {
    compactionTimer = 0
    void saveCompaction()
  }, 400)
}
onBeforeUnmount(() => {
  if (compactionTimer) clearTimeout(compactionTimer)
})

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
  <div class="panel-wrap wb-form">
    <SettingsSection icon="folder" :title="t('settings.behavior.workspace')" :desc="t('settings.behavior.workspaceHint')">
      <template #actions>
        <el-button size="small" @click="pickWorkspace">{{ t('settings.behavior.workspacePick') }}</el-button>
      </template>
      <p class="path-line" :title="workspace">{{ workspace || '—' }}</p>
    </SettingsSection>

    <SettingsSection icon="sliders" :title="t('settings.behavior.title')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.sendOnEnter') }}</b>
          <p>{{ t('settings.behavior.sendOnEnterHint') }}</p>
        </div>
        <el-switch :model-value="sendOnEnter" @change="toggleValue('send_on_enter', !sendOnEnter)" />
      </div>
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
        <el-switch :model-value="autoStart" @change="toggleAutoStart" />
      </div>
    </SettingsSection>

    <SettingsSection icon="brain" :title="t('settings.behavior.thinking')" :desc="t('settings.behavior.thinkingHint')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx"><b>{{ t('settings.behavior.thinkingLevel') }}</b></div>
        <el-select v-model="level" class="sel" :disabled="!!busy" @change="saveThinking">
          <el-option value="" :label="t('settings.behavior.thinkingDefault')" />
          <el-option v-for="l in LEVELS" :key="l" :value="l" :label="t(`thinking.${l}`)" />
        </el-select>
      </div>
    </SettingsSection>

    <SettingsSection icon="arrow-down-up" :title="t('settings.behavior.compaction')" divided>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.compactionReserve') }}</b>
          <p>{{ t('settings.behavior.compactionReserveHint') }}</p>
        </div>
        <el-input-number
          v-model="reserve"
          :min="0"
          :step="1000"
          controls-position="right"
          :disabled="!!busy"
          @change="queueCompaction"
        />
      </div>
      <div class="wb-set-row">
        <div class="wb-set-row-tx">
          <b>{{ t('settings.behavior.compactionKeep') }}</b>
          <p>{{ t('settings.behavior.compactionKeepHint') }}</p>
        </div>
        <el-input-number
          v-model="keep"
          :min="0"
          :step="1000"
          controls-position="right"
          :disabled="!!busy"
          @change="queueCompaction"
        />
      </div>
    </SettingsSection>

    <SettingsSection icon="braces" :title="t('settings.behavior.piPaths')" :desc="t('settings.behavior.piPathsHint')" divided>
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
/* 表单类面板收窄：控件列与标签列都靠左，面板宽 920px 时右侧会拖出一片空地 */
.panel-wrap {
  --wb-panel-w: calc(720px * var(--wb-vw-scale));
}
.sel {
  max-width: calc(220px * var(--wb-vw-scale));
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
  background: var(--wb-elev-hover);
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
