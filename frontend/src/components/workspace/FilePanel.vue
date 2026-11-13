<script setup lang="ts">
// 文件面板：工作区目录树 + 单文件预览/编辑。
// 浏览与轻量编辑（新建 / 改名 / 删除 / 保存文本）在这里；批量改文件是助手的事，
// 界面动手之前都先落在「看得见的单个目标」上，改动范围一眼可见。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import FilePreview from '@/components/workspace/FilePreview.vue'
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
// 行内动作进行中的条目名：按钮转圈、行不可重复点
const busyEntry = ref('')

const preview = ref<FileRead | null>(null)
const previewOpen = ref(false)
const previewLoading = ref(false)
// 预览即编辑：开编辑后正文进 textarea，保存走 /files/save
const editing = ref(false)
const editText = ref('')
const savingEdit = ref(false)
// 编辑后未保存就关抽屉的提示
const dirty = computed(() => editing.value && preview.value != null && editText.value !== preview.value.file_text)

const current = computed(() => stack.value[stack.value.length - 1] ?? '')
// 可视化预览只认这几类；认不出的二进制仍走占位。
const isPreviewable = computed(() =>
  ['pdf', 'sheet', 'doc', 'image'].includes(preview.value?.file_kind ?? ''),
)
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
  editing.value = false
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

/** 弹一个输入框；取消给 null，空串当没填。 */
async function promptText(titleKey: string, placeholderKey: string, initial = ''): Promise<string | null> {
  try {
    const res = await ElMessageBox.prompt(t(placeholderKey), t(titleKey), {
      inputValue: initial,
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
    })
    return (res.value ?? '').trim()
  } catch {
    return null
  }
}

