import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { KnowledgeDoc, KnowledgeDocReq } from '@/types/api'

/**
 * KnowledgeDocs store（snake_case 契约）。
 *
 * <p>职责：知识文档 CRUD + 按 group（来源类型）过滤 + 全文搜索。
 */
export const useKnowledgeDocsStore = defineStore('kdocs', () => {
  const toast = useToast()
  const docs = ref<KnowledgeDoc[]>([])
  const groups = ref<string[]>([])
  const filterGroup = ref('')
  const search = ref('')
  const error = ref<string | null>(null)
  const showCreate = ref(false)
  const form = ref<KnowledgeDocReq>({ name: '', source: '', source_type: 'text' })
  const editing = ref<KnowledgeDoc | null>(null)

  /** 按 search / filterGroup 拼 url 加载文档列表与分组。
   *  健壮性：每次重新加载前清空 error；docs/groups 任意一个失败不影响另一个；
   *  docs 兜底空数组（避免上游偶发返回 null 触发下游 forEach 崩溃）。 */
  async function load(): Promise<void> {
    error.value = null
    try {
      let url = '/api/v1/kdocs'
      if (search.value.trim()) {
        url = `/api/v1/kdocs/search?q=${encodeURIComponent(search.value.trim())}&limit=20`
      } else if (filterGroup.value) {
        url = `/api/v1/kdocs/group/${encodeURIComponent(filterGroup.value)}?limit=50`
      }
      const list = await apiGet<KnowledgeDoc[]>(url)
      docs.value = Array.isArray(list) ? list : []
    } catch (e) {
      docs.value = []
      error.value = e instanceof Error ? e.message : String(e)
    }
    // 分组失败不影响主列表（前端只是少一个 filter 选项）
    try {
      const g = await apiGet<string[]>('/api/v1/kdocs/groups')
      groups.value = Array.isArray(g) ? g : []
    } catch {
      groups.value = []
    }
  }

  /** 新建或更新文档（编辑态走 update）。 */
  async function doSave(): Promise<void> {
    if (!form.value.name.trim() || !form.value.source.trim()) {
      error.value = t('kdoc.errRequired')
      toast.warning(t('kdoc.errRequired'))
      return
    }
    try {
      const req: KnowledgeDocReq = {
        name: form.value.name.trim(),
        source: form.value.source.trim(),
        source_type: form.value.source_type || 'text'
      }
      if (editing.value) {
        await apiPost<KnowledgeDoc>(`/api/v1/kdocs/${editing.value.id}/update`, req)
      } else {
        await apiPost<KnowledgeDoc>('/api/v1/kdocs', req)
      }
      showCreate.value = false
      editing.value = null
      form.value = { name: '', source: '', source_type: 'text' }
      await load()
      toast.success(t('common.saved'))
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
    }
  }

  /** 进入编辑态，回填表单。 */
  function startEdit(d: KnowledgeDoc): void {
    editing.value = d
    form.value = {
      name: d.name,
      source: d.source,
      source_type: d.source_type
    }
    showCreate.value = true
  }

  /** 删除文档。 */
  async function doDelete(d: KnowledgeDoc): Promise<void> {
    const dialog = useDialog()
    if (!await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('common.confirmDeleteNamed', d.name), danger: true })) return
    try {
      await apiPost(`/api/v1/kdocs/${d.id}/delete`, {})
      await load()
      toast.success(t('common.deleted'))
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('chat.operationFailed'), msg)
    }
  }

  return {
    docs,
    groups,
    filterGroup,
    search,
    error,
    showCreate,
    form,
    editing,
    load,
    doSave,
    /** createFolder 是 KnowledgeDocsView 等模板里的别名，避免上层误以为「点击无反应」。
     *  新建/编辑共用 doSave：内部按 editing 是否走 update 还是 create。 */
    createFolder: doSave,
    startEdit,
    doDelete
  }
})
