<script setup lang="ts">
// 文件面板：工作区目录树 + 单文件编辑。
// 浏览与轻量编辑（新建 / 改名 / 删除 / 保存文本）在这里；批量改文件是助手的事，
// 界面动手之前都先落在「看得见的单个目标」上，改动范围一眼可见。
// 点文件只往上抛一个「选中」，正文由右侧那一栏显示——同一份内容不在两处画。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmDanger } from '@/utils/confirm'
import { ElMessageBox } from 'element-plus'
import AppIcon from '@/components/common/AppIcon.vue'
import WbEmpty from '@/components/common/WbEmpty.vue'
import FilePreviewDrawer from '@/components/workspace/FilePreviewDrawer.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useToasts } from '@/composables/useToasts'
import type { FileEntry, FileRead } from '@/types/api'

const { t } = useI18n()
const app = useAppStore()
const toasts = useToasts()

const props = defineProps<{
  /** 当前选中的文件路径（相对工作目录）：这一行给个选中标记 */
  selected?: string
}>()

const emit = defineEmits<{
  /** 选中了一个文件（相对工作目录的路径）：右侧那一栏据此显示正文 */
  pick: [path: string]
  /** 文件被保存过：右侧那一栏据此重读一次 */
  saved: []
}>()

// 逐级路径栈：栈里每一项都是相对工作目录的目录路径（'' 是根）。
const stack = ref<string[]>([''])
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const truncated = ref(false)
// 行内动作进行中的条目名：按钮转圈、行不可重复点
const busyEntry = ref('')
// 新建（文件 / 目录）进行中的种类：页头按钮转圈、防连点
const busyCreate = ref('')

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
  // 只往上抛选中：正文在右侧那一栏画，树上不重复画一份。
  emit('pick', join(entry.entry_name))
}

async function up() {
  const parts = current.value.split('/').filter(Boolean)
  parts.pop()
  await load(parts.join('/'))
}

/** 打开编辑抽屉：正文已经在右侧那一栏看过了，这里只为「改」。 */
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

