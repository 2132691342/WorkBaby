import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet } from '@/api/client'
import type { Session } from '@/types/api'

/**
 * Home store：首页「最近会话」只读列表（{@code GET /api/v1/chat/sessions?page=1&page_size=5}）。
 */
export const useHomeStore = defineStore('home', () => {
  const sessions = ref<Session[]>([])
  const error = ref<string | null>(null)
  const loading = ref(false)

  /** 加载最近会话列表（最多 5 条）。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const page = await apiGet<{ items: Session[]; total: number }>('/api/v1/chat/sessions?page=1&page_size=5')
      sessions.value = page.items
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  return {
    sessions,
    error,
    loading,
    load
  }
})