async function newFile() {
  const name = await promptText('files.newFileTitle', 'files.newFilePlaceholder')
  if (!name) return
  try {
    // 空正文建空文件：父目录不存在由后端一并建出。
    await api.fileSave(join(name), '')
    toasts.push('ok', t('common.saved'))
    await load()
    await openFile(join(name))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function newDir() {
  const name = await promptText('files.newDirTitle', 'files.newDirPlaceholder')
  if (!name) return
  try {
    await api.fileMkdir(join(name))
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  }
}

async function renameEntry(entry: FileEntry) {
  const name = await promptText('files.renameTitle', 'files.renamePlaceholder', entry.entry_name)
  if (!name || name === entry.entry_name) return
  busyEntry.value = entry.entry_name
  try {
    await api.fileRename(join(entry.entry_name), join(name))
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyEntry.value = ''
  }
}

async function removeEntry(entry: FileEntry) {
  try {
    await ElMessageBox.confirm(t('files.deleteConfirm', { name: entry.entry_name }), t('files.deleteTitle'), {
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  busyEntry.value = entry.entry_name
  try {
    await api.fileDelete(join(entry.entry_name))
    // 删的正是预览中的文件（或它的上级）时，抽屉跟着关，不留死预览。
    if (preview.value && preview.value.file_path.includes(entry.entry_name)) previewOpen.value = false
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyEntry.value = ''
  }
}

function startEdit() {
  if (!preview.value || preview.value.is_binary) return
  editText.value = preview.value.file_text
  editing.value = true
}

async function saveEdit() {
  if (!preview.value) return
  savingEdit.value = true
  try {
    await api.fileSave(preview.value.file_path, editText.value)
    preview.value = await api.fileRead(preview.value.file_path)
    editing.value = false
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    savingEdit.value = false
  }
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
      <button class="wb-btn icon ghost sm" :title="t('files.newFileTitle')" @click="newFile">
        <AppIcon name="plus" :size="13" />
      </button>
      <button class="wb-btn icon ghost sm" :title="t('files.newDirTitle')" @click="newDir">
        <AppIcon name="folder" :size="13" />
      </button>
      <button class="wb-btn icon ghost sm" :class="{ 'is-loading': loading }" :title="t('common.refresh')" @click="load()">
        <AppIcon name="refresh" :size="13" />
      </button>
    </header>

    <div class="files-body">
      <div v-if="!entries.length && !loading" class="files-empty">{{ t('files.empty') }}</div>
      <div v-for="entry in entries" :key="entry.entry_name" class="file-row wb-row-click" @click="enter(entry)">
        <AppIcon :name="entry.entry_is_dir ? 'folder' : 'file'" :size="13" />
        <span class="file-name">{{ entry.entry_name }}</span>
        <span class="row-actions" @click.stop>
          <button
            class="row-btn"
            :title="t('files.renameTitle')"
            :disabled="busyEntry === entry.entry_name"
            @click="renameEntry(entry)"
          >
            <AppIcon name="edit" :size="12" />
          </button>
          <button
            class="row-btn"
            :title="t('common.delete')"
            :disabled="busyEntry === entry.entry_name"
            @click="removeEntry(entry)"
          >
            <AppIcon name="trash" :size="12" />
          </button>
        </span>
        <span v-if="entry.entry_is_dir" class="file-size">›</span>
        <span v-else class="file-size">{{ formatSize(entry.size_bytes ?? 0) }}</span>
      </div>
      <p v-if="truncated" class="files-note">{{ t('files.truncated') }}</p>
    </div>

    <el-drawer v-model="previewOpen" :title="preview?.file_path ?? ''" size="560px" direction="rtl">
      <div v-if="previewLoading" class="preview-state">
        <span class="dot busy" /> {{ t('common.loading') }}
      </div>
      <div v-else-if="preview?.is_binary && isPreviewable" class="binary-preview">
        <FilePreview :file="preview" />
      </div>
      <div v-else-if="preview?.is_binary" class="preview-state">{{ t('files.binary') }}</div>
      <template v-else-if="preview">
        <p v-if="preview.truncated" class="files-note">
          {{ t('files.previewTruncated', { size: formatSize(preview.size_bytes) }) }}
        </p>
        <p v-if="dirty" class="files-note edited">{{ t('files.editedHint') }}</p>
        <el-input
          v-if="editing"
          v-model="editText"
          type="textarea"
          :rows="22"
          class="edit-area"
          :autosize="{ minRows: 22 }"
        />
        <pre v-else class="preview selectable">{{ preview.file_text }}</pre>
      </template>
      <template #footer>
        <div v-if="preview && !preview.is_binary && !previewLoading" class="preview-actions">
          <template v-if="editing">
            <button class="wb-btn sm" :disabled="savingEdit" @click="editing = false">
              {{ t('common.cancel') }}
            </button>
            <button
              class="wb-btn sm primary"
              :class="{ 'is-loading': savingEdit }"
              :disabled="savingEdit"
              @click="saveEdit"
            >
              {{ t('common.save') }}
            </button>
          </template>
          <button v-else class="wb-btn sm primary" @click="startEdit">
            <AppIcon name="edit" :size="13" />
            {{ t('files.editFile') }}
          </button>
        </div>
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
  color: var(--wb-ink);
  font-size: var(--wb-fs-xs);
  cursor: pointer;
}
.file-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 行内动作平时藏着，悬停那行才亮：满屏按钮比没有按钮更吵 */
.row-actions {
  display: none;
  align-items: center;
  gap: 2px;
  flex: none;
}
.file-row:hover .row-actions {
  display: inline-flex;
}
.row-btn {
  border: none;
  background: none;
  color: var(--wb-mute);
  cursor: pointer;
  padding: 1px 3px;
  border-radius: var(--wb-r-xs);
  display: inline-flex;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    background-color var(--wb-dur-fast) var(--wb-ease);
}
.row-btn:hover:not(:disabled) {
  color: var(--wb-brand);
  background: var(--wb-hover);
}
.row-btn:disabled {
  opacity: 0.5;
  cursor: default;
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
.files-note.edited {
  color: var(--wb-warn);
}
.preview {
  margin: 0;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}
/* 编辑用等宽字体：这里改的是代码/配置文件，比例字体对不齐缩进 */
.edit-area :deep(.el-textarea__inner) {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.6;
}

/* 二进制预览（图片）：居中摆放，窄图不贴左缘 */
.binary-preview {
  display: flex;
  justify-content: center;
}

.preview-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
</style>
