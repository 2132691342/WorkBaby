<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { BookOpen, Upload, Search, Plus, Eye, Pencil, Trash2 } from '@/components/common/icons'
import { OpenFileDialog } from '@/wailsjs/go/main/App'
import { apiGet, apiPost } from '@/api/client'
import { useKnowledgeDocsStore } from '@/stores/kdocs'
import { t } from '@/i18n'
import { formatDateTime } from '@/utils/time'
import { useToast } from '@/composables/useToast'
import type { KnowledgeDoc } from '@/types/api'

/**
 * 知识文档库。
 *
 * <p>文档 = 知识库条目（name + source + source_type + 索引状态）。
 * 注意：顶部 header / 工具栏始终渲染，避免初次加载或异常时整页空白（截图复现）。
 */
const kdocs = useKnowledgeDocsStore()
const { docs, groups, filterGroup, search, error, showCreate, form, editing } = storeToRefs(kdocs)
const { load, startEdit, doDelete, doSave } = kdocs

/** source_type 选项（中英 i18n 双语，避免硬编码英文给中文用户）。 */
const SOURCE_TYPES = computed(() => [
  { value: 'text', label: t('kdoc.sourceType.text') },
  { value: 'file', label: t('kdoc.sourceType.file') },
  { value: 'url', label: t('kdoc.sourceType.url') }
])

/** 初始加载与失败重试按钮共用。 */
const loading = ref(false)
async function reload(): Promise<void> {
  loading.value = true
  try {
    await load()
  } finally {
    loading.value = false
  }
}

onMounted(reload)

/** 提交态：锁定弹窗关闭与重复提交。 */
const saving = ref(false)

/** 表单校验错误（行内提示，不再只靠 toast）。 */
const formError = ref('')

/** 弹窗关闭。 */
function closeDialog(): void {
  if (saving.value) return
  showCreate.value = false
  editing.value = null
  formError.value = ''
}

/** 保存（新建 / 编辑共用 store.doSave）。 */
async function submit(): Promise<void> {
  if (!form.value.name.trim()) {
    formError.value = t('kdoc.errRequired')
    return
  }
  if (!form.value.source.trim()) {
    formError.value = t('kdoc.errRequired')
    return
  }
  formError.value = ''
  saving.value = true
  try {
    await doSave()
    if (!kdocs.error) closeDialog()
  } finally {
    saving.value = false
  }
}

/** 来源类型切换时给出对应占位与提示。 */
const sourcePlaceholder = computed(() =>
  form.value.source_type === 'file'
    ? t('kdoc.sourcePlaceholderFile')
    : form.value.source_type === 'url'
      ? 'https://example.com/docs/getting-started'
      : t('kdoc.sourcePlaceholder')
)

const sourceTypeHint = computed(() => {
  switch (form.value.source_type) {
    case 'file':
      return t('kdoc.emptyHintFile')
    case 'url':
      return t('kdoc.emptyHintUrl')
    default:
      return t('kdoc.emptyHintText')
  }
})

/** 打开新建对话框。 */
function openCreate(): void {
  openCreateWithType('text')
}

/** 按 source 类型打开新建对话框（空状态卡片点击）。 */
function openCreateWithType(sourceType: 'text' | 'file' | 'url'): void {
  editing.value = null
  form.value = { name: '', source: '', source_type: sourceType }
  showCreate.value = true
}

const toast = useToast()

// ===== 查看详情：列表只有增删改，用户需要不进入编辑即可核对文档元数据 =====
const viewing = ref<KnowledgeDoc | null>(null)
function openView(row: KnowledgeDoc): void {
  viewing.value = row
}

/** 从文件取 basename（去扩展名），作为文档默认名。 */
function basenameNoExt(p: string): string {
  const seg = p.replace(/\\/g, '/').split('/').pop() ?? ''
  return seg.replace(/\.[^.]+$/, '') || ''
}

/** 字节数 → 人类可读（KB / MB），原型表格大小列展示。 */
function formatSize(bytes?: number | null): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

/** 列表「来源」列展示：file 型只显示文件名（源文件已受管复制到知识库目录，不向用户暴露磁盘路径）。
 *  text/url 仍显示原文/URL，便于识别。 */
function displaySource(d: KnowledgeDoc): string {
  if (d.source_type === 'file') {
    return d.source.replace(/\\/g, '/').split('/').pop() ?? d.source
  }
  return d.source
}

/** file 型文档：原生文件对话框选路径 → 回填 source，免手动粘贴路径。 */
async function pickSourceFile(): Promise<void> {
  try {
    const selected = await OpenFileDialog(t('kdoc.pickFileTitle'), DOC_FILTER)
    if (!selected) return
    form.value.source = selected
    if (!form.value.name?.trim()) {
      form.value.name = basenameNoExt(selected)
    }
  } catch (e) {
    toast.error(t('kdoc.pickFileFailed'), e instanceof Error ? e.message : String(e))
  }
}

