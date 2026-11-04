<script setup lang="ts">
// 插件页：内置清单 + 用户自加的包。清单来自 pi 的 settings.json packages 声明，
// 装包经内置 npm 落进沙箱（agentDir/npm），启停只改声明——磁盘不动，秒级生效。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import PluginTable from '@/components/plugins/PluginTable.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { PluginRow } from '@/types/api'

type GroupKind = 'all' | 'builtin' | 'custom'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const rows = ref<PluginRow[]>([])
const registry = ref('')
const installRoot = ref('')
const builtinInstalled = ref(0)
const builtinTotal = ref(0)
const readError = ref('')
const loading = ref(false)
const busyName = ref('')
const source = ref('')
const installing = ref(false)
const retryingAll = ref(false)
const toggling = ref('')
const group = ref<GroupKind>('all')
const keyword = ref('')
/** 正在安装的那一行的实时进度行。 */
const liveLines = ref<Record<string, string>>({})
/** 整体重试进度；total=0 时不显示。 */
const queue = ref({ done: 0, total: 0 })

const caps = computed(() => app.capsOf('pi'))
const missingCount = computed(() => rows.value.filter((r) => !r.installed).length)
const failedCount = computed(() => rows.value.filter((r) => r.plugin_state === 'failed').length)
const builtinPercent = computed(() =>
  builtinTotal.value === 0 ? 0 : Math.round((builtinInstalled.value / builtinTotal.value) * 100),
)

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return rows.value.filter((row) => {
    if (group.value === 'builtin' && !row.builtin) return false
    if (group.value === 'custom' && row.builtin) return false
    if (!kw) return true
    return row.plugin_name.toLowerCase().includes(kw) || (row.plugin_desc ?? '').toLowerCase().includes(kw)
  })
})

