<script setup lang="ts">
// 启动行为：开机自启、关闭动作、恢复上次会话、数据目录。
// 前三项以系统为准：「设置了但没生效」只能系统说了算，
// 界面显示读回来的事实，不是我们记的那一份。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { usePrefsStore } from '@/stores/prefs'

const props = defineProps<{
  autoStartBusy: boolean
  restoreBusy: boolean
}>()

const emit = defineEmits<{
  toggleAutoStart: [enabled: boolean]
  setCloseAction: [action: string]
  toggleRestore: [enabled: boolean]
  openDataDir: []
}>()

const { t } = useI18n()
const prefs = usePrefsStore()

/** 上次会话的标题优先；没有标题时退到文件名。 */
const lastSessionLabel = computed(() => {
  const p = prefs.prefs
  if (!p?.last_session_file) return t('settings.lastSessionNone')
  return p.last_session_title || p.last_session_file
})
</script>

<template>
  <section class="card-pad">
    <div class="wb-section-title">{{ t('settings.groups.behavior') }}</div>

    <div class="set-row">
      <div>
        <div class="set-label">{{ t('settings.autoStart') }}</div>
        <div class="set-hint">{{ t('settings.autoStartHint') }}</div>
      </div>
      <el-switch
        :model-value="prefs.prefs?.auto_start ?? false"
        :loading="props.autoStartBusy"
        :disabled="props.autoStartBusy || !prefs.prefs"
        @change="(v: string | number | boolean) => emit('toggleAutoStart', Boolean(v))"
      />
    </div>

    <div class="set-row">
      <div>
        <div class="set-label">{{ t('settings.closeAction') }}</div>
        <div class="set-hint">{{ t('settings.closeActionHint') }}</div>
      </div>
      <el-radio-group
        :model-value="prefs.prefs?.close_action ?? 'tray'"
        :disabled="!prefs.prefs"
        @change="(v: string | number | boolean | undefined) => emit('setCloseAction', String(v))"
      >
        <el-radio-button value="tray">{{ t('settings.closeToTray') }}</el-radio-button>
        <el-radio-button value="quit">{{ t('settings.closeToQuit') }}</el-radio-button>
      </el-radio-group>
    </div>

    <div class="set-row">
      <div>
        <div class="set-label">{{ t('settings.restoreSession') }}</div>
        <div class="set-hint">{{ t('settings.restoreSessionHint') }}</div>
      </div>
      <el-switch
        :model-value="prefs.prefs?.restore_session ?? true"
        :loading="props.restoreBusy"
        :disabled="props.restoreBusy || !prefs.prefs"
        @change="(v: string | number | boolean) => emit('toggleRestore', Boolean(v))"
      />
    </div>

    <div class="set-row">
      <div class="min-w-0">
        <div class="set-label">{{ t('settings.lastSession') }}</div>
        <div class="set-hint">{{ lastSessionLabel }}</div>
      </div>
    </div>

    <div class="set-row">
      <div>
        <div class="set-label">{{ t('settings.dataDir') }}</div>
        <div class="set-hint mono selectable">{{ prefs.prefs?.data_dir || '—' }}</div>
        <div class="set-hint">{{ t('settings.dataDirHint') }}</div>
      </div>
      <button class="wb-btn sm" @click="emit('openDataDir')">
        <AppIcon name="folder" :size="13" />
        {{ t('settings.openDataDir') }}
      </button>
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
