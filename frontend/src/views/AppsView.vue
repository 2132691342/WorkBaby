<script setup lang="ts">
// 应用页：装 / 升级 / 卸载两个引擎。
// 不自己实现安装：跑的是引擎官方安装器的原话，界面显示与真正执行是同一串；
// 每行同时回答「装的什么版本、能不能装、会跑哪条命令」。渲染细节在 AppCard。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
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

const groups = computed(() => [
  { key: 'installed', rows: app.apps.filter((row) => row.installed) },
  { key: 'missing', rows: app.apps.filter((row) => !row.installed) },
])

async function load(refresh = false) {
  loading.value = true
  try {
    await app.loadApps(refresh)
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
  if (busy.value[row.kind]) return
  busy.value = { ...busy.value, [row.kind]: true }
  try {
    if (action === 'install') await api.installApp(row.kind)
    else if (action === 'update') await api.updateApp(row.kind)
    else await api.uninstallApp(row.kind)
    await app.loadApps(true)
    await app.refreshEngines()
    toasts.push('ok', t(`apps.done.${action}`, { name: row.app_name }))
  } catch (err) {
    // 失败不弹窗：卡片里已有命令原文与输出，消息只补一句「为什么失败」。
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busy.value = { ...busy.value, [row.kind]: false }
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
          :title="t('apps.recheckLatest')"
          :class="{ 'is-loading': loading }"
          :disabled="loading"
          @click="load(true)"
        >
          <AppIcon name="refresh" :size="15" />
        </button>
      </div>
    </header>

    <div class="page-body">
      <section v-for="group in groups" :key="group.key" v-show="group.rows.length">
        <div class="wb-sec-head">
          <span class="wb-sec-title">{{ t(`apps.group.${group.key}`) }} <span class="count">{{ group.rows.length }}</span></span>
        </div>
        <div class="app-list">
          <AppCard
            v-for="row in group.rows"
            :key="row.kind"
            :row="row"
            :progress="app.appProgress[row.kind]"
            :busy="Boolean(busy[row.kind])"
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
  max-width: var(--wb-w-desc, 620px);
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
