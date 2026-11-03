<script setup lang="ts">
// 内置服务商预设选择器：首启没配服务商时的主入口。
// 抽成独立组件有两个理由——模型服务面板已经顶到 500 行，
// 且这段的失败态（拉不到清单 / 已添加过）与面板其余部分的交互无关。
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ProviderPresetVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import { errorText } from '../../utils/errors'

const emit = defineEmits<{ added: [] }>()
const { t } = useI18n()
const store = useSettingsStore()
const toast = useToastStore()

const presets = ref<ProviderPresetVO[]>([])
const apiKey = ref('')
const busy = ref('')

/** 拉预设清单；失败静默——预设只是快捷方式，拉不到还能手写配置。 */
async function load() {
  try {
    presets.value = await api.providers.presets()
  } catch {
    presets.value = []
  }
}

/** 添加一个预设：密钥可选；成功后刷新引导快照并把默认指向它的第一个模型。 */
async function apply(p: ProviderPresetVO) {
  if (busy.value) return
  busy.value = p.preset_id
  try {
    await api.providers.applyPreset(p.preset_id, apiKey.value.trim())
    apiKey.value = ''
    await store.refreshBoot()
    await store.loadProviders()
    await load()
    toast.ok(t('settings.providers.presetAdded', { name: p.preset_name }))
    emit('added')
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <!-- 全部预设都添加过时整块消失：没有可点的东西就不该占一块位置 -->
  <div v-if="presets.some((p) => !p.preset_installed)" class="preset-box">
    <div class="preset-head">
      <b>{{ t('settings.providers.presets') }}</b>
      <p>{{ t('settings.providers.presetsHint') }}</p>
    </div>
    <el-input
      v-model="apiKey"
      size="small"
      clearable
      show-password
      :placeholder="t('settings.providers.presetKeyPlaceholder')"
      class="preset-key"
    >
      <template #prefix><AppIcon name="key" size="ic-xs" /></template>
    </el-input>
    <div class="preset-grid">
      <button
        v-for="p in presets.filter((x) => !x.preset_installed)"
        :key="p.preset_id"
        type="button"
        class="preset wb-press"
        :disabled="!!busy"
        :class="{ 'is-loading': busy === p.preset_id }"
        :title="p.preset_hint"
        @click="apply(p)"
      >
        <span class="preset-name">{{ p.preset_name }}</span>
        <span class="preset-hint">{{ p.preset_hint }}</span>
        <span class="preset-meta">{{ p.preset_base_url }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.preset-box {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-lg);
  border: 1px dashed var(--wb-border-strong);
  background: var(--wb-elev-surface);
}
.preset-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.preset-head p {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.preset-key {
  max-width: calc(420px * var(--wb-vw-scale));
}
.preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(calc(200px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-2);
}
.preset {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  text-align: left;
  border-radius: var(--wb-radius-md);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  cursor: pointer;
}
.preset:hover:not(:disabled) {
  border-color: var(--wb-brand-2);
  background: var(--wb-brand-3);
}
.preset:disabled {
  cursor: default;
  opacity: 0.6;
}
.preset-name {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.preset-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: 1.35;
}
.preset-meta {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
  overflow-wrap: anywhere;
}
</style>
