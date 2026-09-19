import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import type { TrustEntry, TrustResolveRESP, TrustState } from '@/types/api'

/**
 * 工作目录信任 store。
 *
 * 信任登记仅由用户显式写入；列表 / 解析 / 决定 / 撤销全部走 HTTP 兜底。
 * 询问（ask）由后端在工具闸门前自动发起，前端只需展示登记与撤销入口。
 */
export const useTrustStore = defineStore('trust', () => {
  const entries = ref<TrustEntry[]>([])
  const roots = ref<string[]>([])
  const loading = ref(false)

  async function loadAll(): Promise<void> {
    loading.value = true
    try {
      const [list, rs] = await Promise.all([
        apiGet<TrustEntry[]>('/api/v1/trust'),
        apiGet<string[]>('/api/v1/trust/roots')
      ])
      entries.value = Array.isArray(list) ? list : []
      roots.value = Array.isArray(rs) ? rs : []
    } finally {
      loading.value = false
    }
  }

  async function resolve(path: string): Promise<TrustResolveRESP> {
    const enc = encodeURIComponent(path)
    return await apiGet<TrustResolveRESP>(`/api/v1/trust/resolve?path=${enc}`)
  }

  async function decide(path: string, state: TrustState): Promise<TrustEntry | null> {
    try {
      const resp = await apiPost<TrustEntry>('/api/v1/trust', { path, state })
      await loadAll()
      return resp
    } catch {
      return null
    }
  }

  async function revoke(path: string): Promise<boolean> {
    try {
      await apiPost('/api/v1/trust/revoke', { path })
      await loadAll()
      return true
    } catch {
      return false
    }
  }

  return { entries, roots, loading, loadAll, resolve, decide, revoke }
})
