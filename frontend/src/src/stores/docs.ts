import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet } from '@/api/client'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { DocItem, DocDetail } from '@/types/api'

/**
 * Docs store：内置文档列表 + 详情（只读）。
 *
 * <p>{@link load} 串行执行 list → detail（避免 selected 渲染慢一拍 / 双触发不确定）。
 */
export const useDocsStore = defineStore('docs', () => {
  const toast = useToast()
  const docs = ref<DocItem[]>([])
  const selected = ref<DocDetail | null>(null)
  const error = ref<string | null>(null)
  const loading = ref(false)

  /**
   * 加载文档列表，并在非空时默认打开第一篇（串行 await；结束前 selected 一定就绪）。
   */
  async function load(): Promise<void> {
    error.value = null
    loading.value = true
    try {
      const list = await apiGet<DocItem[]>('/api/v1/docs')
      docs.value = list
      if (list.length > 0) {
        // 串行打开：保证 selected 在 load() resolve 时一定就绪
        const first = await apiGet<DocDetail>(`/api/v1/docs/${list[0].name}`)
        selected.value = first
      } else {
        selected.value = null
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      selected.value = null
      toast.error(t('chat.operationFailed'), msg)
    } finally {
      loading.value = false
    }
  }

  /**
   * 按文档 name 加载详情（用户切换左侧列表时调用）。
   * 失败要给 toast 反馈：静默失败会让 selected 不变，用户以为「点哪个都显示第一篇」。
   */
  async function open(name: string): Promise<void> {
    error.value = null
    try {
      selected.value = await apiGet<DocDetail>(`/api/v1/docs/${name}`)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('chat.operationFailed'), msg)
    }
  }

  return {
    docs,
    selected,
    error,
    loading,
    load,
    open
  }
})