<script setup lang="ts">
// 单文件检查体：「差异 / 文件」双标签。差异问 git 要工作区真实那份（引擎给的是
// 它当时的改动，用户可能又改过）；文件问 files/read。工作台每个标签渲染一份。
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import DiffView from '@/components/workspace/DiffView.vue'
import FilePreview from '@/components/workspace/FilePreview.vue'
import WbSeg from '@/components/common/WbSeg.vue'
import AppIcon from '@/components/common/AppIcon.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import type { FileRead, GitDiff } from '@/types/api'

const props = defineProps<{
  /** 目标文件绝对路径 */
  path: string
  /** 当前视图：差异 / 文件 */
  mode: 'diff' | 'file'
}>()

const emit = defineEmits<{ 'update:mode': [mode: 'diff' | 'file'] }>()

const app = useAppStore()
const router = useRouter()
const { t } = useI18n()

const diff = ref<GitDiff | null>(null)
const file = ref<FileRead | null>(null)
const diffLoading = ref(false)
const fileLoading = ref(false)
const diffError = ref('')
const fileError = ref('')

// 问 git 用的路径：工作区内折成相对路径，工作区外原样用绝对路径。
function toRel(target: string): string {
  const root = (app.workspace || '').replace(/[\\/]+$/, '')
  if (!root) return target
  const key = (s: string) => s.replace(/\//g, '\\').toLowerCase()
  if (!key(target).startsWith(key(root) + '\\')) return target
  return target.slice(root.length + 1)
}

const relPath = computed(() => (props.path ? toRel(props.path) : ''))

const segItems = computed(() => [
  { value: 'diff', label: t('chat.peekTabDiff'), icon: 'git' },
  { value: 'file', label: t('chat.peekTabFile'), icon: 'file' },
])

const isPreviewable = computed(() => ['pdf', 'sheet', 'doc', 'image'].includes(file.value?.file_kind ?? ''))

async function loadDiff() {
  diffLoading.value = true
  diffError.value = ''
  try {
    diff.value = await api.gitDiff(relPath.value)
  } catch (err) {
    diffError.value = err instanceof Error ? err.message : String(err)
    diff.value = null
  } finally {
    diffLoading.value = false
  }
}

async function loadFile() {
  fileLoading.value = true
  fileError.value = ''
  try {
    file.value = await api.fileRead(props.path)
  } catch (err) {
    fileError.value = err instanceof Error ? err.message : String(err)
    file.value = null
  } finally {
    fileLoading.value = false
  }
}

// 路径变了就两份都预取：切标签即看，不必再等一次往返。
watch(
  () => props.path,
  (p) => {
    if (!p) {
      diff.value = null
      file.value = null
      return
    }
    void loadDiff()
    void loadFile()
  },
  { immediate: true },
)

function openWorkspace(): void {
  void router.push({ name: 'workspace' })
}
</script>

<template>
  <div class="inspect">
    <div class="inspect-head">
      <WbSeg :model-value="mode" :items="segItems" size="sm" @update:model-value="(v: string) => emit('update:mode', v as 'diff' | 'file')" />
      <button class="wb-btn text sm inspect-ws" :title="t('chat.peekOpenWorkspace')" @click="openWorkspace">
        <AppIcon name="folder" :size="13" />
        {{ t('chat.peekOpenWorkspace') }}
      </button>
    </div>
    <p class="inspect-path selectable" :title="path">{{ relPath }}</p>

    <div class="inspect-body">
      <template v-if="mode === 'diff'">
        <p v-if="diffError" class="inspect-err">{{ diffError }}</p>
        <DiffView v-else :diff="diff" :path="relPath" :loading="diffLoading" />
      </template>

      <template v-else>
        <p v-if="fileError" class="inspect-err">{{ fileError }}</p>
        <div v-else-if="fileLoading" class="inspect-state">
          <span class="dot busy" />
          {{ t('common.loading') }}
        </div>
        <div v-else-if="file?.is_binary && isPreviewable" class="inspect-binary">
          <FilePreview :file="file" />
        </div>
        <div v-else-if="file?.is_binary" class="inspect-state">{{ t('files.binary') }}</div>
        <template v-else-if="file">
          <p v-if="file.truncated" class="inspect-note">{{ t('files.previewTruncated', { size: file.size_bytes }) }}</p>
          <pre class="inspect-pre selectable">{{ file.file_text }}</pre>
        </template>
      </template>
    </div>
  </div>
</template>

<style scoped>
.inspect {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
}

.inspect-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
}

.inspect-ws {
  margin-left: auto;
}

.inspect-path {
  margin: var(--wb-sp-1) 0 var(--wb-sp-2);
  padding: 0 var(--wb-sp-3);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  word-break: break-all;
}

.inspect-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 var(--wb-sp-3) var(--wb-sp-3);
}

.inspect-err {
  margin: 0;
  padding: var(--wb-sp-4);
  font-size: var(--wb-fs-sm);
  color: var(--wb-bad);
  line-height: 1.7;
}

.inspect-state {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-6);
  color: var(--wb-mute);
  font-size: var(--wb-fs-sm);
}

.inspect-note {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-warn);
}

.inspect-binary {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-md);
  overflow: hidden;
}

.inspect-pre {
  margin: 0;
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-md);
  background: var(--wb-sunken);
  color: var(--wb-ink);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>