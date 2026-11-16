<script setup lang="ts">
// 联网：搜索服务配置 + 搜索与抓取。
// 这一页是「模型之外的第二张嘴」：模型能搜网读网页，用户也能自己搜。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { openExternal } from '@/api/wails'
import type { WebFetchView, WebSearchItem, WebStatusView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

// 联网设置。
const status = ref<WebStatusView | null>(null)
const kind = ref('')
const apiKey = ref('')
const saving = ref(false)
// 首屏取数是否失败：失败时不能显示「服务清单是空的」——那是一句假断言。
const loadFailed = ref(false)
const loading = ref(false)

// 搜索。
const query = ref('')
const searching = ref(false)
const results = ref<WebSearchItem[]>([])
const searched = ref(false)

// 抓取。
const fetchUrl = ref('')
const fetching = ref(false)
const fetched = ref<WebFetchView | null>(null)

// 竞态守卫：连点搜索 / 抓取时，晚到的旧响应不能盖掉新结果。
let searchSeq = 0
let fetchSeq = 0

// 当前搜索服务需要密钥时，密钥输入框才露出来。
const needsKey = computed(() => kind.value !== 'duckduckgo')
const canSearch = computed(() => query.value.trim().length > 0 && !searching.value)

/** 读一次联网状态：首屏与「重试」共用一份。 */
async function reload(): Promise<void> {
  loading.value = true
  try {
    status.value = await api.webStatus()
    kind.value = status.value.web_kind
    loadFailed.value = false
  } catch (err) {
    loadFailed.value = true
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void reload()
})

/** 保存联网设置。 */
async function save(): Promise<void> {
  if (saving.value) return
  saving.value = true
  try {
    status.value = await api.webSettings(kind.value, apiKey.value.trim())
    // 密钥不回显：提交后清空输入框，避免密钥留在屏幕上。
    apiKey.value = ''
    toasts.push('ok', t('web.saved'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

/** 执行搜索。 */
async function search(): Promise<void> {
  const term = query.value.trim()
  if (!term || searching.value) return
  const mine = ++searchSeq
  searching.value = true
  try {
    const res = await api.webSearch(term, 10)
    if (mine !== searchSeq) return
    results.value = res.search_results ?? []
    searched.value = true
    if (!results.value.length) toasts.push('info', t('web.noResults'))
  } catch (err) {
    if (mine !== searchSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === searchSeq) searching.value = false
  }
}

/** 抓取一个网页。 */
async function fetchPage(): Promise<void> {
  const url = fetchUrl.value.trim()
  if (!url || fetching.value) return
  const mine = ++fetchSeq
  fetching.value = true
  fetched.value = null
  try {
    const next = await api.webFetch(url)
    if (mine !== fetchSeq) return
    fetched.value = next
  } catch (err) {
    if (mine !== fetchSeq) return
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    if (mine === fetchSeq) fetching.value = false
  }
}

/** 把结果里的地址填进抓取框。 */
function useUrl(url: string): void {
  fetchUrl.value = url
}

/** 交给系统浏览器打开：失败要出声，否则看起来就是「这个链接是死的」。 */
async function openInBrowser(url: string): Promise<void> {
  try {
    await openExternal(url)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('web.title') }}</h1>
        <p class="page-sub">{{ t('web.desc') }}</p>
      </div>
    </header>

    <div class="page-body">

    <!-- 首屏取数失败：说清「没取到」并给重试，不摆一个空的服务下拉 -->
    <section v-if="loadFailed && !status" class="load-failed">
      <AppIcon name="alert" :size="20" class="failed-ic" />
      <p class="failed-title">{{ t('common.loadFailed') }}</p>
      <p class="failed-hint">{{ t('web.loadFailedHint') }}</p>
      <button class="wb-btn sm primary" type="button" @click="reload">{{ t('common.retry') }}</button>
    </section>

    <div v-else class="web-cols">
      <!-- 搜索服务配置 -->
      <section class="wb-card card-pad">
        <h2 class="wb-section-title">{{ t('web.providerTitle') }}</h2>
        <div class="field-row">
          <label class="field-label">{{ t('web.providerKind') }}</label>
          <el-select v-model="kind" class="field-control">
            <el-option
              v-for="item in status?.web_kinds ?? []"
              :key="item"
              :label="item"
              :value="item"
            />
          </el-select>
        </div>
        <div v-if="needsKey" class="field-row">
          <label class="field-label">{{ t('web.apiKey') }}</label>
          <el-input
            v-model="apiKey"
            type="password"
            show-password
            :placeholder="t('web.apiKeyPlaceholder')"
            class="field-control"
          />
        </div>
        <p class="set-hint">{{ status?.web_message }}</p>
        <button type="button"
          class="wb-btn primary"
          :class="{ 'is-loading': saving }"
          :disabled="saving"
          @click="save"
        >
          {{ t('common.save') }}
        </button>
      </section>

      <!-- 搜索 -->
      <section class="wb-card card-pad">
        <h2 class="wb-section-title">{{ t('web.searchTitle') }}</h2>
        <div class="search-bar">
          <el-input
            v-model="query"
            :placeholder="t('web.searchPlaceholder')"
            clearable
            @keyup.enter="search"
          />
          <button type="button"
            class="wb-btn primary"
            :class="{ 'is-loading': searching }"
            :disabled="!canSearch"
            @click="search"
          >
            {{ t('web.searchAction') }}
          </button>
        </div>
        <ul v-if="results.length" class="result-list">
          <li v-for="(item, index) in results" :key="index" class="result-item">
            <!-- 不开新窗口：WebView2 里 target=_blank 什么都不发生（点了没反应）。
                 走系统浏览器，用户才能真读到这一页。 -->
            <button class="result-title" type="button" :title="item.result_url" @click="openInBrowser(item.result_url)">
              {{ item.result_title || item.result_url }}
            </button>
            <span class="result-url">{{ item.result_url }}</span>
            <p v-if="item.result_snippet" class="result-snippet">{{ item.result_snippet }}</p>
            <button type="button" class="wb-btn text sm" @click="useUrl(item.result_url)">
              {{ t('web.readIt') }}
            </button>
          </li>
        </ul>
        <p v-else-if="searched" class="empty-hint">{{ t('web.noResults') }}</p>
      </section>

      <!-- 抓取 -->
      <section class="wb-card card-pad">
        <h2 class="wb-section-title">{{ t('web.fetchTitle') }}</h2>
        <div class="search-bar">
          <el-input
            v-model="fetchUrl"
            :placeholder="t('web.fetchPlaceholder')"
            clearable
            @keyup.enter="fetchPage"
          />
          <button type="button"
            class="wb-btn"
            :class="{ 'is-loading': fetching }"
            :disabled="!fetchUrl.trim() || fetching"
            @click="fetchPage"
          >
            {{ t('web.fetchAction') }}
          </button>
        </div>
        <div v-if="fetched" class="fetched">
          <p class="fetched-meta">{{ fetched.fetch_url }} · {{ fetched.fetch_char_count }}</p>
          <pre class="fetched-text">{{ fetched.fetch_text }}</pre>
        </div>
      </section>
    </div>
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

/* 页头：标题与说明在左——与其余管理页同一副骨架 */
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
  margin: 2px 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-2xs);
  line-height: 1.55;
  color: var(--wb-mute);
}

.page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}

.web-cols {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  max-width: var(--wb-w-desc);
}

.field-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  margin-bottom: var(--wb-sp-3);
}

.field-label {
  width: 88px;
  flex: none;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}

.field-control {
  flex: 1;
  max-width: 320px;
}

.search-bar {
  display: flex;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}

.result-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}

