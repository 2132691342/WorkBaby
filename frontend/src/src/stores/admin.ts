import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet } from '@/api/client'
import type { AdminOverview, RuntimeStatus } from '@/types/api'

/**
 * Admin store：管理后台系统概览与内置运行时状态（只读）。
 *
 * <p>概览：{@code GET /api/v1/admin/overview}；运行时：{@code GET /api/v1/meta/runtime}。
 */
export const useAdminStore = defineStore('admin', () => {
  const overview = ref<AdminOverview | null>(null)
  const runtimeStatus = ref<RuntimeStatus | null>(null)
  const error = ref<string | null>(null)

  /** 加载系统概览、聚合计数与内置运行时状态；运行时失败不影响概览展示。 */
  async function load(): Promise<void> {
    error.value = null
    try {
      overview.value = await apiGet<AdminOverview>('/api/v1/admin/overview')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return
    }
    try {
      runtimeStatus.value = await apiGet<RuntimeStatus>('/api/v1/meta/runtime')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  return {
    overview,
    runtimeStatus,
    error,
    load
  }
})
