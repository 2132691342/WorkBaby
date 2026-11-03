<script setup lang="ts">
// 帮助页：内置文档目录 + Markdown 阅读；URL 是唯一真相（/help/:name）。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import { renderMarkdown } from '@/utils/md'
import type { HelpDocVO } from '@/types/api'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const toasts = useToasts()

const items = ref<HelpDocVO[]>([])
const html = ref('')
const loading = ref(true)
const docError = ref('')
const filter = ref('')

const current = computed(() => (typeof route.params.name === 'string' ? route.params.name : ''))
const title = computed(
  () => items.value.find((d) => d.doc_name === current.value)?.doc_title || t('help.title'),
)
const shown = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter(
    (d) => d.doc_title.toLowerCase().includes(q) || d.doc_name.toLowerCase().includes(q),
  )
})

async function loadList() {
  try {
    items.value = await api.docs()
    // 直接进 /help 时落到第一篇（新手最需要的「快速上手」）
    if (!current.value && items.value.length) {
      void router.replace(`/help/${items.value[0].doc_name}`)
    }
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function loadDoc(name: string) {
  if (!name) return
  loading.value = true
  docError.value = ''
  try {
    const doc = await api.docContent(name)
    // 首行 H1 就是页面头，正文里再渲染一遍会变成「标题下面还有一个同样的标题」
    html.value = renderMarkdown(doc.doc_content.replace(/^#\s.*\r?\n?/, ''))
  } catch (err) {
    docError.value = err instanceof Error ? err.message : t('help.missing')
    html.value = ''
  } finally {
    loading.value = false
  }
}

watch(
  () => route.params.name,
  (n) => {
    if (typeof n === 'string') void loadDoc(n)
  },
  { immediate: true },
)
void loadList()
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">{{ title }}</h1>
        <p class="page-sub">{{ t('help.subtitle') }}</p>
      </div>
    </header>

    <div class="help-body">
      <aside class="help-toc wb-card">
        <el-input v-model="filter" size="small" :placeholder="t('help.search')" clearable>
          <template #prefix><AppIcon name="search" :size="13" /></template>
        </el-input>
        <nav class="help-nav">
          <button
            v-for="d in shown"
            :key="d.doc_name"
            class="help-item"
            :class="{ 'is-on': d.doc_name === current }"
            type="button"
            @click="router.push(`/help/${d.doc_name}`)"
          >
            <AppIcon name="book" :size="14" />
            <span>{{ d.doc_title }}</span>
          </button>
          <p v-if="!shown.length" class="help-empty">{{ t('help.none') }}</p>
        </nav>
      </aside>

      <div class="help-main">
        <div v-if="loading" class="wb-card help-card"><el-skeleton animated :rows="8" /></div>
        <div v-else-if="docError" class="wb-card help-card help-blank">
          <AppIcon name="alert" :size="22" />
          <b>{{ docError }}</b>
          <button class="wb-btn sm" @click="loadDoc(current)">{{ t('common.retry') }}</button>
        </div>
        <article v-else class="wb-card help-card prose" v-html="html" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.help-body {
  display: flex;
  gap: var(--wb-sp-4);
  flex: 1;
  min-height: 0;
  align-items: stretch;
}

.help-toc {
  width: calc(220px * var(--wb-vw-scale));
  flex: none;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  overflow-y: auto;
}

.help-nav {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.help-item {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-2);
  border: none;
  background: transparent;
  border-radius: var(--wb-r-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease);
}

.help-item:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.help-item.is-on {
  background: var(--wb-selected);
  color: var(--wb-brand-strong);
}

.help-empty {
  padding: var(--wb-sp-3);
  font-size: var(--wb-fs-xs);
  color: var(--wb-mute);
}

.help-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}

.help-card {
  padding: var(--wb-sp-6) var(--wb-sp-8);
}

.help-blank {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10);
  text-align: center;
  color: var(--wb-mute);
}

.help-blank b {
  color: var(--wb-ink);
}
</style>
