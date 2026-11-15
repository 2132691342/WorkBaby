<script setup lang="ts">
// 应用页：外部工具链（git / node / python）的检测状态。
// 引擎在进程内，没有「装没装」这件事，所以这里没有引擎这一组。
// 壳不代装任何一个：这里只回答「检测到了什么、从哪发现的、没检测到该去做什么」。
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/common/AppIcon.vue'
import AppCard from '@/components/apps/AppCard.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()
const router = useRouter()

const loading = ref(false)
const detecting = ref(false)
const copied = ref('')
// 竞态守卫：连点刷新时，晚到的旧清单不能盖掉新的。
let loadSeq = 0

// 引擎在进程内，应用页只剩外部工具链这一组。
const envRows = computed(() => app.apps)

async function load() {
  const mine = ++loadSeq
  loading.value = true
  try {
    await app.loadApps()
  } catch (err) {
    if (mine !== loadSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === loadSeq) loading.value = false
  }
}

/** 重新探测：用户刚在终端里装好工具，回来点这一下就能用。 */
async function detect() {
  if (detecting.value) return
  detecting.value = true
  try {
    await api.detectEngines()
    await app.loadApps()
    await app.refreshEngines()
    toasts.push('ok', t('apps.detected'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    detecting.value = false
  }
}

onUnmounted(() => {
  loadSeq += 1
})

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
      <WbSkeleton v-if="loading && !envRows.length" :rows="3" />

      <section v-else-if="envRows.length">
        <div class="wb-sec-head">
          <span class="wb-sec-title">
            {{ t('apps.group.runtime') }} <span class="count">{{ envRows.length }}</span>
          </span>
          <div class="sec-actions">
            <button class="wb-btn sm" :class="{ 'is-loading': detecting }" :disabled="detecting" @click="detect">
              <AppIcon name="refresh" :size="14" />
              {{ t('apps.detect') }}
            </button>
          </div>
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

      <WbEmpty
        v-else-if="!loading"
        icon="terminal"
        :title="t('apps.emptyTitle')"
        :hint="t('apps.emptyHint')"
      >
        <template #action>
          <button class="wb-btn sm primary" @click="router.push({ name: 'chat' })">
            {{ t('apps.startChat') }}
          </button>
        </template>
      </WbEmpty>

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
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
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
