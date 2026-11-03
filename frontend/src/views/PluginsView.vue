<script setup lang="ts">
// 插件页：列、装、卸。
// 列表来自引擎自己的登记清单（pi 的 packages / omp 的 dependencies），
// 不扫 node_modules——那会把间接依赖当成插件。装 / 卸代跑引擎自己的 CLI。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import EngineScopeBar from '@/components/common/EngineScopeBar.vue'
import { useEngineScope } from '@/composables/useEngineScope'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { EngineKind, PluginRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const rows = ref<PluginRow[]>([])
const registry = ref('')
const readError = ref('')
const loading = ref(false)
const busyName = ref('')
const source = ref('')
const installing = ref(false)
const output = ref('')
const restarting = ref(false)

const kind = computed<EngineKind>(() => app.active)
// 本页作用域（omp / pi / 全部），不跟对话引擎联动。
const { scope, set: setScope } = useEngineScope('plugins')
const installedKinds = computed(() => app.installed.map((e) => e.kind))
const writeKind = computed<EngineKind>(() => (scope.value === 'all' ? 'omp' : scope.value))
const caps = computed(() => app.capsOf(writeKind.value))
const engineParam = computed<EngineKind | undefined>(() => (scope.value === 'all' ? undefined : scope.value))

/** 一行属于哪个引擎：「全部」下靠行自身的 engine_kind 定向写操作。 */
function rowEngine(row: PluginRow): EngineKind {
  return (row.engine_kind || writeKind.value) as EngineKind
}

async function load() {
  loading.value = true
  try {
    const data = await api.plugins(engineParam.value)
    rows.value = data.plugins ?? []
    registry.value = data.registry_path ?? ''
    readError.value = data.read_error ?? ''
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function install() {
  const target = source.value.trim()
  if (!target) return
  installing.value = true
  output.value = ''
  try {
    const res = await api.installPlugin(target, writeKind.value)
    output.value = res.output
    source.value = ''
    ElMessage.success(t('plugins.installed'))
    await load()
  } catch (err) {
    output.value = err instanceof Error ? err.message : String(err)
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    installing.value = false
  }
}

// 卸的是清单里那一行原文而不是裸包名：pi 的 packages 记的是
// `npm:name@1.2.3`，只给裸名会匹配不到。
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
  output.value = ''
  try {
    const res = await api.uninstallPlugin(row.plugin_source, rowEngine(row))
    output.value = res.output
    await load()
  } catch (err) {
    output.value = err instanceof Error ? err.message : String(err)
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

// 启停是 omp 的 CLI 能力（改它的锁文件），pi 行不出开关。
// 乐观更新让开关立即回弹到位，失败滚回来；成功后重拉清单拿真实状态。
const toggling = ref('')

function canToggle(row: PluginRow): boolean {
  return Boolean(app.capsOf(rowEngine(row))?.plugin_toggle)
}

function canUpgrade(row: PluginRow): boolean {
  return Boolean(app.capsOf(rowEngine(row))?.plugin_upgrade)
}

async function toggle(row: PluginRow, enabled: boolean) {
  if (row.enabled === enabled) return
  toggling.value = row.plugin_name
  const prev = row.enabled
  row.enabled = enabled
  try {
    await api.togglePlugin(row.plugin_name, enabled, rowEngine(row))
    ElMessage.success(t('plugins.toggleDone'))
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
  output.value = ''
  try {
    const res = await api.upgradePlugin(row.plugin_source, rowEngine(row))
    output.value = res.output
    ElMessage.success(t('plugins.upgradeDone'))
    await load()
  } catch (err) {
    output.value = err instanceof Error ? err.message : String(err)
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyName.value = ''
  }
}

async function restart() {
  restarting.value = true
  try {
    await api.restartEngines(writeKind.value)
    await app.refreshEngines()
    ElMessage.success(t('common.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    restarting.value = false
  }
}

function resourceLabel(kinds: string[]): string {
  if (!kinds?.length) return '—'
  return kinds.join(' · ')
}

watch(kind, () => {
  rows.value = []
  output.value = ''
  void load()
})

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('plugins.title') }}</h1>
        <p class="page-sub">{{ t('plugins.scopedSubtitle') }}</p>
      </div>
      <div class="head-actions">
        <EngineScopeBar :scope="scope" :installed="installedKinds" @pick="setScope" />
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
      <p class="wb-empty-hint">{{ t('plugins.unsupportedHint', { engine: scope }) }}</p>
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

      <p v-if="readError" class="notice">
        <AppIcon name="alert" :size="14" />
        {{ readError }}
      </p>

      <section v-if="output" class="wb-card card-pad">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('plugins.output') }}</span>
        </div>
        <pre class="output selectable">{{ output }}</pre>
      </section>

      <section>
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('nav.plugins') }} <span class="count">{{ rows.length }}</span></span>
          <span v-if="registry" class="wb-mono registry">{{ registry }}</span>
        </div>

        <div v-if="!rows.length && !loading" class="wb-empty">
          <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="plug" :size="22" /></span>
          <p class="wb-empty-title">{{ t('plugins.emptyTitle') }}</p>
          <p class="wb-empty-hint">{{ t('plugins.emptyHint') }}</p>
        </div>

        <el-table v-else :data="rows" size="small">
          <el-table-column :label="t('plugins.engine')" width="72">
            <template #default="{ row }">
              <span class="wb-mono">{{ row.engine_kind }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('plugins.name')" min-width="180">
            <template #default="{ row }">
              <div class="cell-name">
                <span class="name">{{ row.plugin_name }}</span>
                <span v-if="row.plugin_desc" class="desc">{{ row.plugin_desc }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('plugins.version')" width="92">
            <template #default="{ row }">
              <span class="wb-mono">{{ row.plugin_version || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('plugins.resources')" width="120">
            <template #default="{ row }">
              <span class="wb-mono">{{ resourceLabel(row.resource_kinds) }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('plugins.state')" width="96">
            <template #default="{ row }">
              <span class="tag" :class="row.installed ? 'ok' : 'warn'">
                {{ row.installed ? t('plugins.installedState') : t('plugins.missingState') }}
              </span>
            </template>
          </el-table-column>
          <!-- 启用列只对有启停能力的引擎行出现：能力缺失不渲染，不渲染灰开关 -->
          <el-table-column v-if="rows.some((r) => canToggle(r))" :label="t('plugins.enabledLabel')" width="80">
            <template #default="{ row }">
              <el-switch
                v-if="canToggle(row)"
                :model-value="row.enabled"
                :loading="toggling === row.plugin_name"
                :disabled="(Boolean(toggling) && toggling !== row.plugin_name) || !row.installed"
                @change="(v: string | number | boolean) => toggle(row, Boolean(v))"
              />
              <span v-else class="wb-mono">—</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('plugins.path')" min-width="180">
            <template #default="{ row }">
              <span class="wb-mono clip">{{ row.plugin_path || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.actions')" width="150" align="right">
            <template #default="{ row }">
              <div class="row-actions">
                <button
                  v-if="canUpgrade(row)"
                  class="wb-btn text sm"
                  :class="{ 'is-loading': busyName === row.plugin_name }"
                  :disabled="Boolean(busyName)"
                  @click="upgrade(row)"
                >
                  {{ t('plugins.upgrade') }}
                </button>
                <button
                  class="wb-btn text sm danger"
                  :class="{ 'is-loading': busyName === row.plugin_name }"
                  :disabled="Boolean(busyName)"
                  @click="uninstall(row)"
                >
                  {{ t('common.uninstall') }}
                </button>
              </div>
            </template>
          </el-table-column>
        </el-table>
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

.output {
  margin: 0;
  max-height: 240px;
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-r);
  background: var(--wb-code-bg);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  line-height: 1.6;
  color: var(--wb-ink-5);
  white-space: pre-wrap;
}

.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}

.registry {
  margin-left: auto;
  max-width: 340px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-name {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.name {
  font-weight: 650;
  color: var(--wb-ink);
}

.desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.clip {
  display: block;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