// ===== 导入文件到受管知识库（后端复制进 {home}/knowledge 并后台索引）=====
const importingFile = ref(false)
/** 导入进度文案：上传中 → 后台索引中… */
const importStatus = ref('')

/** 与后端 managedDocExts 白名单一致：pdf / html / txt / json / yaml / xml / md / csv */
const DOC_FILTER = '*.pdf;*.html;*.htm;*.txt;*.md;*.markdown;*.csv;*.json;*.yaml;*.yml;*.xml'

/** 导入 + 状态轮询：完成后自动刷新列表。 */
async function importManagedFile(): Promise<void> {
  try {
    const selected = await OpenFileDialog(t('kdoc.pickFileTitle'), DOC_FILTER)
    if (!selected) return
    importingFile.value = true
    importStatus.value = t('kdoc.importCopying')
    const doc = await apiPost<KnowledgeDoc>('/api/v1/kdocs/import-file', {
      name: basenameNoExt(selected),
      source_path: selected,
      folder_id: null
    })
    toast.success(t('kdoc.importDone', doc?.name ?? basenameNoExt(selected)))
    importStatus.value = t('kdoc.importIndexing')
    await reload()
    // 后台索引状态轮询（pending/parsing → indexed/failed 后刷新一次）
    const id = doc?.id
    if (!id) return
    const start = Date.now()
    let last = doc.status ?? 'pending'
    while ((last === 'pending' || last === 'parsing') && Date.now() - start < 90_000) {
      await new Promise((r) => setTimeout(r, 1200))
      try {
        const cur = await apiGet<KnowledgeDoc>(`/api/v1/kdocs/${id}`)
        if (cur && cur.status !== last) {
          last = cur.status
          if (cur.status === 'indexed') {
            importStatus.value = t('kdoc.importIndexed', cur.chunk_count)
            toast.success(t('kdoc.importIndexed', cur.chunk_count))
          } else if (cur.status === 'failed') {
            importStatus.value = ''
            toast.error(t('kdoc.importFailed'), cur.error_msg || '')
          }
          await reload()
        }
      } catch {
        /* 单次查询失败不中断轮询 */
      }
    }
  } catch (e) {
    toast.error(t('kdoc.importError'), e instanceof Error ? e.message : String(e))
  } finally {
    importingFile.value = false
    importStatus.value = ''
  }
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap" style="max-width: 1000px">
      <!-- Hero -->
      <header class="hero">
        <div class="tile"><BookOpen class="ic" /></div>
        <div>
          <h1>{{ t('kdoc.title') }}</h1>
          <p>{{ t('kdoc.count', docs.length, groups.length) }}</p>
        </div>
        <span class="sp" />
        <span v-if="importStatus" class="flex-r fs11 muted">
          <span class="led g" />
          {{ importStatus }}
        </span>
        <button class="btn" :disabled="importingFile" @click="importManagedFile">
          <Upload class="ic ic-sm" />
          {{ t('kdoc.importFile') }}
        </button>
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('kdoc.newDoc') }}
        </button>
      </header>

      <!-- 工具栏：搜索框弹性撑满 + 分组选择器定宽 + 按钮与提示不换行（窄屏允许整体换行） -->
      <div class="kdoc-toolbar">
        <div class="field-wrap kdoc-toolbar__search">
          <Search class="ic ic-sm" />
          <input v-model="search" class="input with-icon" :placeholder="t('kdoc.searchPlaceholder')" @keyup.enter="reload" />
        </div>
        <select v-model="filterGroup" class="input narrow" @change="reload">
          <option value="">{{ t('kdoc.allGroups') }}</option>
          <option v-for="g in groups" :key="g" :value="g">{{ g }}</option>
        </select>
        <button class="btn" @click="reload">
          <Search class="ic ic-sm" />
          {{ t('knowledge.search') }}
        </button>
        <span class="fs11 muted">{{ t('kdoc.chunkHint') }}</span>
      </div>

      <div v-if="error" class="alert a-danger">
        <span>{{ error }}</span>
        <button class="btn btn-sm" @click="reload">{{ t('common.retry') }}</button>
      </div>

      <!-- 表格 -->
      <div class="card p-sm">
        <div v-if="loading && docs.length === 0" class="empty">{{ t('ui.status.loading') }}</div>
        <div v-else-if="docs.length === 0" class="empty">{{ t('kdoc.emptyTitle') }}</div>
        <div v-else class="tbl-wrap">
          <table class="tbl">
            <thead>
              <tr>
                <th style="min-width: 210px">{{ t('kdoc.titleField') }}</th>
                <th>{{ t('kdoc.sourceTypeLabel') }}</th>
                <th class="ta-r">{{ t('kdoc.sizeLabel') }}</th>
                <th class="ta-r">{{ t('kdoc.chunkCount') }}</th>
                <th>{{ t('kdoc.groupLabel') }}</th>
                <th>{{ t('kdoc.statusLabel') }}</th>
                <th class="ta-r">{{ t('memory.center.col.time') }}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in docs" :key="row.id">
                <td style="font-weight: 500">{{ row.name }}</td>
                <td>
                  <span class="badge" :class="row.source_type === 'file' ? 'b-neutral' : row.source_type === 'pdf' ? 'b-danger' : row.source_type === 'xlsx' ? 'b-info' : row.source_type === 'docx' ? 'b-primary' : 'b-neutral'">
                    {{ row.source_type }}
                  </span>
                </td>
                <td class="ta-r mono">{{ formatSize(row.size_bytes) }}</td>
                <td class="ta-r mono">{{ row.chunk_count }}</td>
                <td>{{ (row as Record<string, unknown>).group as string || '—' }}</td>
                <td>
                  <span class="badge" :class="`b-${row.status === 'indexed' ? 'success' : row.status === 'failed' ? 'danger' : row.status === 'indexing' ? 'warning' : 'neutral'}`">
                    <span class="dot" />{{ t(`kdoc.status.${row.status || 'pending'}`) }}
                  </span>
                </td>
                <td class="ta-r mono">{{ formatDateTime(row.updated_at) }}</td>
                <td>
                  <div class="tbl-actions">
                    <button class="btn-icon" :title="t('kdoc.viewDoc')" @click="openView(row)"><Eye class="ic ic-sm" /></button>
                    <button class="btn-icon" :title="t('ui.btn.edit')" @click="startEdit(row)"><Pencil class="ic ic-sm" /></button>
                    <button class="btn-icon" style="color: var(--wb-danger)" :title="t('ui.btn.delete')" @click="doDelete(row)"><Trash2 class="ic ic-sm" /></button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- 新建 / 编辑：与全局表单弹窗一致（720px + 28px 控件 + 12px 字号） -->
    <FormDialog
      v-model="showCreate"
      :title="editing ? t('kdoc.editDoc') : t('kdoc.newDocTitle')"
      :submitting="saving"
      @confirm="submit"
      @close="closeDialog"
    >
      <div class="wb-fgrid">
        <Field :label="t('kdoc.titleField')" required full :error="formError">
          <input v-model="form.name" class="input" :placeholder="t('kdoc.titleField')" />
        </Field>

        <Field :label="t('kdoc.sourceTypeLabel')" :hint="sourceTypeHint">
          <select v-model="form.source_type" class="input">
            <option v-for="st in SOURCE_TYPES" :key="st.value" :value="st.value">{{ st.label }}</option>
          </select>
        </Field>

        <Field :label="t('kdoc.sourceField')" required full>
          <div v-if="form.source_type === 'file'" class="kdoc-src">
            <input v-model="form.source" class="input" :placeholder="sourcePlaceholder" />
            <button class="btn" @click="pickSourceFile">
              <Upload class="ic ic-sm" />
              {{ t('kdoc.pickFile') }}
            </button>
          </div>
          <textarea
            v-else
            v-model="form.source"
            class="input"
            rows="6"
            :placeholder="sourcePlaceholder"
          />
        </Field>
      </div>
    </FormDialog>

    <!-- 详情：只读核对元数据 -->
    <el-dialog
      :model-value="viewing !== null"
      :title="viewing?.name ?? ''"
      width="640px"
      align-center
      class="wb-form-dialog"
      @update:model-value="viewing = null"
    >
      <div v-if="viewing" class="kv">
        <span>{{ t('kdoc.sourceTypeLabel') }}</span>
        <span>{{ viewing.source_type }}</span>
        <span>{{ t('kdoc.sourceField') }}</span>
        <span class="mono">{{ displaySource(viewing) }}</span>
        <span>{{ t('kdoc.sizeLabel') }}</span>
        <span class="mono">{{ formatSize(viewing.size_bytes) }}</span>
        <span>{{ t('kdoc.chunkCount') }}</span>
        <span class="mono">{{ viewing.chunk_count }}</span>
        <span>{{ t('kdoc.statusLabel') }}</span>
        <span>{{ t(`kdoc.status.${viewing.status || 'pending'}`) }}</span>
        <span>{{ t('kdoc.viewUpdatedAt') }}</span>
        <span class="mono">{{ formatDateTime(viewing.updated_at) }}</span>
      </div>
      <div v-if="viewing?.error_msg" class="alert a-danger mt10">{{ viewing.error_msg }}</div>
      <template #footer>
        <div class="wb-form-actions">
          <button class="btn" @click="viewing = null">{{ t('ui.btn.cancel') }}</button>
          <button class="btn btn-primary" @click="viewing && startEdit(viewing); viewing = null">
            {{ t('ui.btn.edit') }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.kdoc-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.kdoc-toolbar__search {
  flex: 1 1 220px;
  min-width: 0;
}
.kdoc-src {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
