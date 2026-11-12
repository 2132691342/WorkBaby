<script setup lang="ts">
// 应用页：pi 与它依赖的外部环境的检测状态。
// 壳不代装任何一个：pi 是用户自己的，node / git / python 也是。
// 这里只回答「检测到了什么、从哪发现的、没检测到该去做什么」。
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
const detecting = ref(false)
const copied = ref('')
const locating = ref(false)

// 引擎与它依赖的环境分开摆：前者是「能不能干活」，后者是「能干多少活」。
const engineRows = computed(() => app.apps.filter((row) => row.app_key === 'pi'))
const envRows = computed(() => app.apps.filter((row) => row.app_key !== 'pi'))

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

/** 重新探测：用户刚在终端里装好 pi，回来点这一下就能用。 */
async function detect() {
  if (detecting.value) return
  detecting.value = true
  try {
    await api.detectEngines()
    await app.loadApps()
    await app.refreshEngines()
    const ready = app.apps.find((row) => row.app_key === 'pi')?.installed
    toasts.push(ready ? 'ok' : 'warn', ready ? t('apps.detected') : t('apps.notDetected'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    detecting.value = false
  }
}

/** 手动指定 pi 的位置：自动探测不到时的兜底。 */
async function locate() {
  if (locating.value) return
  locating.value = true
  try {
    const picked = await app.pickPiPath()
    if (!picked) return
    await api.locatePi(picked)
    await app.loadApps()
    await app.refreshEngines()
    toasts.push('ok', t('apps.located'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    locating.value = false
  }
}

/** 清掉手动路径，回到自动探测。 */
async function resetPath(row: AppRow) {
  if (row.app_source !== 'manual') return
  try {
    await api.locatePi('')
    await app.loadApps()
    await app.refreshEngines()
    toasts.push('ok', t('apps.pathReset'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
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
          <div class="sec-actions">
            <button class="wb-btn sm" :class="{ 'is-loading': detecting }" :disabled="detecting" @click="detect">
              <AppIcon name="refresh" :size="14" />
              {{ t('apps.detect') }}
            </button>
            <button class="wb-btn sm ghost" :class="{ 'is-loading': locating }" :disabled="locating" @click="locate">
              <AppIcon name="folder" :size="14" />
              {{ t('apps.locate') }}
            </button>
          </div>
        </div>
        <div class="app-list">
          <AppCard
            v-for="row in engineRows"
            :key="row.app_key"
            :row="row"
            :progress="app.appProgress"
            :busy="detecting"
            :copied="copied"
            @reset="resetPath(row)"
            @copy="copy"
          />
        </div>
      </section>

      <section v-if="envRows.length">
        <div class="wb-sec-head">
          <span class="wb-sec-title">
            {{ t('apps.group.runtime') }} <span class="count">{{ envRows.length }}</span>
          </span>
        </div>
        <div class="app-list">
          <AppCard
            v-for="row in envRows"
            :key="row.app_key"
            :row="row"
            :progress="app.appProgress"
            :busy="false"
            :copied="copied"
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

.sec-actions {
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
