<script setup lang="ts">
/**
 * Wiki 仓库导读（设置中心 tab）：结构化导读当前会话工作区——
 * 语言统计 / 入口文件 / 目录树 + 目录页与文件页（源码正文回链路径）。
 * 确定性生成（不依赖 LLM）：扫描即产出，刷新即最新。
 */
import { computed, onMounted, ref } from 'vue'
import { BookOpen, RefreshCw, FolderSearch, Code2 } from '@/components/common/icons'
import { apiGet } from '@/api/client'
import { useChatStore } from '@/stores/chat'
import { t } from '@/i18n'
import { useToast } from '@/composables/useToast'

interface WikiNode {
  name: string
  path: string
  type: 'dir' | 'file'
  lines: number
  lang?: string
  summary?: string
  children?: WikiNode[]
}
interface WikiLang { lang: string; files: number; lines: number }
interface WikiEntry { path: string; lang: string; lines: number; summary: string }
interface WikiOverview {
  root: string
  name: string
  total_files: number
  total_lines: number
  langs: WikiLang[]
  entries: WikiEntry[]
  tree: WikiNode[]
}
interface WikiFileSummary { path: string; lang: string; lines: number; summary: string }
interface WikiPage {
  path: string
  type: 'dir' | 'file'
  lang?: string
  lines: number
  summary: string
  children?: WikiFileSummary[]
  content?: string
  truncated?: boolean
}

const chat = useChatStore()
const toast = useToast()

const overview = ref<WikiOverview | null>(null)
const page = ref<WikiPage | null>(null)
const loading = ref(false)
const failed = ref(false)
const scanning = ref(false)

const sessionId = computed(() => chat.currentID ?? '')

onMounted(load)

async function load(): Promise<void> {
  if (!sessionId.value) return
  loading.value = true
  failed.value = false
  page.value = null
  try {
    overview.value = await apiGet<WikiOverview>(
      `/api/v1/wiki/overview?session_id=${encodeURIComponent(sessionId.value)}`
    )
  } catch {
    overview.value = null
    failed.value = true
  } finally {
    loading.value = false
  }
}

async function openPage(node: { path: string; type?: string }): Promise<void> {
  if (!sessionId.value) return
  scanning.value = true
  try {
    page.value = await apiGet<WikiPage>(
      `/api/v1/wiki/page?session_id=${encodeURIComponent(sessionId.value)}&path=${encodeURIComponent(node.path)}`
    )
  } catch (e) {
    toast.error(t('wiki.pageFailed'), e instanceof Error ? e.message : String(e))
  } finally {
    scanning.value = false
  }
}

