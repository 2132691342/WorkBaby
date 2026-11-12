<script setup lang="ts">
// 代码索引：工作区的预索引与快速定位。
// 这一页回答两个问题：「这个仓库有哪些文件与符号」「我要的那个在哪」。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { IndexHitView, IndexStatusView } from '@/types/api'

const { t } = useI18n()
const toasts = useToasts()

// 索引状态。
const status = ref<IndexStatusView | null>(null)
const rebuilding = ref(false)

// 检索。
const query = ref('')
const searching = ref(false)
const hits = ref<IndexHitView[]>([])
const searched = ref(false)

// 索引里有没有东西：没建过索引时检索框不该让人干等。
const ready = computed(() => Boolean(status.value?.index_built))
const canSearch = computed(() => query.value.trim().length > 0 && !searching.value)

onMounted(async () => {
  try {
    status.value = await api.indexStatus()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
})

/** 重建索引。 */
async function rebuild(): Promise<void> {
  if (rebuilding.value) return
  rebuilding.value = true
  try {
    status.value = await api.indexRebuild()
    toasts.push('ok', t('index.rebuild'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    rebuilding.value = false
  }
}

/** 执行检索。 */
async function search(): Promise<void> {
  const term = query.value.trim()
  if (!term || searching.value) return
  searching.value = true
  try {
    const res = await api.indexSearch(term, 30)
    hits.value = res.search_hits ?? []
    status.value = res.index_status
    searched.value = true
    if (!hits.value.length) toasts.push('info', t('index.noResults'))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    searching.value = false
  }
}

/** 命中种类的人话标签。 */
function kindLabel(hit: IndexHitView): string {
  return hit.doc_kind === 'symbol' ? t('index.hitKindSymbol') : t('index.hitKindFile')
}
</script>

<template>
  <div class="index-view">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ t('index.title') }}</h1>
        <p class="page-desc">{{ t('index.desc') }}</p>
      </div>
      <button
        class="wb-btn"
        :class="{ 'is-loading': rebuilding }"
        :disabled="rebuilding"
        @click="rebuild"
      >
        {{ t('index.rebuild') }}
      </button>
    </header>

    <section class="wb-card card-pad">
      <h2 class="wb-section-title">{{ t('index.statusTitle') }}</h2>
      <div class="stat-row">
        <div class="stat">
          <span class="stat-label">{{ t('index.docCount') }}</span>
          <span class="stat-value">{{ status?.index_doc_count ?? 0 }}</span>
        </div>
        <div class="stat">
          <span class="stat-label">{{ t('index.builtAt') }}</span>
          <span class="stat-value small">
            {{
              status?.index_built
                ? new Date(status.index_built_at).toLocaleString()
                : t('index.notBuilt')
            }}
          </span>
        </div>
      </div>
      <p v-if="status?.index_building" class="set-hint">{{ t('index.building') }}</p>
      <p v-else-if="!ready" class="set-hint">{{ t('index.rebuildHint') }}</p>
    </section>

    <section class="wb-card card-pad">
      <h2 class="wb-section-title">{{ t('index.searchTitle') }}</h2>
      <div class="search-bar">
        <el-input
          v-model="query"
          :placeholder="t('index.searchPlaceholder')"
          clearable
          @keyup.enter="search"
        />
        <button
          class="wb-btn primary"
          :class="{ 'is-loading': searching }"
          :disabled="!canSearch"
          @click="search"
        >
          {{ t('index.searchAction') }}
        </button>
      </div>
      <ul v-if="hits.length" class="hit-list">
        <li v-for="(hit, index) in hits" :key="index" class="hit-item">
          <span class="hit-kind">{{ kindLabel(hit) }}</span>
          <span class="hit-path">{{ hit.doc_path }}</span>
          <span v-if="hit.doc_symbol" class="hit-symbol">
            {{ hit.doc_symbol }}<template v-if="hit.doc_line">:{{ hit.doc_line }}</template>
          </span>
          <span v-if="hit.hit_reason" class="hit-reason">{{ hit.hit_reason }}</span>
        </li>
      </ul>
      <p v-else-if="searched" class="empty-hint">{{ t('index.noResults') }}</p>
    </section>
  </div>
</template>

<style scoped>
.index-view {
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

.stat-row {
  display: flex;
  gap: var(--wb-sp-8);
  margin-bottom: var(--wb-sp-2);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stat-label {
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.stat-value {
  font-family: var(--wb-font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 600;
  color: var(--wb-ink);
}

.stat-value.small {
  font-family: var(--wb-font-sans);
  font-size: var(--wb-fs-sm);
  font-weight: 500;
}

.set-hint,
.empty-hint {
  margin: var(--wb-sp-2) 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.search-bar {
  display: flex;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-3);
}

.hit-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}

.hit-item {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-r-sm);
  transition: background-color var(--wb-dur-fast) var(--wb-ease);
}

.hit-item:hover {
  background: var(--wb-hover);
}

.hit-kind {
  flex: none;
  width: 40px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}

.hit-path {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
  word-break: break-all;
}

.hit-symbol {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-brand);
}

.hit-reason {
  margin-left: auto;
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
}
</style>
