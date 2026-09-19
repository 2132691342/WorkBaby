<script setup lang="ts">
import { ref, watch } from 'vue'
import { FolderSearch } from '@/components/common/icons'
import { pickDirectoryViaShell } from '@/api/shellBridge'
import { t } from '@/i18n'

/**
 * 工作区目录选择器：给会话绑定一个本地目录。
 *
 * <p>桌面壳（JCEF）下调原生 JFileChooser；浏览器 / headless 环境退化为手动输入绝对路径。
 * 提交空串等价于 `null`——解绑回默认会话工作区。
 */
const props = defineProps<{
  open: boolean
  /** 当前绑定路径（绝对路径或 null），用于打开时初始化输入框。 */
  currentPath: string | null
}>()

const emit = defineEmits<{
  close: []
  pick: [path: string | null]
}>()

const draft = ref<string>(props.currentPath ?? '')
const verifying = ref(false)
const valid = ref<boolean | null>(null)
const error = ref<string | null>(null)
const picking = ref(false)

watch(
  () => [props.open, props.currentPath],
  ([open, path]) => {
    if (open) {
      draft.value = (path as string | null) ?? ''
      valid.value = null
      error.value = null
    }
  }
)

/** 桌面壳 IPC 调原生 JFileChooser；失败时（浏览器 / headless）走文本兜底。 */
async function pickViaShell(): Promise<void> {
  picking.value = true
  try {
    const picked = await pickDirectoryViaShell(draft.value || undefined)
    if (picked) {
      draft.value = picked
      await verify()
    }
  } finally {
    picking.value = false
  }
}

/** 前端只做形状校验（绝对路径）；目录是否真实存在由后端 updateWorkspace 校验。 */
async function verify(): Promise<void> {
  error.value = null
  valid.value = null
  const v = draft.value.trim()
  if (!v) return
  verifying.value = true
  try {
    const ok = /^[a-zA-Z]:[\\/]/.test(v) || v.startsWith('/') || v.startsWith('\\')
    if (!ok) {
      valid.value = false
      error.value = t('chat.workspacePicker.absoluteRequired')
      return
    }
    valid.value = true
  } finally {
    verifying.value = false
  }
}

function submit(): void {
  const v = draft.value.trim()
  emit('pick', v === '' ? null : v)
}

function clear(): void {
  draft.value = ''
  valid.value = null
  error.value = null
  emit('pick', null)
}
</script>

<template>
  <el-dialog
    :model-value="open"
    :title="t('chat.workspacePicker.title')"
    width="520"
    align-center
    append-to-body
    @update:model-value="emit('close')"
  >
    <p class="mb-2 text-xs text-wb-muted">{{ t('chat.workspacePicker.hint') }}</p>
    <p class="mb-4 text-xs text-wb-muted">{{ t('chat.workspace.boundHint') }}</p>

    <div class="flex items-center gap-2">
      <el-input
        v-model="draft"
        :placeholder="t('chat.workspacePicker.placeholder')"
        @blur="verify"
        @keyup.enter="submit"
      />
      <el-button :loading="picking" @click="pickViaShell">
        <el-icon class="mr-1"><FolderSearch /></el-icon>
        {{ picking ? t('chat.workspacePicker.picking') : t('chat.workspacePicker.browse') }}
      </el-button>
    </div>

    <el-alert
      v-if="valid === true"
      class="mt-2"
      type="success"
      :title="t('chat.workspacePicker.valid')"
      :closable="false"
      show-icon
    />
    <el-alert
      v-else-if="valid === false"
      class="mt-2"
      type="error"
      :title="error ?? ''"
      :closable="false"
      show-icon
    />
    <p v-else class="mt-2 text-xs text-wb-muted">{{ t('chat.workspacePicker.hintPath') }}</p>

    <template #footer>
      <div class="flex items-center justify-between">
        <el-button text type="danger" @click="clear">
          {{ t('chat.workspacePicker.unbind') }}
        </el-button>
        <div class="flex gap-2">
          <el-button @click="emit('close')">{{ t('ui.btn.cancel') }}</el-button>
          <el-button
            type="primary"
            :disabled="draft.trim() !== '' && valid !== true"
            @click="submit"
          >
            {{ t('ui.btn.save') }}
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>
