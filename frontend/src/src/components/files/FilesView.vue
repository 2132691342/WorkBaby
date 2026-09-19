<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { File, Image, Database, Search, Upload, Trash2 } from '@/components/common/icons'
import { useFilesStore } from '@/stores/files'
import { t } from '@/i18n'
import type { FileInfo } from '@/types/api'
import { formatDate } from '@/utils/time'
import type { Component } from 'vue'

/**
 * 文件管理（照 prd/WorkBaby-UI-Prototype.html 13 屏）：
 * hero + 统计条（总数/大小/搜索/上传）+ 文件卡网格（fcard）。
 */
const filesStore = useFilesStore()
const { files, error, loading, uploading } = storeToRefs(filesStore)
const { load, upload, remove } = filesStore

const search = ref('')

function fmtSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

const totalSize = computed(() => files.value.reduce((a, f) => a + (f.size ?? 0), 0))

const filtered = computed<FileInfo[]>(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return files.value
  return files.value.filter((f) => (f.original_name ?? '').toLowerCase().includes(q))
})

/** 文件类型 → 缩略图图标。 */
function thumbIcon(f: FileInfo): Component {
  const ft = (f.file_type ?? '').toLowerCase()
  if (ft.includes('image') || /\.(png|jpg|jpeg|gif|svg|webp)$/.test(ft)) return Image
  if (ft.includes('json') || ft.includes('xml') || ft.includes('csv') || ft.includes('excel')) return Database
  return File
}

function onFileChange(file: { raw?: File }): void {
  void handleUpload(file.raw)
}
async function handleUpload(file: File | undefined): Promise<void> {
  if (!file) return
  await upload(file)
}

onMounted(load)
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap" style="max-width: 860px">
      <!-- Hero -->
      <header class="hero">
        <div class="tile"><File class="ic" /></div>
        <div>
          <h1>{{ t('file.title') }}</h1>
          <p>{{ t('file.subtitle') }}</p>
        </div>
      </header>

      <!-- 统计 + 操作条 -->
      <div class="card p-sm">
        <div class="flex-r">
          <span class="fs12 muted">{{ files.length }} {{ t('file.items') }} · {{ fmtSize(totalSize) }}</span>
          <span class="sp" />
          <div class="field-wrap" style="width: 220px">
            <Search class="ic ic-sm" />
            <input v-model="search" class="input with-icon" :placeholder="t('file.searchPlaceholder')" />
          </div>
          <el-upload :show-file-list="false" :auto-upload="false" :disabled="uploading" @change="onFileChange">
            <button class="btn btn-primary">
              <Upload class="ic ic-sm" />
              {{ uploading ? t('file.uploading') : t('file.upload') }}
            </button>
          </el-upload>
        </div>
      </div>

      <div v-if="error" class="alert a-danger">{{ error }}</div>

      <!-- 文件卡网格 -->
      <div v-if="loading" class="empty">{{ t('ui.status.loading') }}</div>
      <div v-else-if="filtered.length === 0" class="empty">{{ t('file.empty') }}</div>
      <div v-else class="fgrid">
        <div v-for="f in filtered" :key="f.id" class="fcard" style="position: relative">
          <button
            class="btn-icon"
            style="position: absolute; right: 6px; top: 6px; opacity: 0.6"
            :title="t('ui.btn.delete')"
            @click="remove(f.id)"
          >
            <Trash2 class="ic ic-sm" style="color: var(--wb-danger)" />
          </button>
          <div class="thumb">
            <component :is="thumbIcon(f)" class="ic" />
          </div>
          <h5>{{ f.original_name }}</h5>
          <p>{{ fmtSize(f.size ?? 0) }} · {{ formatDate(f.created_at) }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