.result-item {
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  background: var(--wb-surface);
  transition: border-color var(--wb-dur-fast) var(--wb-ease);
}

.result-item:hover {
  border-color: var(--wb-brand-2);
}

.result-title {
  display: block;
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  font-family: inherit;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-brand);
  text-align: left;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.result-title:hover {
  text-decoration: underline;
  color: var(--wb-brand-strong);
}

.result-title:active {
  transform: scale(var(--wb-press-active));
}

.result-url {
  display: block;
  margin-top: 2px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
  word-break: break-all;
}

.result-snippet {
  margin: var(--wb-sp-2) 0 0;
  font-size: var(--wb-fs-xs);
  line-height: var(--wb-lh-base);
  color: var(--wb-ink-2);
}

.empty-hint,
.set-hint {
  margin: var(--wb-sp-2) 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

/* 首屏取数失败：一块说明 + 一个重试，不是一个空的服务下拉 */
.load-failed {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-5);
  text-align: center;
}
.failed-ic {
  color: var(--wb-warn);
}
.failed-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink-2);
}
.failed-hint {
  margin: 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-2xs);
  line-height: 1.6;
  color: var(--wb-mute);
}

.fetched {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r);
  overflow: hidden;
}

.fetched-meta {
  margin: 0;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  background: var(--wb-sunken);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.fetched-text {
  margin: 0;
  padding: var(--wb-sp-3);
  max-height: 420px;
  overflow: auto;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  line-height: var(--wb-lh-base);
  color: var(--wb-ink);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
