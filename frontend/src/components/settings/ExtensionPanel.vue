<script setup lang="ts">
// 插件面板：扩展（目录 + settings.json 登记）与包（pi install）。装 / 卸 / 更新都走 PI 的 CLI，
// 结束信号来自 chat:setup 的 done 帧——靠定时器猜结束会提前弹「已更新」。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSetupLog } from '../../composables/useSetupLog'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type {
  ExtensionListRESP,
  ExtensionVO,
  MarketListRESP,
  MarketPackageVO,
  PackageLatestVO,
  PackageVO,
} from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()
const settingsStore = useSettingsStore()

const data = ref<ExtensionListRESP | null>(null)
const busy = ref('')
const source = ref('')
// 装 / 卸 / 更新的进度挂在 SSE 的全局通道上（发生在任何会话之外）
const { lines: log, begin: beginLog, end: endLog, whenDone } = useSetupLog()

/** 更新动作可能跑几分钟（npm 拉包），等真正的结束帧而不是定时器 */
const UPDATE_TIMEOUT = 5 * 60 * 1000

const entries = computed(() => data.value?.entries ?? [])
const packages = computed(() => data.value?.packages ?? [])

// 插件市场：按 PI 官方发现机制（npm 关键字 pi-package）搜包；首次切到市场页才拉清单。
const tab = ref<'installed' | 'market'>('installed')
const market = ref<MarketListRESP | null>(null)
const marketLoading = ref(false)
const marketFilter = ref('')
const marketPackages = computed(() => {
  const kw = marketFilter.value.trim().toLowerCase()
  const list = market.value?.packages ?? []
  if (!kw) return list
  return list.filter(
    (p) => p.name.toLowerCase().includes(kw) || (p.description || '').toLowerCase().includes(kw),
  )
})

watch(tab, (v) => {
  if (v === 'market' && !market.value && !marketLoading.value) void loadMarket()
})

async function loadMarket(refresh = false) {
  if (marketLoading.value) return
  marketLoading.value = true
  try {
    market.value = await api.extensions.market(refresh)
  } catch (e) {
    market.value = { packages: [], error: (e as Error)?.message || '' }
  } finally {
    marketLoading.value = false
  }
}

/** 市场卡片的发布者与时间一行读数；没有的段就省掉 */
function marketMeta(p: MarketPackageVO): string {
  const parts: string[] = []
  if (p.publisher) parts.push(p.publisher)
  if (p.published_at) parts.push(new Date(p.published_at).toLocaleDateString())
  return parts.join(' · ')
}

/** 从市场装一个包：与手填来源同一条链路，结束信号同样是 chat:setup 的 done 帧 */
async function installFromMarket(p: MarketPackageVO) {
  if (busy.value) return
  busy.value = `mkt:${p.name}`
  beginLog('package-install')
  const waiter = whenDone(UPDATE_TIMEOUT)
  try {
    await api.extensions.install(`npm:${p.name}`)
    const result = await waiter
    await reload()
    await loadMarket(true)
    if (!result.done) toast.info(t('settings.extensions.updateTimeout'))
    else if (result.ok) toast.ok(t('settings.extensions.installDone'))
    else toast.bad(t('settings.extensions.installFailed', { msg: await failureReason() }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
    endLog()
  }
}

// 版本对账：本地版本读自包目录，最新版本联网查（引擎页同一个接口）。
// 查不到不影响列表：只是不显示「可更新」。
const latestMap = ref<Record<string, PackageLatestVO>>({})
const checkingVer = ref(false)
const outdated = computed(() =>
  packages.value.filter((p) => latestMap.value[p.source]?.update_available),
)

async function checkVersions() {
  if (checkingVer.value) return
  checkingVer.value = true
  try {
    const r = await api.engine.latest()
    const map: Record<string, PackageLatestVO> = {}
    for (const item of r.packages ?? []) map[item.source] = item
    latestMap.value = map
  } catch {
    latestMap.value = {}
  } finally {
    checkingVer.value = false
  }
}

function versionOf(p: PackageVO): string {
  const latest = latestMap.value[p.source]?.latest_version
  if (p.version && latest && latest !== p.version) return `${p.version} → ${latest}`
  return p.version ? `v${p.version}` : ''
}

/** 更新前的版本快照：更新完用它对账，说「更新了几个」而不是无条件报成功 */
function snapshotVersions(): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of packages.value) out[p.source] = p.version || ''
  return out
}

