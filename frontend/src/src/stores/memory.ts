import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import type { MemoryEntry, MemoryList, MemoryOverview, MemoryWriteREQ } from '@/types/api'

/**
 * 长期记忆 store：一份 MEMORY.md 的浏览 / 检索 / 追加 / 删除 / 覆盖。
 * 三分类（情景 / 语义 / 程序）与回写收件箱已下线——记忆只有一处，写入即生效。
 */
export const useMemoryStore = defineStore('memory', () => {
  const overview = ref<MemoryOverview>({ file: '', entries: 0, sections: [] })
  const items = ref<MemoryEntry[]>([])
  const loading = ref(false)
  const query = ref('')
  const section = ref('')
  const error = ref<string | null>(null)

  /** 加载概览与条目列表（section 非空时按分节过滤）。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const qs = section.value ? `?section=${encodeURIComponent(section.value)}` : ''
      const [ov, list] = await Promise.all([
        apiGet<MemoryOverview>('/api/v1/memory'),
        apiGet<MemoryList>(`/api/v1/memory/list${qs}`)
      ])
      overview.value = ov
      items.value = list.items ?? []
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 检索；查询为空时回到列表。 */
  async function search(): Promise<void> {
    const q = query.value.trim()
    if (!q) {
      await load()
      return
    }
    loading.value = true
    error.value = null
    try {
      items.value = await apiGet<MemoryEntry[]>(`/api/v1/memory/search?q=${encodeURIComponent(q)}&k=50`)
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 追加一条记忆。 */
  async function append(req: MemoryWriteREQ): Promise<void> {
    await apiPost('/api/v1/memory/append', req)
    query.value = ''
    await load()
  }

  /** 删除一条记忆（分节 + 正文精确匹配）。 */
  async function remove(entry: MemoryEntry): Promise<void> {
    await apiPost('/api/v1/memory/delete', { section: entry.section, text: entry.text })
    await load()
  }

  /** 整篇覆盖（编辑器保存）。 */
  async function replace(text: string): Promise<void> {
    await apiPost('/api/v1/memory/replace', { text })
    await load()
  }

  /** 读全文（编辑器打开）。 */
  async function readText(): Promise<string> {
    return apiGet<string>('/api/v1/memory/text')
  }

  return {
    overview, items, loading, query, section, error,
    load, search, append, remove, replace, readText
  }
})
