<script setup lang="ts">
// 插件面板：PI 的扩展（extensions 目录 + settings.json 登记）与包（pi install）。
// 装 / 卸都走 PI 自己的 CLI，进度经 chat:setup 事件流式回传。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { ExtensionListRESP, ExtensionVO, PackageVO } from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()
const settingsStore = useSettingsStore()

const data = ref<ExtensionListRESP | null>(null)
const busy = ref('')
const source = ref('')
const log = ref<string[]>([])

const entries = computed(() => data.value?.entries ?? [])
const packages = computed(() => data.value?.packages ?? [])

async function reload() {
  try {
    data.value = await api.extensions.list()
    settingsStore.extensions = data.value
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  }
}

async function toggle(e: ExtensionVO) {
  if (busy.value) return
  busy.value = `t:${e.path}`
  try {
    await api.extensions.toggle(e.path, !e.enabled)
    await reload()
  } catch (err) {
    toast.bad(t('common.saveFailed', { msg: (err as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function remove(e: ExtensionVO) {
  if (busy.value) return
  busy.value = `d:${e.path}`
  try {
    await api.extensions.remove(e.path)
    await reload()
    toast.ok(`${t('common.delete')} ${e.name}`)
  } catch (err) {
    toast.bad(t('common.saveFailed', { msg: (err as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function importExt() {
  if (busy.value) return
  busy.value = 'import'
  try {
    const path = await OpenFileDialog(t('settings.extensions.importHint'), '*.ts;*.js;*.mjs')
    if (!path) return
    const r = await api.extensions.import([path])
    await reload()
    if (r.imported) toast.ok(`${r.imported}`)
    if (r.skipped?.length) toast.info(r.skipped.join('; '))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function install() {
  const src = source.value.trim()
  if (!src || busy.value) return
  busy.value = 'install'
  log.value = []
  try {
    await api.extensions.install(src)
    toast.ok(t('settings.extensions.installing'))
    source.value = ''
    window.setTimeout(() => void reload(), 3000)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function uninstall(p: PackageVO) {
  if (busy.value) return
  busy.value = `u:${p.source}`
  log.value = []
  try {
    await api.extensions.uninstall(p.source)
    await reload()
    toast.ok(`${t('common.remove')} ${p.name}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function restart() {
  if (busy.value) return
  busy.value = 'reload'
  try {
    await api.extensions.reload()
    await reload()
    toast.ok(t('settings.extensions.reloaded'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

function onSetup(line: string) {
  log.value = [...log.value.slice(-200), line]
}

defineExpose({ onSetup })

onMounted(() => {
  if (settingsStore.extensions) data.value = settingsStore.extensions
  void reload()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection
      icon="braces"
      :title="t('settings.extensions.title')"
      :desc="t('settings.extensions.desc')"
    >
      <template #actions>
        <el-button size="small" :loading="busy === 'import'" :disabled="!!busy" @click="importExt">
          {{ t('settings.extensions.import') }}
        </el-button>
        <el-button size="small" :loading="busy === 'reload'" :disabled="!!busy" @click="restart">
          {{ t('settings.extensions.reload') }}
        </el-button>
      </template>

      <div class="ext-bar">
        <el-input
          v-model="source"
          size="small"
          class="ext-input"
          :placeholder="t('settings.extensions.installPlaceholder')"
          @keydown.enter="install"
        />
        <el-button size="small" type="primary" :loading="busy === 'install'" :disabled="!!busy" @click="install">
          {{ t('settings.extensions.install') }}
        </el-button>
      </div>

      <div v-if="!entries.length && !packages.length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="braces" size="ic-xl" /></span>
        <b>{{ t('settings.extensions.empty') }}</b>
        <p>{{ t('settings.extensions.emptyHint') }}</p>
      </div>

      <template v-else>
        <div v-if="entries.length" class="ext-group">
          <div class="wb-label">{{ t('settings.extensions.extensions') }}</div>
          <div class="ext-list">
            <article v-for="e in entries" :key="e.path" class="ext-row" :class="{ 'is-off': !e.enabled }">
              <span class="ext-tile"><AppIcon name="braces" size="ic-sm" /></span>
              <div class="ext-tx">
                <span class="ext-name">{{ e.name }}</span>
                <span class="wb-mono" :title="e.path">{{ e.path }}</span>
              </div>
              <el-switch
                :model-value="e.enabled"
                :loading="busy === `t:${e.path}`"
                :disabled="!!busy"
                @change="toggle(e)"
              />
              <el-popconfirm :title="t('settings.extensions.deleteTitle')" width="240" @confirm="remove(e)">
                <template #reference>
                  <button class="ext-del" type="button" :disabled="!!busy">
                    <AppIcon name="trash" size="ic-xs" />
                  </button>
                </template>
              </el-popconfirm>
            </article>
          </div>
        </div>

        <div v-if="packages.length" class="ext-group">
          <div class="wb-label">{{ t('settings.extensions.packages') }}</div>
          <div class="ext-list">
            <article v-for="p in packages" :key="p.source" class="ext-row">
              <span class="ext-tile"><AppIcon name="database" size="ic-sm" /></span>
              <div class="ext-tx">
                <span class="ext-name">
                  {{ p.name }}
                  <el-tag v-if="p.missing" size="small" type="warning">{{ t('common.unknown') }}</el-tag>
                </span>
                <span class="wb-mono" :title="p.path">{{ p.source }}</span>
              </div>
              <el-popconfirm :title="t('settings.extensions.removeTitle')" width="260" @confirm="uninstall(p)">
                <template #reference>
                  <el-button size="small" :loading="busy === `u:${p.source}`" :disabled="!!busy">
                    {{ t('settings.extensions.remove') }}
                  </el-button>
                </template>
              </el-popconfirm>
            </article>
          </div>
        </div>
      </template>

      <div v-if="log.length" class="ext-log">
        <div class="wb-label">{{ t('setup.installLog') }}</div>
        <pre class="log-body">{{ log.join('\n') }}</pre>
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
.ext-bar {
  display: flex;
  gap: var(--wb-sp-2);
}
.ext-input {
  max-width: calc(420px * var(--wb-vw-scale));
}
.ext-group {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.ext-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.ext-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
}
.ext-row.is-off {
  opacity: 0.6;
}
.ext-tile {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  flex: none;
}
.ext-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.ext-name {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.ext-tx .wb-mono {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ext-del {
  display: grid;
  place-items: center;
  width: calc(24px * var(--wb-vw-scale));
  height: calc(24px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
}
.ext-del:hover:not(:disabled) {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.ext-log {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.log-body {
  margin: 0;
  max-height: calc(220px * var(--wb-vw-scale));
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-code-bg);
  border: 1px solid var(--wb-border);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-ink-2);
  white-space: pre-wrap;
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(420px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
</style>
