<script setup lang="ts">
import { errorText } from '../utils/errors'
// 帮助页：内置文档目录 + Markdown 阅读；URL 是唯一真相（/help/:name）。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import type { HelpDocVO } from '../types/api'
import { renderMarkdown } from '../utils/md'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'
import SidePanel from '../components/layout/SidePanel.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const items = ref<HelpDocVO[]>([])
const listError = ref('')
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
    items.value = await api.docs.list()
    // 直接进 /help 时落到第一篇（新手最需要的「快速上手」）
    if (!current.value && items.value.length) {
      void router.replace(`/help/${items.value[0].doc_name}`)
    }
  } catch (e) {
    listError.value = errorText(e)
  }
}

async function loadDoc(name: string) {
  if (!name) return
  loading.value = true
  docError.value = ''
  try {
    const doc = await api.docs.content(name)
    // 首行 H1 就是页面头，正文里再渲染一遍会变成「标题下面还有一个同样的标题」
    html.value = renderMarkdown(doc.resp_content.replace(/^#\s.*\r?\n?/, ''))
  } catch (e) {
    docError.value = errorText(e) || t('help.missing')
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
  <div class="help">
    <SidePanel width="sub" :title="t('help.toc')">
      <el-input v-model="filter" size="small" class="help-search" :placeholder="t('help.search')" clearable>
        <template #prefix><AppIcon name="search" size="ic-xs" /></template>
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
          <AppIcon name="doc" size="ic-sm" />
          <span>{{ d.doc_title }}</span>
        </button>
        <p v-if="!shown.length" class="help-empty">{{ t('common.empty') }}</p>
      </nav>
      <p v-if="listError" class="notice-bar notice-bar--bad help-err">{{ listError }}</p>
    </SidePanel>

    <div class="help-main">
      <header class="help-head">
        <h1 class="help-title">{{ title }}</h1>
        <p class="help-sub">{{ t('help.subtitle') }}</p>
      </header>
      <div class="help-main">
        <PageHeader :title="title" />
        <div class="help-scroll">
          <div class="help-col">
            <div v-if="loading" class="wb-card help-card"><el-skeleton animated :rows="8" /></div>
            <div v-else-if="docError" class="wb-card help-card help-blank">
              <AppIcon name="alert" size="ic-xl" />
              <b>{{ docError }}</b>
              <el-button size="small" @click="loadDoc(current)">{{ t('common.retry') }}</el-button>
            </div>
            <article v-else class="wb-card help-card prose" v-html="html" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.help {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.help-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--wb-elev-window);
}
.help-search {
  margin-bottom: var(--wb-sp-2);
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
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  text-align: left;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease);
}
.help-item:hover {
  background: var(--wb-nav-hover-bg);
  color: var(--wb-ink);
}
.help-item.is-on {
  background: var(--wb-nav-active-bg);
  color: var(--wb-nav-active-ink);
  box-shadow: inset 2px 0 0 var(--wb-primary);
}
.help-empty {
  padding: var(--wb-sp-4) var(--wb-sp-3);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.help-err {
  margin-top: var(--wb-sp-2);
}
.help-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.help-head {
  flex: none;
  padding: var(--wb-sp-4) var(--wb-sp-5);
  border-bottom: 1px solid var(--wb-border);
}
.help-title {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 700;
  letter-spacing: -0.015em;
  color: var(--wb-ink);
}
.help-sub {
  margin-top: 2px;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.help-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.help-col {
  max-width: calc(860px * var(--wb-vw-scale));
  margin: 0 auto;
  padding: var(--wb-sp-5);
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
  color: var(--wb-muted);
}
.help-blank b {
  color: var(--wb-ink);
}
@media (max-width: 1100px) {
  .help-card {
    padding: var(--wb-sp-5);
  }
}
</style>
