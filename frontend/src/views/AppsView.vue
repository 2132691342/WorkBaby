<script setup lang="ts">
// 应用页：内置运行时与 pi 引擎的安装状态。
// 不自己实现安装：pi 用的是内置 npm，命令落点全在沙箱里；
// 每行同时回答「装的什么版本、能不能动、落点在哪」。渲染细节在 AppCard。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import AppCard from '@/components/apps/AppCard.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { AppRow } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const loading = ref(false)
const busy = ref<Record<string, boolean>>({})
const copied = ref('')
const repairing = ref(false)

// 内置运行时与引擎分开摆：前者是「环境」，后者是「引擎」，混在一起
// 会让人以为 node 也能升级。
const runtimeRows = computed(() => app.apps.filter((row) => row.app_key !== 'pi'))
const engineRows = computed(() => app.apps.filter((row) => row.app_key === 'pi'))

async function load() {
  loading.value = true
  try {
    await app.loadApps()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    window.setTimeout(() => {
      if (copied.value === text) copied.value = ''
    }, 1600)
  } catch {
    toasts.push('warn', t('apps.copyFailed'))
  }
}

/** 三个动作共用一个入口：只有按钮文案与端点不同。 */
async function run(row: AppRow, action: 'install' | 'update' | 'uninstall') {
  if (busy.value[row.app_key]) return
  busy.value = { ...busy.value, [row.app_key]: true }
  try {
    if (action === 'install') await api.installApp()
    else if (action === 'update') await api.updateApp()
    else await api.uninstallApp()
    await app.loadApps()
    await app.refreshEngines()
    toasts.push('ok', t(`apps.done.${action}`, { name: row.app_name }))
  } catch (err) {
    // 失败不弹窗：卡片里已有进度与输出，消息只补一句「为什么失败」。
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busy.value = { ...busy.value, [row.app_key]: false }
  }
}

/** 修复运行时：停引擎后把三个内置运行时强制重解压，坏掉的安装当场换新。 */
async function repairRuntimes() {
  if (repairing.value) return
  try {
    await ElMessageBox.confirm(t('apps.repairConfirm'), t('apps.repairTitle'), {
      confirmButtonText: t('apps.repair'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  repairing.value = true
  try {
    const out = await api.repairRuntime()
    await app.loadApps()
    await app.refreshEngines()
    if (out.repair_problems.length) {
      toasts.push('warn', out.repair_problems.join('；'))
    } else {
      toasts.push('ok', t('apps.repairDone'))
    }
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    repairing.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('apps.title') }}</h1>
        <p class="page-sub">{{ t('apps.subtitle') }}</p>
      </div>
      <div class="head-actions">
        <button
          class="wb-btn icon ghost"
          :title="t('apps.recheck')"
          :class="{ 'is-loading': loading }"
          :disabled="loading"
          @click="load"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <section v-if="engineRows.length">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t('apps.group.engine') }}</span>
        </div>
        <div class="app-list">
          <AppCard
            v-for="row in engineRows"
            :key="row.app_key"
            :row="row"
            :progress="app.appProgress"
            :busy="Boolean(busy[row.app_key])"
            :copied="copied"
            @run="run(row, $event)"
            @copy="copy"
          />
        </div>
      </section>

      <section v-if="runtimeRows.length">
        <div class="wb-sec-head">
          <span class="wb-sec-title">
            {{ t('apps.group.runtime') }} <span class="count">{{ runtimeRows.length }}</span>
          </span>
          <button
            class="wb-btn sm ghost"
            :class="{ 'is-loading': repairing }"
            :disabled="repairing"
            :title="t('apps.repairConfirm')"
            @click="repairRuntimes"
          >
            <AppIcon name="refresh" :size="14" />
            {{ t('apps.repair') }}
          </button>
        </div>
        <div class="app-list">
          <AppCard
            v-for="row in runtimeRows"
            :key="row.app_key"
            :row="row"
            :progress="app.appProgress"
            :busy="Boolean(busy[row.app_key])"
            :copied="copied"
            @run="run(row, $event)"
            @copy="copy"
          />
        </div>
      </section>

      <p class="note">{{ t('apps.boundary') }}</p>
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
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-5);
}

.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-faint);
  letter-spacing: 0;
  text-transform: none;
}

.app-list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.note {
  max-width: 860px;
  margin: 0;
  font-size: var(--wb-fs-2xs);
  line-height: 1.7;
  color: var(--wb-faint);
}
</style>
