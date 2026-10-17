<script setup lang="ts">
// 帮助页：内置文档目录 + Markdown 阅读；URL 是唯一真相（/help/:name）。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '../api'
import type { HelpDocVO } from '../types/api'
import { renderMarkdown } from '../utils/md'
import AppIcon from '../components/common/AppIcon.vue'
import PageState from '../components/common/PageState.vue'

const route = useRoute()
const router = useRouter()

const items = ref<HelpDocVO[]>([])
const listError = ref('')
const html = ref('')
const loading = ref(true)
const docError = ref('')

const current = computed(() => (typeof route.params.name === 'string' ? route.params.name : ''))
const title = computed(() => items.value.find((d) => d.name === current.value)?.title || '帮助')

async function loadList() {
  try {
    items.value = await api.docs.list()
    // 直接进 /help 时落到第一篇（新手最需要的「快速上手」）
    if (!current.value && items.value.length) {
      void router.replace(`/help/${items.value[0].name}`)
    }
  } catch (e) {
    listError.value = (e as Error)?.message || '读不到帮助文档'
  }
}

async function loadDoc(name: string) {
  if (!name) return
  loading.value = true
  docError.value = ''
  try {
    const doc = await api.docs.content(name)
    // 首行 H1 就是页面头，正文里再渲染一遍会变成「标题下面还有一个同样的标题」
    html.value = renderMarkdown(doc.content.replace(/^#\s.*\r?\n?/, ''))
  } catch (e) {
    docError.value = (e as Error)?.message || '这篇文档打不开'
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
  <div class="help wb-ui">
    <aside class="side">
      <div class="side-brand">
        <div class="logo"><AppIcon name="book" size="ic-sm" /></div>
        <div>
          <b>帮助</b>
          <small>使用手册</small>
        </div>
      </div>
      <nav class="nav">
        <button
          v-for="d in items"
          :key="d.name"
          class="nav-item"
          :class="{ on: d.name === current }"
          type="button"
          @click="router.push(`/help/${d.name}`)"
        >
          <AppIcon name="doc" /> {{ d.title }}
        </button>
      </nav>
      <p v-if="listError" class="alert a-danger side-err">
        <AppIcon name="alert" size="ic-sm" />
        <span>{{ listError }}</span>
      </p>
    </aside>

    <div class="help-main scroll">
      <div class="wrap-md">
        <header class="page-head">
          <h1>{{ title }}</h1>
        </header>
        <PageState :loading="loading" :error="docError">
          <article class="card doc-card prose" v-html="html" />
        </PageState>
      </div>
    </div>
  </div>
</template>

<style scoped>
.help {
  display: flex;
  height: 100%;
  min-height: 0;
  flex: 1;
  min-width: 0;
}
.nav {
  display: grid;
  gap: 2px;
  padding: 0 var(--wb-sp-3);
}
.nav-item {
  width: 100%;
  min-width: 0;
}
.side-err {
  margin: 0 var(--wb-sp-3) var(--wb-sp-3);
}
.help-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  overflow-x: hidden;
}
.wrap-md {
  width: 100%;
  max-width: 860px;
  margin: 0 auto;
  padding: var(--wb-sp-8) var(--wb-sp-6) var(--wb-sp-10);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.page-head h1 {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 600;
  color: var(--wb-ink);
  letter-spacing: -0.01em;
}
/* 阅读版心：文档页比聊天列宽一点，长文段落更好读 */
.doc-card {
  padding: var(--wb-sp-6) var(--wb-sp-8);
  font-size: var(--wb-fs-lg);
  line-height: var(--wb-lh-loose);
}
@media (max-width: 1100px) {
  .doc-card {
    padding: var(--wb-sp-5) var(--wb-sp-5);
  }
}
</style>
