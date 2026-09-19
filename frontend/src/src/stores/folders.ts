import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { t } from '@/i18n'
import type { Folder, FolderTreeNode, FolderReq } from '@/types/api'

/**
 * 文件夹 store：目录树与根目录列表的 CRUD，端点见 doc/16。
 */
export const useFoldersStore = defineStore('folders', () => {
  const tree = ref<FolderTreeNode[]>([])
  const rootChildren = ref<Folder[]>([])
  const selected = ref<Folder | null>(null)
  const error = ref<string | null>(null)
  const showCreate = ref(false)
  const showRename = ref(false)
  const showMove = ref(false)
  const newName = ref('')
  const newDesc = ref('')
  const newParentID = ref<string | null>(null)
  const renameName = ref('')
  const moveTarget = ref<string | null>(null)

  /** 加载文件夹树与根目录列表。 */
  async function load(): Promise<void> {
    try {
      tree.value = await apiGet<FolderTreeNode[]>('/api/v1/folders/tree')
      rootChildren.value = await apiGet<Folder[]>('/api/v1/folders')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 选中目录，右侧显示详情。 */
  async function selectFolder(f: Folder): Promise<void> {
    selected.value = f
  }

  /** 新建文件夹。 */
  async function createFolder(): Promise<void> {
    if (!newName.value.trim()) {
      error.value = t('common.nameRequired')
      return
    }
    try {
      const req: FolderReq = {
        name: newName.value.trim(),
        parent_id: newParentID.value,
        description: newDesc.value || null
      }
      await apiPost<Folder>('/api/v1/folders', req)
      showCreate.value = false
      newName.value = ''
      newDesc.value = ''
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 重命名当前选中目录。 */
  async function doRename(): Promise<void> {
    if (!selected.value || !renameName.value.trim()) return
    try {
      const req: FolderReq = {
        name: renameName.value.trim(),
        parent_id: selected.value.parent_id,
        description: null
      }
      await apiPost<Folder>(`/api/v1/folders/${selected.value.id}/update`, req)
      showRename.value = false
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 移动当前选中目录到指定父目录。 */
  async function doMove(): Promise<void> {
    if (!selected.value) return
    try {
      const req: FolderReq = {
        name: selected.value.name,
        parent_id: moveTarget.value,
        description: selected.value.description
      }
      await apiPost<Folder>(`/api/v1/folders/${selected.value.id}/update`, req)
      showMove.value = false
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 删除指定目录。 */
  async function doDelete(f: Folder): Promise<void> {
    const dialog = useDialog()
    if (!await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('common.confirmDeleteNamed', f.name), danger: true })) return
    try {
      await apiPost(`/api/v1/folders/${f.id}/delete`, {})
      if (selected.value?.id === f.id) selected.value = null
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  return {
    tree,
    rootChildren,
    selected,
    error,
    showCreate,
    showRename,
    showMove,
    newName,
    newDesc,
    newParentID,
    renameName,
    moveTarget,
    load,
    selectFolder,
    createFolder,
    doRename,
    doMove,
    doDelete
  }
})
