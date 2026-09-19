import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import type { McpServer } from '@/types/api'

/**
 * MCP store：直接编辑 mcp.json 原始 JSON（后端原子写 + 校验），保存后触发热重载。
 */
export const useMcpStore = defineStore('mcp', () => {
  const servers = ref<McpServer[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const info = ref<string | null>(null)
  const activeCount = ref(0)

  /** mcp.json 原始内容（JSON 编辑器绑定）。 */
  const rawContent = ref<string>('')
  const savingRaw = ref(false)

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      servers.value = await apiGet<McpServer[]>('/api/v1/mcp/servers')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 读取 mcp.json 原始内容到编辑器。 */
  async function loadRaw(): Promise<void> {
    error.value = null
    try {
      const r = await apiGet<{ content: string }>('/api/v1/mcp/servers/raw')
      rawContent.value = r.content
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 保存 mcp.json 原始内容（后端校验 + 原子写 + 自动 reload）。返回 true=成功。 */
  async function saveRaw(): Promise<boolean> {
    error.value = null
    info.value = null
    savingRaw.value = true
    try {
      const r = await apiPost<{ saved: boolean; active: number }>('/api/v1/mcp/servers/raw', {
        content: rawContent.value
      })
      activeCount.value = r.active
      await load()
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      savingRaw.value = false
    }
  }

  /** 触发后端热重载（让新 MCP server 配置立即生效）。 */
  async function reload(): Promise<{ ok: boolean; active: number }> {
    error.value = null
    try {
      const r = await apiPost<{ reloaded: boolean; active: number }>(
        '/api/v1/mcp/servers/reload',
        {}
      )
      activeCount.value = r.active
      await load()
      return { ok: true, active: r.active }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return { ok: false, active: activeCount.value }
    }
  }

  return {
    servers,
    loading,
    error,
    info,
    activeCount,
    rawContent,
    savingRaw,
    load,
    loadRaw,
    saveRaw,
    reload
  }
})
