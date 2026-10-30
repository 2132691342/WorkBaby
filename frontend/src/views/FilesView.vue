<script setup lang="ts">
// 文件页：浏览工作区文件树，点开文档类文件就地预览（PDF / Word / Excel / 文本）。
// 预览走 /files/bytes 取字节，PDF 用 pdfjs 渲染，Word 用 docx-preview，Excel 用 XLSX。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import { useToastStore } from '../stores/toast'
import type { FileEntryVO } from '../types/api'
import { fmtBytes } from '../utils/num'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'
import DocPreviewDrawer from '../components/doc/DocPreviewDrawer.vue'

const { t } = useI18n()
const toast = useToastStore()

const cwd = ref('')
const entries = ref<FileEntryVO[]>([])
const loading = ref(true)
const error = ref('')
const previewPath = ref('')

/** 可就地预览的扩展名：其余只列出来，不给预览入口。 */
const PREVIEWABLE = [
  '.pdf', '.docx', '.doc', '.xlsx', '.xls', '.csv', '.md', '.txt', '.json',
]

function isPreviewable(name: string): boolean {
  const lower = name.toLowerCase()
  return PREVIEWABLE.some((ext) => lower.endsWith(ext))
}

const dirs = computed(() => entries.value.filter((e) => e.entry_is_dir))
const files = computed(() => entries.value.filter((e) => !e.entry_is_dir))

/** 面包屑：按当前路径切段。 */
const crumbs = computed(() => {
  const parts = cwd.value.split('/').filter(Boolean)
  return [{ label: t('files.root'), path: '' }, ...parts.map((p, i) => ({
    label: p,
    path: parts.slice(0, i + 1).join('/'),
  }))]
})

async function load(path: string) {
  loading.value = true
  error.value = ''
  try {
    const r = await api.files.list(path)
    cwd.value = r.resp_path
    entries.value = r.resp_entries ?? []
  } catch (e) {
    error.value = (e as Error)?.message || t('common.loadFailed', { msg: '' })
    entries.value = []
  } finally {
    loading.value = false
  }
}

function openEntry(e: FileEntryVO) {
  if (e.entry_is_dir) {
    void load(e.entry_path)
    return
  }
  if (!isPreviewable(e.entry_name)) {
    toast.info(t('files.noPreview', { name: e.entry_name }))
    return
  }
  previewPath.value = e.entry_path
}

onMounted(() => {
  void load('')
})

watch(cwd, () => {
  /* 路径变化由 load 主动驱动，这里不重复拉取 */
})
</script>

<template>
  <div class="view">
    <PageHeader :title="t('files.title')" :subtitle="t('files.subtitle')">
      <template #actions>
        <el-button size="small" :loading="loading" @click="load(cwd)">
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('common.refresh') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <nav class="crumbs">
        <button
          v-for="(c, i) in crumbs"
          :key="c.path"
          type="button"
          class="crumb"
          :class="{ 'is-last': i === crumbs.length - 1 }"
          @click="load(c.path)"
        >
          {{ c.label }}
        </button>
      </nav>

      <div v-if="error" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="alert" size="ic-xl" /></span>
        <b>{{ t('common.loadFailed', { msg: '' }) }}</b>
        <p>{{ error }}</p>
      </div>

      <div v-else-if="loading && !entries.length" class="wb-card pad">
        <el-skeleton animated :rows="6" />
      </div>

      <div v-else-if="!entries.length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="folder" size="ic-xl" /></span>
        <b>{{ t('files.empty') }}</b>
      </div>

      <template v-else>
        <ul v-if="dirs.length" class="list">
          <li v-for="d in dirs" :key="d.entry_path">
            <button type="button" class="row" @click="openEntry(d)">
              <span class="ico is-dir"><AppIcon name="folder" size="ic-sm" /></span>
              <span class="name">{{ d.entry_name }}</span>
              <span class="size" />
              <AppIcon name="chevron-right" size="ic-xs" class="go" />
            </button>
          </li>
        </ul>

        <ul v-if="files.length" class="list mt-3">
          <li v-for="f in files" :key="f.entry_path">
            <button type="button" class="row" @click="openEntry(f)">
              <span class="ico"><AppIcon name="doc" size="ic-sm" /></span>
              <span class="name">{{ f.entry_name }}</span>
              <span class="size">{{ fmtBytes(f.entry_size) }}</span>
              <span class="go-space" />
            </button>
          </li>
        </ul>
      </template>
    </div>

    <DocPreviewDrawer v-model="previewPath" />
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
}
.crumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-1);
  margin-bottom: var(--wb-sp-3);
}
.crumb {
  padding: 3px var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-muted);
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover {
  background: var(--wb-elev-hover);
  color: var(--wb-ink);
}
.crumb:active {
  transform: scale(0.96);
}
.crumb.is-last {
  color: var(--wb-ink);
}
.list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.row:hover {
  background: var(--wb-elev-hover);
  transform: translateX(2px);
}
.row:active {
  transform: translateX(2px) scale(0.995);
}
.ico {
  flex: none;
  display: flex;
  color: var(--wb-muted);
}
.ico.is-dir {
  color: var(--wb-primary);
}
.name {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.size {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.go-space {
  width: calc(17px * var(--wb-vw-scale));
}
.go {
  flex: none;
  color: var(--wb-muted);
}
.pad {
  padding: var(--wb-sp-4);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