function changedCount(before: Record<string, string>): number {
  let n = 0
  for (const p of packages.value) {
    if (before[p.source] !== undefined && before[p.source] !== (p.version || '')) n++
  }
  return n
}

/** 更新失败时后端把原因记在 install_error 里，比日志最后一行更准 */
async function failureReason(): Promise<string> {
  try {
    const st = await api.engine.status()
    if (st.install_error) return st.install_error
  } catch {
    // 拿不到状态就退回日志末行
  }
  return log.value[log.value.length - 1] || t('errors.generic')
}

/** 更新一个包或全部：等真正结束，再按版本对账说结果 */
async function updatePackage(source = '') {
  if (busy.value) return
  busy.value = source ? `up:${source}` : 'update-all'
  const before = snapshotVersions()
  beginLog('packages')
  const waiter = whenDone(UPDATE_TIMEOUT)
  try {
    await api.engine.updatePackages(source)
    const result = await waiter
    await reload()
    await checkVersions()
    if (!result.done) {
      toast.info(t('settings.extensions.updateTimeout'))
      return
    }
    if (!result.ok) {
      toast.bad(t('settings.extensions.updateFailed', { msg: await failureReason() }))
      return
    }
    const n = changedCount(before)
    if (n > 0) toast.ok(t('settings.extensions.updatedN', { n }))
    else toast.ok(t('settings.extensions.upToDate'))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    busy.value = ''
    endLog()
  }
}

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
    if (r.imported) toast.ok(t('common.imported', { n: r.imported }))
    if (r.skipped?.length) toast.info(t('common.skipped', { n: r.skipped.length }))
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
  beginLog('package-install')
  const waiter = whenDone(UPDATE_TIMEOUT)
  try {
    await api.extensions.install(src)
    toast.ok(t('settings.extensions.installing'))
    source.value = ''
    const result = await waiter
    await reload()
    await checkVersions()
    if (!result.done) toast.info(t('settings.extensions.updateTimeout'))
    else if (result.ok) toast.ok(t('settings.extensions.installDone'))
    else toast.bad(t('settings.extensions.installFailed', { msg: await failureReason() }))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
    endLog()
  }
}