/** 弹一个带校验的输入框；取消给 null，空串或非法名不给确认。 */
async function promptText(titleKey: string, placeholderKey: string, initial = ''): Promise<string | null> {
  try {
    const res = await ElMessageBox.prompt(t(placeholderKey), t(titleKey), {
      inputValue: initial,
      confirmButtonText: t('common.save'),
      cancelButtonText: t('common.cancel'),
      inputPattern: /^[^\\/:*?"<>|]+$/,
      inputErrorMessage: t('files.nameInvalid'),
    })
    return (res.value ?? '').trim()
  } catch {
    return null
  }
}

async function newFile() {
  const name = await promptText('files.newFileTitle', 'files.newFilePlaceholder')
  if (!name) return
  busyCreate.value = 'file'
  try {
    // 空正文建空文件：父目录不存在由后端一并建出。
    await api.fileSave(join(name), '')
    toasts.push('ok', t('common.saved'))
    await load()
    // 建完就选中它：用户下一步多半是往里写。
    emit('pick', join(name))
    await openFile(join(name))
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyCreate.value = ''
  }
}

async function newDir() {
  const name = await promptText('files.newDirTitle', 'files.newDirPlaceholder')
  if (!name) return
  busyCreate.value = 'dir'
  try {
    await api.fileMkdir(join(name))
    toasts.push('ok', t('common.saved'))
    await load()
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    busyCreate.value = ''
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
  const ok = await confirmDanger(
    t('files.deleteConfirm', { name: entry.entry_name }),
    t('files.deleteTitle'),
    t('common.delete'),
    t('common.cancel'),
  )
  if (!ok) return
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

// 抽屉保存成功：重拉目录（大小与修改时间变了）并重读这份文件，
// 让抽屉里的正文是后端规范化之后的那一份；右侧那一栏也重读一次。
async function onPreviewSaved(): Promise<void> {
  if (preview.value) {
    try {
      preview.value = await api.fileRead(preview.value.file_path)
    } catch {
      /* 读不到就留着旧正文：文件确实在，只是这一次读失败 */
    }
  }
  await load()
  emit('saved')
}

onMounted(() => {
  void load('')
})

defineExpose({ load, openFile })
</script>

<template>
  <section class="files card">
    <header class="files-head">
      <button class="wb-btn icon ghost sm" type="button" :disabled="!current" :title="t('files.up')" :aria-label="t('files.up')" @click="up">
        <AppIcon name="arrow-left" :size="13" />
      </button>
      <div class="crumbs">
        <button
          v-for="(crumb, index) in crumbs"
          :key="crumb.path"
          type="button"
          class="crumb"
          :class="{ last: index === crumbs.length - 1 }"
          @click="load(crumb.path)"
        >
          {{ crumb.label }}
        </button>
      </div>
      <button class="wb-btn icon ghost sm" type="button" :class="{ 'is-loading': busyCreate === 'file' }" :disabled="busyCreate !== ''" :title="t('files.newFileTitle')" :aria-label="t('files.newFileTitle')" @click="newFile">
        <AppIcon name="plus" :size="13" />
      </button>
      <button class="wb-btn icon ghost sm" type="button" :class="{ 'is-loading': busyCreate === 'dir' }" :disabled="busyCreate !== ''" :title="t('files.newDirTitle')" :aria-label="t('files.newDirTitle')" @click="newDir">
        <AppIcon name="folder" :size="13" />
      </button>
      <button class="wb-btn icon ghost sm" type="button" :class="{ 'is-loading': loading }" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="load()">
        <AppIcon name="refresh" :size="13" />
      </button>
    </header>

    <div class="files-body">
      <WbEmpty v-if="!entries.length && !loading" :title="t('files.empty')" :hint="t('files.emptyHint')">
        <template #action>
          <button
            class="wb-btn primary sm"
            type="button"
            :class="{ 'is-loading': busyCreate === 'file' }"
            :disabled="busyCreate !== ''"
            @click="newFile()"
          >
            {{ t('files.newFileTitle') }}
          </button>
        </template>
      </WbEmpty>
      <div
        v-for="entry in entries"
        :key="entry.entry_name"
        class="file-row wb-row-click"
        :class="{ 'is-active': !entry.entry_is_dir && props.selected === join(entry.entry_name) }"
        @click="enter(entry)"
      >
        <AppIcon :name="entry.entry_is_dir ? 'folder' : 'file'" :size="13" />
        <span class="file-name">{{ entry.entry_name }}</span>
        <span class="row-actions" @click.stop>
          <button
            class="row-btn"
            type="button"
            :title="t('files.renameTitle')"
            :aria-label="`${t('files.renameTitle')} ${entry.entry_name}`"
            :disabled="busyEntry === entry.entry_name"
            @click="renameEntry(entry)"
          >
            <AppIcon name="edit" :size="12" />
          </button>
          <button
            class="row-btn"
            type="button"
            :title="t('common.delete')"
            :aria-label="`${t('common.delete')} ${entry.entry_name}`"
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

    <FilePreviewDrawer
      v-model:open="previewOpen"
      :file="preview"
      :loading="previewLoading"
      @saved="onPreviewSaved"
    />
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
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
/* 面包屑是小控件：悬停放大、按下缩小，与全站按钮同一组刻度 */
.crumb:hover {
  color: var(--wb-brand);
  transform: scale(var(--wb-press-hover));
}
.crumb:active {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
}
.crumb.last {
  color: var(--wb-ink);
}
.crumb::after {
  content: '/';
  color: var(--wb-faint);
  margin-left: var(--wb-sp-1);
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
/* 选中的文件：淡底 + 左缘竖条。与 git 改动清单的选中语言一致。 */
.file-row.is-active {
  background: var(--wb-selected);
  box-shadow: inset 2px 0 0 var(--wb-brand);
}
/* 行内动作平时藏着，悬停那行才亮：满屏按钮比没有按钮更吵。
   键盘用户靠 :focus-within 也能看到——只给鼠标的入口等于没有入口。 */
.row-actions {
  display: none;
  align-items: center;
  gap: 2px;
  flex: none;
}
.file-row:hover .row-actions,
.file-row:focus-within .row-actions {
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
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

/* 行内小按钮：悬停放大、按下缩小，刻度复用全站令牌 */
.row-btn:hover:not(:disabled) {
  color: var(--wb-brand);
  background: var(--wb-hover);
  transform: scale(var(--wb-press-hover));
}
.row-btn:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
  transition-duration: 60ms;
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
</style>
