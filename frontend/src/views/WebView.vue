<script setup lang="ts">
// 联网：搜索服务配置 + 搜索与抓取。
// 这一页是「模型之外的第二张嘴」：模型能搜网读网页，用户也能自己搜。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { WebFetchView, WebSearchItem, WebStatusView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

// 联网设置。
const status = ref<WebStatusView | null>(null)
const kind = ref('')
const apiKey = ref('')
const saving = ref(false)

// 搜索。
const query = ref('')
const searching = ref(false)
const results = ref<WebSearchItem[]>([])
const searched = ref(false)

// 抓取。
const fetchUrl = ref('')
const fetching = ref(false)
const fetched = ref<WebFetchView | null>(null)

// 当前搜索服务需要密钥时，密钥输入框才露出来。
const needsKey = computed(() => kind.value !== 'duckduckgo')
const canSearch = computed(() => query.value.trim().length > 0 && !searching.value)

onMounted(async () => {
  try {
    status.value = await api.webStatus()
    kind.value = status.value.web_kind
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
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
  searching.value = true
  try {
    const res = await api.webSearch(term, 10)
    results.value = res.search_results ?? []
    searched.value = true
    if (!results.value.length) toasts.push('info', t('web.noResults'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    searching.value = false
  }
}

/** 抓取一个网页。 */
async function fetchPage(): Promise<void> {
  const url = fetchUrl.value.trim()
  if (!url || fetching.value) return
  fetching.value = true
  fetched.value = null
  try {
    fetched.value = await api.webFetch(url)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    fetching.value = false
  }
}

/** 把结果里的地址填进抓取框。 */
function useUrl(url: string): void {
  fetchUrl.value = url
}
</script>

<template>
  <div class="web-view">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('web.title') }}</h1>
        <p class="page-desc">{{ t('web.desc') }}</p>
      </div>
    </header>

    <div class="web-cols">
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
        <button
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
          <button
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
            <a
              class="result-title"
              :href="item.result_url"
              target="_blank"
              rel="noreferrer noopener"
            >{{ item.result_title || item.result_url }}</a>
            <span class="result-url">{{ item.result_url }}</span>
            <p v-if="item.result_snippet" class="result-snippet">{{ item.result_snippet }}</p>
            <button class="wb-btn text sm" @click="useUrl(item.result_url)">
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
          <button
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
</template>

<style scoped>
.web-view {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  padding: var(--wb-sp-5);
  height: 100%;
  overflow-y: auto;
}

.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-4);
}

.page-title {
  margin: 0;
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-2xl);
  font-weight: 600;
  color: var(--wb-ink);
}

.page-desc {
  margin: var(--wb-sp-1) 0 0;
  max-width: var(--wb-w-desc);
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
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
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-brand);
  text-decoration: none;
}

.result-title:hover {
  text-decoration: underline;
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
