<script setup lang="ts">
// 文件面板：工作区目录树 + 单文件预览。
// 只读：写文件是助手的事，界面有第二个写入口，「谁改的这一行」就说不清了。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { FileEntry, FileRead } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

// 逐级路径栈：栈里每一项都是相对工作目录的目录路径（'' 是根）。
const stack = ref<string[]>([''])
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const truncated = ref(false)

const preview = ref<FileRead | null>(null)
const previewOpen = ref(false)
const previewLoading = ref(false)

const current = computed(() => stack.value[stack.value.length - 1] ?? '')
const crumbs = computed(() => {
  const parts = current.value.split('/').filter(Boolean)
  const out: { label: string; path: string }[] = [{ label: app.workspace.split(/[\\/]/).pop() || '/', path: '' }]
  let acc = ''
  for (const part of parts) {
    acc = acc ? `${acc}/${part}` : part
    out.push({ label: part, path: acc })
  }
  return out
})

function join(name: string): string {
  return current.value ? `${current.value}/${name}` : name
}

async function load(path = current.value) {
  loading.value = true
  try {
    const data = await api.files(path)
    entries.value = data.entries ?? []
    truncated.value = data.truncated
    stack.value = path === '' ? [''] : path.split('/')
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function enter(entry: FileEntry) {
  if (entry.entry_is_dir) {
    await load(join(entry.entry_name))
    return
  }
  await openFile(join(entry.entry_name))
}

async function up() {
  const parts = current.value.split('/').filter(Boolean)
  parts.pop()
  await load(parts.join('/'))
}

async function openFile(path: string) {
  previewOpen.value = true
  previewLoading.value = true
  preview.value = null
  try {
    preview.value = await api.fileRead(path)
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
    previewOpen.value = false
  } finally {
    previewLoading.value = false
  }
}

function formatSize(n: number): string {
  if (!n) return ''
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(1)}M`
  if (n >= 1024) return `${(n / 1024).toFixed(0)}k`
  return `${n}B`
}

onMounted(() => {
  void load('')
})

defineExpose({ load, openFile })
</script>

<template>
  <section class="files card">
    <header class="files-head">
      <button class="wb-btn icon ghost sm" :disabled="!current" :title="t('files.up')" @click="up">
        <AppIcon name="arrow-left" :size="13" />
      </button>
      <div class="crumbs">
        <button
          v-for="(crumb, index) in crumbs"
          :key="crumb.path"
          class="crumb"
          :class="{ last: index === crumbs.length - 1 }"
          @click="load(crumb.path)"
        >
          {{ crumb.label }}
        </button>
      </div>
      <button class="wb-btn icon ghost sm" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load()">
        <AppIcon name="refresh" :size="13" />
      </button>
    </header>

    <div class="files-body">
      <div v-if="!entries.length && !loading" class="files-empty">{{ t('files.empty') }}</div>
      <button
        v-for="entry in entries"
        :key="entry.entry_name"
        class="file-row wb-row-click"
        @click="enter(entry)"
      >
        <AppIcon :name="entry.entry_is_dir ? 'folder' : 'file'" :size="13" />
        <span class="file-name">{{ entry.entry_name }}</span>
        <span v-if="entry.entry_is_dir" class="file-size">›</span>
        <span v-else class="file-size">{{ formatSize(entry.size_bytes ?? 0) }}</span>
      </button>
      <p v-if="truncated" class="files-note">{{ t('files.truncated') }}</p>
    </div>

    <el-drawer v-model="previewOpen" :title="preview?.file_path ?? ''" size="560px" direction="rtl">
      <div v-if="previewLoading" class="preview-state">
        <span class="dot busy" /> {{ t('common.loading') }}
      </div>
      <div v-else-if="preview?.is_binary" class="preview-state">{{ t('files.binary') }}</div>
      <template v-else-if="preview">
        <p v-if="preview.truncated" class="files-note">
          {{ t('files.previewTruncated', { size: formatSize(preview.size_bytes) }) }}
        </p>
        <pre class="preview selectable">{{ preview.file_text }}</pre>
      </template>
    </el-drawer>
  </section>
</template>

<style scoped>
.files {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.files-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-2);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}
.crumbs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 2px;
  overflow: hidden;
}
.crumb {
  border: none;
  background: transparent;
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  padding: 2px 4px;
  border-radius: var(--wb-r-xs);
  cursor: pointer;
  white-space: nowrap;
  transition: color var(--wb-dur-fast) var(--wb-ease);
}
.crumb:hover {
  color: var(--wb-brand);
}
.crumb.last {
  color: var(--wb-ink);
}
.crumb::after {
  content: '/';
  color: var(--wb-faint);
  margin-left: 4px;
}
.crumb.last::after {
  content: '';
}
.files-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--wb-sp-1);
}
.file-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 4px var(--wb-sp-2);
  border: none;
  background: transparent;
  color: var(--wb-ink);
  font-size: var(--wb-fs-xs);
  text-align: left;
  cursor: pointer;
}
.file-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-size {
  color: var(--wb-faint);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  flex: none;
}
.files-empty,
.files-note,
.preview-state {
  padding: var(--wb-sp-3);
  color: var(--wb-faint);
  font-size: var(--wb-fs-2xs);
  text-align: center;
}
.preview {
  margin: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
