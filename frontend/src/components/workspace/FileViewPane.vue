<script setup lang="ts">
// 文件预览面板：文件树里点一个文件，这里显示它的内容。
// 与 git 无关：普通目录里的文件同样能看——文件接口不依赖仓库。
// 只读；想改走「编辑」按钮打开抽屉（与文件树共用那一份抽屉）。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import FilePreview from '@/components/workspace/FilePreview.vue'
import WbSkeleton from '@/components/common/WbSkeleton.vue'
import { api } from '@/api/http'
import type { FileRead } from '@/types/api'

const props = defineProps<{
  /** 相对工作目录的路径；空串表示还没选文件 */
  path: string
  /** 每次 +1 就重读一次：文件被保存后正文要跟着变 */
  reloadKey?: number
}>()

const emit = defineEmits<{ edit: [path: string] }>()

const { t } = useI18n()

const file = ref<FileRead | null>(null)
const loading = ref(false)
const error = ref('')
// 竞态守卫：连点两个文件时，晚到的旧响应不能盖掉新文件。
let seq = 0

/** 可视化预览只认这几类；其余二进制只给占位。 */
const previewable = computed(() => ['pdf', 'sheet', 'doc', 'image'].includes(file.value?.file_kind ?? ''))

async function load(path: string): Promise<void> {
  const mine = ++seq
  if (!path) {
    file.value = null
    error.value = ''
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  try {
    const next = await api.fileRead(path)
    if (mine !== seq) return
    file.value = next
  } catch (err) {
    if (mine !== seq) return
    file.value = null
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (mine === seq) loading.value = false
  }
}

watch(
  () => [props.path, props.reloadKey ?? 0] as const,
  ([path]) => {
    void load(path)
  },
  { immediate: true },
)
</script>

<template>
  <section class="file-view card">
    <header class="fv-head">
      <AppIcon name="file" :size="13" />
      <span class="fv-path selectable" :title="props.path">{{ props.path || t('workspace.preview') }}</span>
      <button
        v-if="file && !file.is_binary"
        class="wb-btn sm ghost"
        type="button"
        :title="t('files.editFile')"
        @click="emit('edit', props.path)"
      >
        <AppIcon name="edit" :size="12" />
        {{ t('files.editFile') }}
      </button>
    </header>

    <div class="fv-body">
      <!-- 骨架只在真取数时出现：取完之后它必须消失，否则失败也停在转圈上 -->
      <WbSkeleton v-if="loading && !file" :rows="5" />

      <p v-else-if="error" class="fv-error selectable">
        <AppIcon name="alert" :size="15" />
        {{ error }}
      </p>

      <div v-else-if="!props.path" class="fv-empty">
        <AppIcon name="file" :size="22" class="fv-empty-ic" />
        <p class="fv-empty-title">{{ t('workspace.previewEmptyTitle') }}</p>
        <p class="fv-empty-hint">{{ t('workspace.previewEmptyHint') }}</p>
      </div>

      <div v-else-if="file && file.is_binary && previewable" class="fv-binary">
        <FilePreview :file="file" />
      </div>

      <p v-else-if="file && file.is_binary" class="fv-empty-hint">{{ t('files.binary') }}</p>

      <template v-else-if="file">
        <p v-if="file.truncated" class="fv-note">{{ t('files.previewTruncated', { size: file.size_bytes }) }}</p>
        <pre class="fv-text selectable">{{ file.file_text }}</pre>
      </template>
    </div>
  </section>
</template>

<style scoped>
.file-view {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}
.fv-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
  color: var(--wb-mute);
}
.fv-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
}
.fv-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--wb-sp-3);
}
.fv-error {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  margin: 0;
  color: var(--wb-bad);
  font-size: var(--wb-fs-xs);
  line-height: 1.7;
  overflow-wrap: anywhere;
}
.fv-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  height: 100%;
  min-height: 160px;
  text-align: center;
}
.fv-empty-ic {
  color: var(--wb-faint);
}
.fv-empty-title {
  margin: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 650;
  color: var(--wb-ink-2);
}
.fv-empty-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-mute);
}
.fv-note {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-warn);
}
.fv-binary {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-md);
  overflow: hidden;
}
.fv-text {
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
