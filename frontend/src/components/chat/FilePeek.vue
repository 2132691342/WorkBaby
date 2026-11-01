<script setup lang="ts">
// 就地看文件改动：从工具卡打开，不动当前滚动位置、不离开对话。
// 数据走工作区的 diff 出参——与工作区页看的是同一份真相。
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useFilePeek } from '../../composables/filePeek'
import type { GitDiffLineVO } from '../../types/api'
import DiffView from '../workspace/DiffView.vue'
import ResizableDrawer from '../common/ResizableDrawer.vue'
import { errorText } from '../../utils/errors'

const { t } = useI18n()
const session = useSessionStore()
const settings = useSettingsStore()
const { path: peekPath, close: closePeek } = useFilePeek()

const lines = ref<GitDiffLineVO[]>([])
const truncated = ref(false)
const dropped = ref(0)
const loading = ref(false)
const error = ref('')
const relPath = ref('')

// 工具参数里的路径可能是绝对的：git 的 diff 只认工作区内的相对路径，
// 先按工作目录归一，落到工作区外面就直说看不了。
function toRel(target: string): string | null {
  const root = (session.current?.session_workspace || settings.boot?.boot_workspace || '').replace(/[\\/]+$/, '')
  if (!root) return target
  const key = (s: string) => s.replace(/\//g, '\\').toLowerCase()
  if (!key(target).startsWith(key(root) + '\\')) return key(target) === key(root) ? '' : null
  return target.slice(root.length + 1)
}

async function load(target: string) {
  if (!target) return
  error.value = ''
  relPath.value = ''
  loading.value = true
  try {
    const rel = toRel(target)
    if (rel === null) {
      error.value = t('chat.peekOutside')
      lines.value = []
      return
    }
    relPath.value = rel
    const d = await api.git.diff(rel, session.currentId || '')
    lines.value = d?.git_lines ?? []
    truncated.value = !!d?.git_truncated
    dropped.value = d?.git_dropped || 0
  } catch (e) {
    error.value = errorText(e) || t('errors.generic')
    lines.value = []
  } finally {
    loading.value = false
  }
}

watch(
  peekPath,
  (p) => {
    if (p) void load(p)
  },
  { immediate: true },
)
</script>

<template>
  <ResizableDrawer
    :model-value="!!peekPath"
    storage-key="chat-file-peek"
    :title="relPath || peekPath"
    class="peek-drawer"
    @update:model-value="(v: boolean) => { if (!v) closePeek() }"
  >
    <p v-if="error" class="peek-err">{{ error }}</p>
    <DiffView
      v-else
      :lines="lines"
      :path="relPath"
      :truncated="truncated"
      :dropped="dropped"
      :loading="loading"
    />
  </ResizableDrawer>
</template>

<style scoped>
.peek-err {
  padding: var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
.peek-drawer :deep(.el-drawer__body) {
  padding: var(--wb-sp-3);
}
</style>
