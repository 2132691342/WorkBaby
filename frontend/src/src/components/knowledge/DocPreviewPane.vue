<script setup lang="ts">
/**
 * 知识库文档预览：PDF / Word / Excel / 纯文本。
 *
 * <p>二进制文件走 GET /api/v1/kdocs/:id/file 拿字节流，落到 Blob URL 后喂给对应的渲染器：
 * <ul>
 *   <li>PDF — {@code vue-pdf-embed}（基于 pdf.js）</li>
 *   <li>Excel/CSV — {@code xlsx}（SheetJS）转首个 sheet 为 HTML 表格</li>
 *   <li>Word — {@code docx-preview}（docx.js）渲染到容器</li>
 *   <li>text / url — 直接显示 source 原文（保持 pre-wrap）</li>
 * </ul>
 * 不支持的格式：给出下载链接兜底。
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import VuePdfEmbed from 'vue-pdf-embed'
import * as XLSX from 'xlsx'
import { renderAsync } from 'docx-preview'
import { Download } from '@/components/common/icons'
import { getApiBase } from '@/api/http'
import { t } from '@/i18n'
import type { KnowledgeDoc } from '@/types/api'

const props = defineProps<{
  doc: KnowledgeDoc
  /** text/url 类型：直接展示 source 原文（后端已 fetch 过则传这里，否则留空让组件拉） */
  text?: string | null
}>()

const loading = ref(false)
const error = ref<string | null>(null)

const ext = computed(() => {
  const s = props.doc.source || ''
  const i = s.lastIndexOf('.')
  return i >= 0 ? s.slice(i + 1).toLowerCase() : ''
})

const isPdf = computed(() => ext.value === 'pdf')
const isXlsx = computed(() => ['xlsx', 'xls', 'csv'].includes(ext.value))
const isDocx = computed(() => ext.value === 'docx')
const isText = computed(() => props.doc.source_type === 'text' || props.doc.source_type === 'url')
const canPreview = computed(() => isPdf.value || isXlsx.value || isDocx.value || isText.value)

/** Blob URL：PDF / docx-preview 共用，避免重复解码 */
const blobUrl = ref<string | null>(null)
/** SheetJS 转出来的首个 sheet HTML */
const xlsxHtml = ref<string | null>(null)
/** docx-preview 渲染目标容器 */
const docxRef = ref<HTMLDivElement | null>(null)

function revoke(): void {
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
    blobUrl.value = null
  }
}

async function load(): Promise<void> {
  loading.value = true
  error.value = null
  xlsxHtml.value = null
  if (docxRef.value) docxRef.value.innerHTML = ''
  revoke()

  try {
    if (isText.value) return // text 类型只显示 props.text
    const url = `${getApiBase()}/kdocs/${encodeURIComponent(props.doc.id)}/file`
    const res = await fetch(url, { credentials: 'same-origin' })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const blob = await res.blob()
    blobUrl.value = URL.createObjectURL(blob)

    if (isXlsx.value) {
      const buf = await blob.arrayBuffer()
      const wb = XLSX.read(buf, { type: 'array' })
      const first = wb.SheetNames[0]
      if (first) xlsxHtml.value = XLSX.utils.sheet_to_html(wb.Sheets[first])
    } else if (isDocx.value && docxRef.value) {
      // docx-preview 自行创建内部样式，无需外部 stylesheet
      await renderAsync(blob, docxRef.value, undefined, {
        className: 'docx-preview-root',
        inWrapper: true
      })
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

watch(() => props.doc.id, () => load(), { immediate: true })
onBeforeUnmount(revoke)

function downloadUrl(): string {
  return `${getApiBase()}/kdocs/${encodeURIComponent(props.doc.id)}/file`
}
</script>

<template>
  <div class="kdoc-preview">
    <div v-if="loading" class="kdoc-loading">
      <span class="led g" />{{ t('ui.status.loading') }}
    </div>
    <div v-else-if="error" class="alert a-danger">{{ error }}</div>

    <!-- text / url：直接显示原文 -->
    <template v-else-if="isText">
      <pre class="kdoc-text">{{ text ?? doc.source }}</pre>
    </template>

    <!-- 不支持预览的格式：给下载链接 -->
    <template v-else-if="!canPreview">
      <div class="kdoc-fallback">
        <p class="muted fs11">{{ t('kdoc.previewUnsupported', ext) }}</p>
        <a class="btn btn-sm" :href="downloadUrl()" target="_blank" download>
          <Download class="ic ic-sm" />{{ t('kdoc.downloadFile') }}
        </a>
      </div>
    </template>

    <!-- PDF -->
    <VuePdfEmbed
      v-else-if="isPdf && blobUrl"
      :source="blobUrl"
      class="kdoc-pdf"
    />

    <!-- Excel / CSV -->
    <div
      v-else-if="isXlsx"
      class="kdoc-xlsx"
      v-html="xlsxHtml"
    />

    <!-- Word -->
    <div
      v-else-if="isDocx"
      ref="docxRef"
      class="kdoc-docx"
    />
  </div>
</template>

<style scoped>
.kdoc-preview {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.kdoc-loading {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--wb-muted);
}
.kdoc-text {
  margin: 0;
  padding: 12px 14px;
  max-height: 56vh;
  overflow: auto;
  background: var(--wb-bg);
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--wb-ink);
}
.kdoc-pdf {
  height: 56vh;
  overflow: auto;
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  background: var(--wb-bg);
}
.kdoc-pdf :deep(canvas) {
  display: block;
  margin: 0 auto;
}
.kdoc-xlsx {
  max-height: 56vh;
  overflow: auto;
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  background: var(--wb-bg);
  padding: 8px 10px;
  font-size: 12px;
  color: var(--wb-ink);
}
.kdoc-xlsx :deep(table) {
  border-collapse: collapse;
  width: 100%;
}
.kdoc-xlsx :deep(th),
.kdoc-xlsx :deep(td) {
  border: 1px solid var(--wb-border);
  padding: 4px 8px;
  text-align: left;
}
.kdoc-xlsx :deep(th) {
  background: var(--wb-surface-hover);
  font-weight: 600;
}
.kdoc-docx {
  max-height: 56vh;
  overflow: auto;
  padding: 10px 14px;
  background: var(--wb-bg);
  border: 1px solid var(--wb-border);
  border-radius: 12px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--wb-ink);
}
.kdoc-fallback {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}
</style>