async function uninstall(p: PackageVO) {
  if (busy.value) return
  busy.value = `u:${p.source}`
  beginLog('package-remove')
  const waiter = whenDone(UPDATE_TIMEOUT)
  try {
    await api.extensions.uninstall(p.source)
    const result = await waiter
    await reload()
    if (!result.done) toast.info(t('settings.extensions.updateTimeout'))
    else if (result.ok) toast.ok(`${t('common.remove')} ${p.name}`)
    else toast.bad(t('settings.extensions.installFailed', { msg: await failureReason() }))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
    endLog()
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

onMounted(() => {
  if (settingsStore.extensions) data.value = settingsStore.extensions
  void reload().then(() => checkVersions())
})
onBeforeUnmount(() => {
  endLog()
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
        <template v-if="tab === 'installed'">
          <el-button
            size="small"
            :loading="checkingVer"
            :disabled="!!busy || checkingVer"
            @click="checkVersions"
          >
            {{ checkingVer ? t('settings.engine.checking') : t('settings.engine.check') }}
          </el-button>
          <el-button
            v-if="outdated.length"
            size="small"
            type="primary"
            :loading="busy === 'update-all'"
            :disabled="!!busy"
            @click="updatePackage()"
          >
            {{ t('settings.extensions.updateAll') }}
          </el-button>
          <el-button size="small" :loading="busy === 'import'" :disabled="!!busy" @click="importExt">
            {{ t('settings.extensions.import') }}
          </el-button>
          <el-button size="small" :loading="busy === 'reload'" :disabled="!!busy" @click="restart">
            {{ t('settings.extensions.reload') }}
          </el-button>
        </template>
        <el-button
          v-else
          size="small"
          :loading="marketLoading"
          :disabled="marketLoading"
          @click="loadMarket(true)"
        >
          {{ t('settings.extensions.marketRefresh') }}
        </el-button>
      </template>

      <div class="ext-tabs">
        <el-radio-group v-model="tab" size="small">
          <el-radio-button value="installed">{{ t('settings.extensions.tabInstalled') }}</el-radio-button>
          <el-radio-button value="market">{{ t('settings.extensions.tabMarket') }}</el-radio-button>
        </el-radio-group>
      </div>

      <template v-if="tab === 'installed'">
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
                  <el-tag v-if="versionOf(p)" size="small" type="info" class="ext-ver">{{ versionOf(p) }}</el-tag>
                  <el-tag v-if="latestMap[p.source]?.update_available" size="small" type="warning">
                    {{ t('settings.extensions.latest', { version: latestMap[p.source].latest_version }) }}
                  </el-tag>
                </span>
                <span class="wb-mono" :title="p.path || p.source">{{ p.source }}</span>
              </div>
              <el-button
                v-if="latestMap[p.source]?.update_available"
                size="small"
                type="primary"
                :loading="busy === `up:${p.source}`"
                :disabled="!!busy"
                @click="updatePackage(p.source)"
              >
                {{ t('settings.extensions.update') }}
              </el-button>
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
      </template>

      <div v-else class="ext-group">
        <div class="ext-bar">
          <el-input
            v-model="marketFilter"
            size="small"
            class="ext-input"
            clearable
            :placeholder="t('settings.extensions.marketFilter')"
          />
        </div>
        <p v-if="market?.error" class="ext-warn">
          <AppIcon name="alert" size="ic-2xs" />
          {{ t('settings.extensions.marketFail', { msg: market.error }) }}
        </p>
        <div v-else-if="marketLoading && !market" class="wb-card ext-card">
          <el-skeleton animated :rows="4" />
        </div>
        <div v-else-if="marketPackages.length" class="ext-list">
          <article v-for="p in marketPackages" :key="p.name" class="ext-row">
            <span class="ext-tile"><AppIcon name="database" size="ic-sm" /></span>
            <div class="ext-tx">
              <span class="ext-name">
                {{ p.name }}
                <el-tag size="small" type="info" class="ext-ver">v{{ p.version }}</el-tag>
              </span>
              <span class="ext-desc" :title="p.description">{{ p.description || '—' }}</span>
              <span v-if="marketMeta(p)" class="ext-meta">{{ marketMeta(p) }}</span>
            </div>
            <el-tag v-if="p.installed" size="small" type="success" class="ext-installed">
              {{ t('settings.extensions.marketInstalled') }}
            </el-tag>
            <el-tag v-else-if="p.installed_version" size="small" type="info" class="ext-ver">
              {{ p.installed_version }}
            </el-tag>
            <el-button
              v-if="!p.installed"
              size="small"
              type="primary"
              :loading="busy === `mkt:${p.name}`"
              :disabled="!!busy"
              @click="installFromMarket(p)"
            >
              {{ t('settings.extensions.marketInstall') }}
            </el-button>
          </article>
        </div>
        <div v-else class="wb-card empty-box">
          <span class="wb-tile wb-tile--lg"><AppIcon name="braces" size="ic-xl" /></span>
          <b>{{ marketFilter ? t('settings.extensions.marketNoHit') : t('settings.extensions.marketEmpty') }}</b>
          <p>{{ t('settings.extensions.marketEmptyHint') }}</p>
        </div>
      </div>

      <div v-if="log.length" class="ext-log">
        <div class="wb-label">{{ t('setup.installLog') }}</div>
        <pre class="log-body">{{ log.join('\n') }}</pre>
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
.ext-tabs {
  display: flex;
  margin-bottom: var(--wb-sp-2);
}
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
.ext-card {
  padding: var(--wb-sp-4);
}
.ext-desc {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.ext-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.ext-warn {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  line-height: var(--wb-lh-base);
}
.ext-installed {
  flex: none;
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
.ext-ver {
  font-family: var(--font-mono);
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
