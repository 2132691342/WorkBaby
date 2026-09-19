<script setup lang="ts">
import type { Component } from 'vue'
import { PackageCheck, Globe, FileImage, FileText, FileVideo, File as FileIcon, Download } from '@/components/common/icons'
import type { ArtifactFile, ArtifactPayload } from '@/types/api'
import { openExternal } from '@/api/shellBridge'
import { t } from '@/i18n'

/**
 * 交付卡片：present_files 工具的产出清单。
 *
 * <p>渲染 present_files 工具发布的 {@code artifact} 事件：第一项自动聚焦（可预览类型内联预览），
 * 其余以卡片形式展示，点击打开 / 下载。
 */
const props = defineProps<{
  payload: ArtifactPayload
}>()

const primary = props.payload.files[0] ?? null

function kindIcon(kind: string): Component {
  if (kind === 'html' || kind === 'url') return Globe
  if (kind === 'image') return FileImage
  if (kind === 'video') return FileVideo
  if (kind === 'pdf' || kind === 'text') return FileText
  return FileIcon
}

function canPreview(f: ArtifactFile): boolean {
  return ['html', 'url', 'image', 'text', 'pdf', 'video'].includes(f.kind)
}

function fmtSize(size?: number): string {
  if (!size) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <div class="rounded-xl border border-wb-border bg-wb-surface p-3 shadow-sm">
    <div class="mb-2 flex items-center gap-2 text-xs font-medium text-wb-ink">
      <PackageCheck class="h-4 w-4 shrink-0 text-wb-mint" />
      <span>{{ t('chat.artifacts') }}</span>
      <span v-if="props.payload.explanation" class="min-w-0 truncate font-normal text-wb-muted">
        {{ props.payload.explanation }}
      </span>
    </div>

    <!-- 第一项自动聚焦（可预览类型内联预览） -->
    <div
      v-if="primary && canPreview(primary) && primary.url"
      class="mb-2 overflow-hidden rounded-lg border border-wb-border bg-wb-surface"
    >
      <iframe
        v-if="['html', 'url', 'text', 'pdf'].includes(primary.kind)"
        :src="primary.url"
        :title="primary.name"
        class="h-64 w-full bg-wb-surface"
        sandbox="allow-scripts allow-popups allow-forms"
      />
      <img
        v-else-if="primary.kind === 'image'"
        :src="primary.url"
        :alt="primary.name"
        class="max-h-64 w-full object-contain bg-wb-surface"
      />
      <video v-else-if="primary.kind === 'video'" :src="primary.url" controls class="h-64 w-full bg-wb-surface" />
    </div>

    <!-- 交付卡片列表 -->
    <div class="flex flex-wrap gap-2">
      <div
        v-for="f in props.payload.files"
        :key="f.path"
        class="flex items-center gap-2 rounded-lg border border-wb-border bg-wb-surface px-2.5 py-1.5 text-xs"
      >
        <component :is="kindIcon(f.kind)" class="h-3.5 w-3.5 shrink-0 text-wb-primary-strong" />
        <!-- 链接一律拦截交系统浏览器：WebView 内导航会把 SPA 页面整个替换掉 -->
        <a
          v-if="f.url"
          :href="f.url"
          class="max-w-[160px] cursor-pointer truncate text-wb-ink transition-colors hover:text-wb-primary-strong"
          @click.prevent="openExternal(f.url)"
        >
          {{ f.name }}
        </a>
        <span v-else class="max-w-[160px] truncate text-wb-muted">{{ f.name }}</span>
        <span v-if="f.size" class="shrink-0 text-[10px] text-wb-muted">{{ fmtSize(f.size) }}</span>
        <a
          v-if="f.url && f.kind !== 'url'"
          :href="f.url"
          :download="f.name"
          :title="t('chat.download')"
          class="shrink-0 cursor-pointer text-wb-muted transition-colors hover:text-wb-primary-strong"
          @click.prevent="openExternal(f.url)"
        >
          <Download class="h-3 w-3" />
        </a>
      </div>
    </div>
  </div>
</template>