function onTreeNodeClick(node: WikiNode): void {
  void openPage(node)
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-lg">
      <header class="hero">
        <div class="tile"><BookOpen class="ic" /></div>
        <div>
          <h1>{{ t('wiki.title') }}</h1>
          <p>{{ t('wiki.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn" :disabled="loading || !sessionId" @click="load">
          <RefreshCw class="ic ic-sm" :class="{ 'animate-spin': loading }" />
          {{ t('wiki.rescan') }}
        </button>
      </header>

      <!-- 未选会话 / 工作区不可用 -->
      <div v-if="!sessionId" class="alert a-info">{{ t('wiki.needSession') }}</div>
      <div v-else-if="loading" class="card p-5 text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
      <div v-else-if="failed" class="alert a-warning">{{ t('wiki.unavailable') }}</div>

      <template v-else-if="overview">
        <!-- 画像条 -->
        <section class="card mb-3 p-4">
          <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
            <span class="font-display text-sm font-semibold text-wb-ink">{{ overview.name }}</span>
            <span class="fs11 muted" :title="overview.root">{{ overview.total_files }} {{ t('wiki.files') }} · {{ overview.total_lines }} {{ t('wiki.lines') }}</span>
          </div>
          <div class="flex flex-wrap gap-1.5">
            <span v-for="l in overview.langs.slice(0, 8)" :key="l.lang" class="wiki-lang">
              {{ l.lang }} · {{ l.files }} · {{ l.lines }}
            </span>
          </div>
          <!-- 入口文件 -->
          <div v-if="overview.entries.length" class="mt-3 grid gap-2 md:grid-cols-2">
            <button
              v-for="e in overview.entries"
              :key="e.path"
              type="button"
              class="wiki-entry"
              @click="openPage(e)"
            >
              <Code2 class="h-3.5 w-3.5 shrink-0 text-wb-primary" />
              <span class="min-w-0 flex-1 text-left">
                <span class="block truncate font-medium text-wb-ink">{{ e.path }}</span>
                <span class="block truncate text-[11px] text-wb-muted">{{ e.summary }}</span>
              </span>
            </button>
          </div>
        </section>

        <!-- 目录树 + 页面 -->
        <div class="grid gap-3 lg:grid-cols-[22rem_1fr]">
          <section class="card wiki-tree p-3">
            <p class="mb-2 flex items-center gap-1.5 text-xs font-semibold text-wb-ink">
              <FolderSearch class="h-3.5 w-3.5 text-wb-primary" />
              {{ t('wiki.tree') }}
            </p>
            <el-tree
              :data="overview.tree"
              :props="{ label: 'name', children: 'children' }"
              node-key="path"
              :expand-on-click-node="false"
              :default-expand-all="false"
              class="wiki-el-tree"
              @node-click="onTreeNodeClick"
            >
              <template #default="{ data }">
                <span class="wiki-tree-node" :class="{ 'is-file': data.type === 'file' }">
                  <span class="truncate" :title="data.summary || data.name">{{ data.name }}</span>
                  <span v-if="data.type === 'file'" class="wiki-lines">{{ data.lines }}</span>
                </span>
              </template>
            </el-tree>
          </section>

          <section class="card min-h-[24rem] p-4">
            <div v-if="scanning" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
            <template v-else-if="page">
              <div class="mb-2 flex items-baseline gap-2">
                <h2 class="truncate font-display text-sm font-semibold text-wb-ink" :title="page.path">
                  {{ page.path || t('wiki.rootPage') }}
                </h2>
                <span v-if="page.lines" class="fs11 muted">{{ page.lines }} {{ t('wiki.lines') }}</span>
              </div>

              <!-- 目录页：子项清单 -->
              <div v-if="page.type === 'dir' && page.children" class="space-y-1">
                <button
                  v-for="c in page.children"
                  :key="c.path"
                  type="button"
                  class="wiki-child"
                  @click="openPage(c)"
                >
                  <span class="min-w-0 flex-1 truncate text-left font-medium text-wb-ink">{{ c.path }}</span>
                  <span class="shrink-0 text-[11px] text-wb-muted">{{ c.lang || '' }} {{ c.lines }}</span>
                  <span class="hidden min-w-0 flex-[2] truncate text-left text-[11px] text-wb-muted md:block">{{ c.summary }}</span>
                </button>
              </div>

              <!-- 文件页：正文 -->
              <pre v-else-if="page.content" class="wiki-code">{{ page.content }}</pre>
              <p v-else class="text-xs text-wb-muted">{{ t('wiki.emptyPage') }}</p>
              <p v-if="page.truncated" class="mt-2 fs11 text-wb-warning">{{ t('wiki.truncated') }}</p>
            </template>
            <p v-else class="text-sm text-wb-muted">{{ t('wiki.pickNode') }}</p>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.wiki-lang {
  border: 1px solid var(--wb-border);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 11px;
  color: var(--wb-ink);
  background: var(--wb-surface-2);
}
.wiki-entry {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--wb-border);
  border-radius: 10px;
  background: var(--wb-surface-2);
  padding: 8px 10px;
  cursor: pointer;
  transition: border-color 0.15s ease;
}
.wiki-entry:hover {
  border-color: var(--wb-primary);
}
.wiki-tree {
  max-height: 34rem;
  overflow: auto;
}
.wiki-tree-node {
  display: flex;
  flex: 1;
  min-width: 0;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--wb-ink);
}
.wiki-tree-node.is-file {
  color: var(--wb-muted);
}
.wiki-lines {
  margin-left: auto;
  font-size: 10px;
  color: var(--wb-muted);
}
.wiki-child {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  border: 1px solid transparent;
  border-radius: 8px;
  padding: 5px 8px;
  font-size: 12px;
  cursor: pointer;
  color: var(--wb-ink);
}
.wiki-child:hover {
  border-color: var(--wb-border);
  background: var(--wb-surface-2);
}
.wiki-code {
  max-height: 34rem;
  overflow: auto;
  border: 1px solid var(--wb-border);
  border-radius: 10px;
  background: color-mix(in srgb, var(--wb-ink) 4%, transparent);
  padding: 12px;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre;
  color: var(--wb-ink);
}
</style>
