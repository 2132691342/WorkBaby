<script setup lang="ts">
// 插件页：列、装、卸、启停、升级。
// 列表来自引擎自己的登记清单（pi 的 settings.json packages），不扫 node_modules——
// 那会把间接依赖当成插件。装 / 卸 / 升级都用内置 npm 跑进沙箱。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import PluginTable from '@/components/plugins/PluginTable.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useRefreshStore } from '@/stores/refresh'
import { useToasts } from '@/composables/useToasts'
import type { PluginRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const rows = ref<PluginRow[]>([])
const registry = ref('')
const installRoot = ref('')
const builtinTotal = ref(0)
const builtinInstalled = ref(0)
const busy = ref(false)
const loading = ref(false)
const busyName = ref('')
const source = ref('')
const installing = ref(false)
const retrying = ref(false)
const upgradingAll = ref(false)
const restarting = ref(false)
const checking = ref(false)

const caps = computed(() => app.caps)
const builtinMissing = computed(() => builtinTotal.value - builtinInstalled.value)
const upgradableCount = computed(() => rows.value.filter((row) => row.plugin_upgradable).length)

async function load() {
  loading.value = true
  try {
    const data = await api.plugins()
    rows.value = data.plugins ?? []
    registry.value = data.registry_path ?? ''
    installRoot.value = data.install_root ?? ''
    builtinTotal.value = data.builtin_total
    builtinInstalled.value = data.builtin_installed
    busy.value = data.busy
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

// 从输入的来源猜包名，用于「已经装过」的提醒。带版本的来源截掉 @version，
// 带 ref 的 git 源猜不准就整个当名字，反正匹配不上只是少提醒一次。
function pluginNameOf(source: string): string {
  const spec = source.startsWith('npm:') ? source.slice(4) : source
  const at = spec.lastIndexOf('@')
  return at > 1 ? spec.slice(0, at) : spec
}

async function install() {
  const target = source.value.trim()
  if (!target) return
  // 撞上已装的同名插件要先说明后果：覆盖成新版本，数据不动。
  const name = pluginNameOf(target)
  const existing = rows.value.find((row) => row.plugin_name === name)
  if (existing) {
    try {
      await ElMessageBox.confirm(
        t('plugins.installExisting', { name, version: existing.plugin_version }),
        t('plugins.installTitle'),
        { confirmButtonText: t('common.install'), cancelButtonText: t('common.cancel'), type: 'info' },
      )
    } catch {
      return
    }
  }
  installing.value = true
  try {
    await api.installPlugin(target)
    source.value = ''
    ElMessage.success(t('plugins.installed'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    installing.value = false
  }
}

async function retryAll() {
  retrying.value = true
  try {
    const res = await api.retryAllPlugins()
    ElMessage.success(t('plugins.retryDone', { ok: res.installed, fail: res.failed }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    retrying.value = false
  }
}

async function upgradeAll() {
  upgradingAll.value = true
  try {
    const res = await api.upgradeAllPlugins()
    ElMessage.success(t('plugins.upgradeAllDone', { ok: res.installed, fail: res.failed }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    upgradingAll.value = false
  }
}

// 问 npm registry 的最新版：结果直接并进清单（plugin_latest），离线时留空不猜。
async function checkUpdates() {
  if (checking.value) return
  checking.value = true
  try {
    const data = await api.checkPluginUpdates()
    rows.value = data.plugins ?? []
    builtinInstalled.value = data.builtin_installed
    const upgradable = (data.plugins ?? []).filter((row) => row.plugin_upgradable).length
    ElMessage.success(t('plugins.checkUpdatesDone', { n: upgradable }))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    checking.value = false
  }
}

async function uninstall(row: PluginRow) {
  try {
    await ElMessageBox.confirm(t('plugins.uninstallConfirm', { name: row.plugin_name }), t('plugins.uninstallTitle'), {
      confirmButtonText: t('common.uninstall'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  busyName.value = row.plugin_name
  try {
    await api.uninstallPlugin(row.plugin_name)
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

// 启停只改 settings.json 的 packages 条目，磁盘不动，秒级生效。
const toggling = ref('')

async function toggle(row: PluginRow, enabled: boolean) {
  if (row.plugin_enabled === enabled) return
  toggling.value = row.plugin_name
  const prev = row.plugin_enabled
  row.plugin_enabled = enabled
  try {
    await api.togglePlugin(row.plugin_name, enabled)
  } catch (err) {
    row.plugin_enabled = prev
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    toggling.value = ''
  }
}

// 内置项缺了给单个重试；已装项给升级。
const upgrading = ref('')

async function upgrade(row: PluginRow) {
  busyName.value = row.plugin_name
  try {
    await api.upgradePlugin(row.plugin_name)
    ElMessage.success(t('plugins.upgradeDone'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function retry(row: PluginRow) {
  upgrading.value = row.plugin_name
  try {
    await api.retryPlugin(row.plugin_name)
    ElMessage.success(t('plugins.installed'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    upgrading.value = ''
  }
}

async function restart() {
  restarting.value = true
  try {
    await api.restartEngine()
    await app.refreshEngines()
    ElMessage.success(t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    restarting.value = false
  }
}

// 首启装配在后台跑完时页面可能开着没动：事件路由 bump 一下，这里防抖重拉。
// 自己动作后的 load 不经过这里，不受影响。
const refreshStore = useRefreshStore()
let reloadTimer = 0
watch(
  () => refreshStore.plugins,
  () => {
    window.clearTimeout(reloadTimer)
    reloadTimer = window.setTimeout(() => void load(), 400)
  },
)

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('plugins.title') }}</h1>
        <p class="page-sub">{{ t('plugins.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <button
          v-if="upgradableCount > 0"
          class="wb-btn"
          :class="{ 'is-loading': upgradingAll }"
          :disabled="upgradingAll || busy"
          @click="upgradeAll"
        >
          <AppIcon name="download" :size="14" />
          {{ t('plugins.upgradeAll', { n: upgradableCount }) }}
        </button>
        <button
          v-if="builtinMissing > 0"
          class="wb-btn"
          :class="{ 'is-loading': retrying }"
          :disabled="retrying || busy"
          @click="retryAll"
        >
          <AppIcon name="download" :size="14" />
          {{ t('plugins.retryAll', { n: builtinMissing }) }}
        </button>
        <button class="wb-btn" :class="{ 'is-loading': checking }" :disabled="checking || busy" @click="checkUpdates">
          <AppIcon name="search" :size="14" />
          {{ t('plugins.checkUpdates') }}
        </button>
        <button class="wb-btn" :class="{ 'is-loading': restarting }" :disabled="restarting" @click="restart">
          <AppIcon name="refresh" :size="14" />
          {{ t('plugins.restart') }}
        </button>
        <button class="wb-btn icon ghost" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load">
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div v-if="!caps?.extensions" class="wb-empty">
      <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="plug" :size="22" /></span>
      <p class="wb-empty-title">{{ t('plugins.unsupported') }}</p>
      <p class="wb-empty-hint">{{ t('plugins.unsupportedHint') }}</p>
    </div>

    <div v-else class="page-body">
      <section class="wb-card card-pad install-card">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('plugins.installTitle') }}</span>
        </div>
        <div class="install-row">
          <el-input
            v-model="source"
            :placeholder="t('plugins.installPlaceholder')"
            :disabled="installing"
            size="small"
            @keyup.enter="install"
          />
          <button
            class="wb-btn primary"
            :class="{ 'is-loading': installing }"
            :disabled="installing || !source.trim()"
            @click="install"
          >
            <AppIcon name="download" :size="14" />
            {{ t('common.install') }}
          </button>
        </div>
        <p class="hint">{{ t('plugins.restartHint') }}</p>
      </section>

      <section>
        <div class="wb-sec-head">
          <span class="wb-sec-title">
            {{ t('nav.plugins') }} <span class="count">{{ rows.length }}</span>
            <span class="count">· {{ builtinInstalled }}/{{ builtinTotal }}</span>
          </span>
          <span v-if="registry" class="wb-mono registry" :title="installRoot">{{ registry }}</span>
        </div>

        <div v-if="!rows.length && !loading" class="wb-empty">
          <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="plug" :size="22" /></span>
          <p class="wb-empty-title">{{ t('plugins.emptyTitle') }}</p>
          <p class="wb-empty-hint">{{ t('plugins.emptyHint') }}</p>
        </div>

        <PluginTable
          v-if="rows.length || loading"
          :rows="rows"
          :loading="loading"
          :busy="busy"
          :busy-name="busyName"
          :toggling="toggling"
          :upgrading="upgrading"
          @toggle="toggle"
          @upgrade="upgrade"
          @retry="retry"
          @uninstall="uninstall"
        />
      </section>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 650;
  color: var(--wb-ink);
}

.page-sub {
  margin: 4px 0 0;
  max-width: 620px;
  font-size: var(--wb-fs-sm);
  line-height: 1.65;
  color: var(--wb-mute);
}

.head-actions {
  display: flex;
  gap: var(--wb-sp-2);
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-12);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

.install-card {
  max-width: 720px;
}

.install-row {
  display: flex;
  gap: var(--wb-sp-2);
}

.hint {
  margin: var(--wb-sp-2) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
  line-height: var(--wb-lh-base);
}

.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}

.registry {
  margin-left: auto;
  max-width: 340px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