async function load() {
  loading.value = true
  try {
    const data = await api.plugins('pi')
    rows.value = data.plugins ?? []
    registry.value = data.registry_path ?? ''
    installRoot.value = data.install_root ?? ''
    builtinInstalled.value = data.builtin_installed ?? 0
    builtinTotal.value = data.builtin_total ?? 0
    readError.value = data.read_error ?? ''
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

/** 安装进度帧：逐行显示 npm 正在干什么。 */
function onProgress(data: Record<string, unknown>) {
  const name = String(data.plugin_name ?? '')
  if (!name) return
  const phase = String(data.phase ?? '')
  if (phase === 'start') {
    liveLines.value = { ...liveLines.value, [name]: String(data.message ?? t('plugins.installing')) }
    return
  }
  if (phase === 'line') {
    liveLines.value = { ...liveLines.value, [name]: String(data.line ?? '') }
    return
  }
  const next = { ...liveLines.value }
  delete next[name]
  liveLines.value = next
  if (phase === 'done' || phase === 'fail') void load()
}

let offStatus: (() => void) | null = null
let offProgress: (() => void) | null = null

async function install() {
  const target = source.value.trim()
  if (!target) return
  installing.value = true
  try {
    const res = await api.installPlugin(target, 'pi')
    source.value = ''
    ElMessage.success(t('plugins.installed', { name: res.plugin_name }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    installing.value = false
  }
}

async function retryAll() {
  retryingAll.value = true
  queue.value = { done: 0, total: missingCount.value }
  try {
    const res = await api.retryAllPlugins('pi')
    if (res.failed > 0) toasts.push('warn', t('plugins.retryPartial', { ok: res.installed, fail: res.failed }))
    else ElMessage.success(t('plugins.retryAllDone', { ok: res.installed }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    retryingAll.value = false
    queue.value = { done: 0, total: 0 }
  }
}

async function retryOne(row: PluginRow) {
  busyName.value = row.plugin_name
  try {
    await api.retryPlugin(row.plugin_name, 'pi')
    ElMessage.success(t('plugins.installed', { name: row.plugin_name }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function uninstall(row: PluginRow) {
  try {
    await ElMessageBox.confirm(
      t('plugins.uninstallConfirm', { name: row.plugin_name }),
      t('plugins.uninstallTitle'),
      { confirmButtonText: t('common.uninstall'), cancelButtonText: t('common.cancel'), type: 'warning' },
    )
  } catch {
    return
  }
  busyName.value = row.plugin_name
  try {
    await api.uninstallPlugin(row.plugin_name, 'pi')
    ElMessage.success(t('plugins.uninstalled', { name: row.plugin_name }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

/** 乐观更新让开关立即回弹到位，失败滚回来；成功后重拉清单拿真实状态。 */
async function toggle(row: PluginRow, enabled: boolean) {
  if (row.enabled === enabled) return
  toggling.value = row.plugin_name
  const prev = row.enabled
  row.enabled = enabled
  try {
    await api.togglePlugin(row.plugin_name, enabled, 'pi')
    await load()
  } catch (err) {
    row.enabled = prev
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    toggling.value = ''
  }
}

async function upgrade(row: PluginRow) {
  busyName.value = row.plugin_name
  try {
    await api.upgradePlugin(row.plugin_name, 'pi')
    ElMessage.success(t('plugins.upgradeDone', { name: row.plugin_name }))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

onMounted(async () => {
  // 资源状态帧走全壳唯一的 SSE，由 app store 统一分发。
  offStatus = app.onStatus(() => void load())
  offProgress = app.onProgress(onProgress)
  await load()
})

onUnmounted(() => {
  offStatus?.()
  offProgress?.()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('plugins.title') }}</h1>
        <p class="page-sub">{{ t('plugins.sandboxSubtitle') }}</p>
      </div>
      <div class="head-actions">
        <button
          v-if="missingCount > 0 || failedCount > 0"
          class="wb-btn"
          :class="{ 'is-loading': retryingAll }"
          :disabled="retryingAll || !caps?.extensions"
          @click="retryAll"
        >
          <AppIcon name="download" :size="14" />
          {{ t('plugins.retryAll') }}
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
      <!-- 内置清单进度：只在还没全装好时出现，装完就消失 -->
      <section v-if="builtinInstalled < builtinTotal" class="wb-card card-pad progress-card">
        <div class="progress-head">
          <span class="progress-title">{{ t('plugins.builtinProgress', { done: builtinInstalled, total: builtinTotal }) }}</span>
          <span class="wb-mono progress-pct">{{ builtinPercent }}%</span>
        </div>
        <div class="bar"><i :style="{ width: `${builtinPercent}%` }" /></div>
        <p class="hint">{{ t('plugins.builtinProgressHint') }}</p>
      </section>

      <section v-if="queue.total > 0" class="wb-card card-pad progress-card">
        <div class="progress-head">
          <span class="progress-title">{{ t('plugins.queueProgress', { done: queue.done, total: queue.total }) }}</span>
        </div>
        <div class="bar"><i class="indeterminate" /></div>
      </section>

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
        <p class="hint">{{ t('plugins.installHint', { root: installRoot }) }}</p>
      </section>

      <p v-if="readError" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ readError }}
      </p>

      <section>
        <div class="wb-sec-head">
          <span class="wb-sec-title">
            {{ t('nav.plugins') }} <span class="count">{{ filtered.length }}</span>
          </span>
          <div class="toolbar">
            <div class="seg">
              <button
                v-for="opt in (['all', 'builtin', 'custom'] as GroupKind[])"
                :key="opt"
                class="seg-btn"
                :class="{ on: group === opt }"
                @click="group = opt"
              >
                {{ t(`plugins.group_${opt}`) }}
              </button>
            </div>
            <el-input v-model="keyword" :placeholder="t('common.search')" size="small" clearable class="search" />
            <span v-if="registry" class="wb-mono registry" :title="registry">{{ registry }}</span>
          </div>
        </div>

        <PluginTable
          :rows="filtered"
          :loading="loading"
          :busy-name="busyName"
          :toggling="toggling"
          :retrying-all="retryingAll"
          :live-lines="liveLines"
          :keyword="keyword"
          @toggle="toggle"
          @upgrade="upgrade"
          @uninstall="uninstall"
          @retry="retryOne"
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
  max-width: var(--wb-w-desc);
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
  word-break: break-all;
}

.progress-card {
  max-width: 720px;
}

.progress-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--wb-sp-2);
}

.progress-title {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}

.progress-pct {
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.bar {
  height: 6px;
  border-radius: 999px;
  background: var(--wb-elev-hover);
  overflow: hidden;
}

.bar > i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--wb-brand-1);
  transition: width var(--wb-dur-slow) var(--wb-ease);
}

.bar > i.indeterminate {
  width: 36%;
  animation: slide 1.2s var(--wb-ease) infinite;
}

@keyframes slide {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(340%);
  }
}

.notice {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin: 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-warn-soft);
  color: var(--wb-warn);
  font-size: var(--wb-fs-xs);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-left: auto;
}

.seg {
  display: inline-flex;
  padding: 2px;
  border-radius: var(--wb-r);
  background: var(--wb-elev-hover);
}

.seg-btn {
  border: none;
  background: transparent;
  padding: 3px var(--wb-sp-3);
  border-radius: calc(var(--wb-r) - 2px);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  cursor: pointer;
  transition: background var(--wb-dur-fast) var(--wb-ease), color var(--wb-dur-fast) var(--wb-ease);
}

.seg-btn:hover {
  color: var(--wb-ink);
}

.seg-btn.on {
  background: var(--wb-elev-raised);
  color: var(--wb-brand-1);
  font-weight: 600;
}

.search {
  width: 160px;
}

.registry {
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}
</style>
