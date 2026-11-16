<script setup lang="ts">
// 文件预览抽屉：只读预览 / 就地编辑 / 二进制走 FilePreview。
// 从 FilePanel 拆出来守住单文件行数上限：抽屉的状态与护栏（未保存确认）
// 跟着它一起走，父组件只剩文件树与导航。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmDanger } from '@/utils/confirm'
import FilePreview from '@/components/workspace/FilePreview.vue'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { FileRead } from '@/types/api'

const props = defineProps<{
  open: boolean
  file: FileRead | null
  loading: boolean
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  /** 保存成功后父组件要重拉目录（大小 / 修改时间会变） */
  saved: []
}>()

const { t } = useI18n()
const toasts = useToasts()

const editing = ref(false)
const editText = ref('')
const saving = ref(false)

/** 可视化预览只认这几类；认不出的二进制仍走占位。 */
const isPreviewable = computed(() =>
  ['pdf', 'sheet', 'doc', 'image'].includes(props.file?.file_kind ?? ''),
)

/** 文件名（路径末段）：确认框里要能认出是哪一份。 */
const fileName = computed(() => {
  const path = props.file?.file_path ?? ''
  return path.split(/[\\/]/).filter(Boolean).pop() || path
})

const dirty = computed(() => editing.value && props.file != null && editText.value !== props.file.file_text)

// 换文件就退出编辑态：留着上一条的草稿会让人以为改的是这一条。
watch(
  () => props.file?.file_path,
  () => {
    editing.value = false
    editText.value = props.file?.file_text ?? ''
  },
)

function formatSize(n: number): string {
  if (!n) return ''
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(1)}M`
  if (n >= 1024) return `${(n / 1024).toFixed(0)}k`
  return `${n}B`
}

function startEdit(): void {
  if (!props.file || props.file.is_binary) return
  editText.value = props.file.file_text
  editing.value = true
}

async function save(): Promise<void> {
  if (!props.file || saving.value) return
  saving.value = true
  try {
    await api.fileSave(props.file.file_path, editText.value)
    editing.value = false
    toasts.push('ok', t('common.saved'))
    // 保存后重新读一遍：正文可能被后端规范化过（换行 / 编码）。
    emit('saved')
  } catch (err) {
    toasts.push('error', err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

// 关抽屉：干净就直接关；脏就先确认，取消则留在编辑态。
async function requestClose(open: boolean): Promise<void> {
  if (open) {
    emit('update:open', true)
    return
  }
  if (dirty.value) {
    const ok = await confirmDanger(
      t('files.closeDirtyBody', { name: fileName.value }),
      t('files.closeDirtyTitle'),
      t('common.discardChanges'),
      t('common.keepEditing'),
    )
    if (!ok) return
  }
  editing.value = false
  emit('update:open', false)
}
</script>

<template>
  <el-drawer
    :model-value="props.open"
    :title="props.file?.file_path ?? ''"
    size="560px"
    direction="rtl"
    @update:model-value="requestClose"
  >
    <div v-if="props.loading" class="preview-state">
      <span class="dot busy" /> {{ t('common.loading') }}
    </div>
    <div v-else-if="props.file?.is_binary && isPreviewable" class="binary-preview">
      <FilePreview :file="props.file" />
    </div>
    <div v-else-if="props.file?.is_binary" class="preview-state">{{ t('files.binary') }}</div>
    <template v-else-if="props.file">
      <p v-if="props.file.truncated" class="files-note">
        {{ t('files.previewTruncated', { size: formatSize(props.file.size_bytes) }) }}
      </p>
      <p v-if="dirty" class="files-note edited">{{ t('files.editedHint') }}</p>
      <el-input
        v-if="editing"
        v-model="editText"
        type="textarea"
        :rows="22"
        class="mono"
        :autosize="{ minRows: 22 }"
      />
      <pre v-else class="preview selectable">{{ props.file.file_text }}</pre>
    </template>
    <template #footer>
      <div v-if="props.file && !props.file.is_binary && !props.loading" class="preview-actions">
        <template v-if="editing">
          <button type="button" class="wb-btn sm" :disabled="saving" @click="editing = false">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="wb-btn sm primary" :class="{ 'is-loading': saving }" :disabled="saving" @click="save">
            {{ t('common.save') }}
          </button>
        </template>
        <button type="button" v-else class="wb-btn sm primary" @click="startEdit">
          {{ t('files.editFile') }}
        </button>
      </div>
    </template>
  </el-drawer>
</template>

<style scoped>
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
