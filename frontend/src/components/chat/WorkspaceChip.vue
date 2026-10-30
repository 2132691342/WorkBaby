<script setup lang="ts">
// 工作目录芯片：在输入框底栏直接切目录。
// 切换是双写——全局默认（KV）+ 重新读引导数据；只改一边会出现「点了没换地方」的错觉。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { pickFolder } from '../../api/shell'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const settings = useSettingsStore()
const toast = useToastStore()
const busy = ref(false)

const current = computed(() => settings.values['workspace'] || settings.boot?.boot_workspace || '')
const short = computed(() => {
  const p = current.value
  if (!p) return t('settings.behavior.workspace')
  const parts = p.split(/[\\/]/).filter(Boolean)
  return parts.length > 2 ? `…\\${parts.slice(-2).join('\\')}` : p
})

async function pick() {
  if (busy.value) return
  busy.value = true
  try {
    const dir = await pickFolder(t('settings.behavior.workspacePick'))
    if (!dir || dir === current.value) return
    await settings.setValue('workspace', dir)
    await settings.loadBoot()
    toast.ok(dir)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <el-tooltip :content="current || t('settings.behavior.workspaceHint')" placement="top" :show-after="250">
    <button
      class="ws-chip"
      type="button"
      :class="{ 'is-default': !current, 'is-busy': busy }"
      :disabled="busy"
      @click="pick"
    >
      <AppIcon name="folder" size="ic-2xs" />
      <span class="ws-t">{{ short }}</span>
      <AppIcon v-if="busy" name="loader" size="ic-2xs" spin />
    </button>
  </el-tooltip>
</template>

<style scoped>
.ws-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-h-sm);
  max-width: calc(260px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  transition:
    background var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ws-chip:hover:not(:disabled) {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border-strong);
  color: var(--wb-ink-2);
  transform: scale(1.06);
}
.ws-chip:active:not(:disabled) {
  transform: scale(0.92);
}
.ws-chip.is-default {
  border-style: dashed;
}
.ws-chip:disabled {
  opacity: 0.6;
  cursor: progress;
}
.ws-t {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
