import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { UploadFile, OpenFileDialog } from '@/wailsjs/go/main/App'
import type { FileInfo } from '@/types/api'

/**
 * Files store：文件列表查询 + 上传 + 删除。
 *
 * <p>列表 {@code GET /api/v1/files/search}；上传走 Wails 文件对话框 + {@code UploadFile} 绑定；删除 {@code POST /api/v1/files/{id}/delete}。
 */
export const useFilesStore = defineStore('files', () => {
  const files = ref<FileInfo[]>([])
  const error = ref<string | null>(null)
  const loading = ref(false)
  const uploading = ref(false)

  /** 拉取文件列表。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      files.value = await apiGet<FileInfo[]>('/api/v1/files/search')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 上传文件：Wails 文件对话框选本地文件 → 绑定 UploadFile（复制到托管目录）并在成功后刷新列表。 */
  async function upload(_file?: File): Promise<void> {
    uploading.value = true
    error.value = null
    try {
      const selected = await OpenFileDialog('选择文件', '*.*')
      if (!selected) return
      await UploadFile('', selected, '', '')
      await load()
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      uploading.value = false
    }
  }

  /** 删除文件并在成功后刷新列表。 */
  async function remove(id: string): Promise<void> {
    try {
      await apiPost(`/api/v1/files/${id}/delete`)
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  return { files, error, loading, uploading, load, upload, remove }
})
