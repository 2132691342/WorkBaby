<script setup lang="ts">
import { Paperclip, X as Close } from '@/components/common/icons'
import { t } from '@/i18n'
import type { FileInfo } from '@/types/api'

/** 输入框上方已选附件条：图片显示缩略图，其余显示文件图标。 */
const props = defineProps<{
  attachments: FileInfo[]
}>()

const emit = defineEmits<{
  remove: [id: string]
}>()

/** 取文件扩展名（小写）。 */
function ext(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

function isImage(mime: string | undefined): boolean {
  return mime?.startsWith('image/') === true
}
</script>

<template>
  <div v-if="props.attachments.length > 0" class="flex flex-wrap gap-2 px-1" role="list">
    <span
      v-for="a in props.attachments"
      :key="a.id"
      role="listitem"
      class="group inline-flex h-11 max-w-[240px] items-center gap-2 rounded-xl border border-wb-border bg-wb-surface pl-1 pr-1 text-xs text-wb-ink transition-colors hover:border-wb-primary"
    >
      <span class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-wb-primary/10">
        <img
          v-if="isImage(a.mime_type)"
          :src="`/files/files/${a.id}`"
          :alt="a.original_name || a.name"
          class="h-full w-full object-cover"
        />
        <Paperclip v-else class="h-4 w-4 text-wb-primary" />
      </span>

      <span class="min-w-0 flex-1 truncate" :title="a.original_name || a.name">
        {{ a.original_name || a.name }}
        <span v-if="ext(a.original_name || a.name)" class="ml-1 text-[10px] uppercase text-wb-muted">
          {{ ext(a.original_name || a.name) }}
        </span>
      </span>

      <el-button
        type="danger"
        text
        size="small"
        circle
        :title="t('chat.removeAttachment')"
        :aria-label="t('chat.removeAttachment')"
        @click="emit('remove', a.id)"
      >
        <el-icon :size="14"><Close /></el-icon>
      </el-button>
    </span>
  </div>
</template>
