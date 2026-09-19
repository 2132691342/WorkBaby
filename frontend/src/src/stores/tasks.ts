import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { getApiBase } from '@/api/http'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { BackgroundTask, TaskSubmitREQ } from '@/types/api'

/**
 * Tasks store：后台任务域的前端状态。
 *
 * 数据面：REST 拉权威列表（进入面板 / 事件兜底）+ SSE scope=task 实时增量
 * （task:created / started / done，载荷带整条任务，整对象 upsert）。
 * 事件通道是「通知」不是「真相源」：断线重连后以 List 全量拉取兜底。
 */
export const useTasksStore = defineStore('tasks', () => {
  const items = ref<BackgroundTask[]>([])
  const total = ref(0)
  const loading = ref(false)
  const submitting = ref(false)

  // ===== 权威列表 =====

  /** 拉取任务列表（最新在前）。 */
  async function load(limit = 50): Promise<void> {
    loading.value = true
    try {
      const resp = await apiGet<{ items: BackgroundTask[]; total: number }>(`/api/v1/tasks?limit=${limit}`)
      items.value = resp.items ?? []
      total.value = resp.total ?? 0
    } catch {
      /* 面板静默：列表拉不到时保留现值，事件通道会再驱动刷新 */
    } finally {
      loading.value = false
    }
  }

  /** 提交后台任务：立即返回 pending 任务，执行在后台进行。 */
  async function submit(sessionID: string, agent: string, prompt: string): Promise<BackgroundTask | null> {
    if (!sessionID) {
      useToast().warning(t('tasks.noSession'))
      return null
    }
    submitting.value = true
    try {
      const req: TaskSubmitREQ = { session_id: sessionID, agent, prompt }
      const row = await apiPost<BackgroundTask>('/api/v1/tasks', req)
      upsert(row)
      useToast().success(t('tasks.submitted', row.id))
      return row
    } catch (e) {
      useToast().error(t('tasks.submitFailed'), e instanceof Error ? e.message : String(e))
      return null
    } finally {
      submitting.value = false
    }
  }

  /** 取消任务（pending 直接取消；running 触发 ctx 取消，终态由后端事件回推）。 */
  async function cancel(id: string): Promise<void> {
    try {
      await apiPost(`/api/v1/tasks/${id}/cancel`)
      const row = items.value.find((x) => x.id === id)
      if (row && row.state === 'pending') {
        // pending 的取消后端立即落终态并发事件；此处乐观置态避免闪 probation 窗口
        row.state = 'cancelled'
      }
    } catch (e) {
      useToast().error(t('tasks.cancelFailed'), e instanceof Error ? e.message : String(e))
    }
  }

  /** 乐观 / 事件驱动的整对象 upsert（最新在前）。 */
  function upsert(row: BackgroundTask): void {
    if (!row?.id) return
    const i = items.value.findIndex((x) => x.id === row.id)
    if (i >= 0) items.value[i] = row
    else items.value.unshift(row)
    total.value = Math.max(total.value, items.value.length)
  }

  // ===== SSE（scope=task）=====

  let es: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let reconnectAttempt = 0
  let wantConnected = false

  /** 终态通知：无论面板是否打开，任务完成/失败/取消都让用户看见。 */
  function notifyTerminal(row: BackgroundTask): void {
    const title = t('tasks.doneToast', row.id)
    if (row.state === 'completed') useToast().success(title, row.result ? row.result.slice(0, 120) : undefined)
    else if (row.state === 'failed') useToast().error(title, row.error || undefined)
    else useToast().info(title)
  }

  function handleEvent(name: string, raw: string): void {
    try {
      const p = JSON.parse(raw) as { task?: BackgroundTask }
      if (!p.task?.id) return
      const before = items.value.find((x) => x.id === p.task!.id)
      upsert(p.task)
      if (name === 'task:done' && before && before.state !== p.task.state) notifyTerminal(p.task)
      if (name === 'task:done' && !before) notifyTerminal(p.task)
    } catch {
      /* 非 JSON 帧忽略 */
    }
  }

  function connect(): void {
    if (es || !wantConnected) return
    const base = getApiBase()
    if (!base) return
    es = new EventSource(`${base}/events?scope=task`)
    es.addEventListener('sse-ready', () => {
      reconnectAttempt = 0
      void load() // 重连成功 → 权威列表兜底
    })
    for (const name of ['task:created', 'task:started', 'task:done']) {
      es.addEventListener(name, (e) => handleEvent(name, (e as MessageEvent).data as string))
    }
    es.onerror = () => {
      es?.close()
      es = null
      if (!wantConnected) return
      reconnectAttempt++
      const delay = Math.min(15000, 1000 * 2 ** Math.min(reconnectAttempt, 4))
      reconnectTimer = setTimeout(connect, delay)
    }
  }

  /** 建立 SSE 订阅（幂等）：ChatView 与任务面板挂载时调用。 */
  function ensureConnected(): void {
    wantConnected = true
    connect()
  }

  /** 断开订阅（卸载时调用；应用级常驻时无需调用）。 */
  function disconnect(): void {
    wantConnected = false
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = null
    es?.close()
    es = null
  }

  return {
    items, total, loading, submitting,
    load, submit, cancel, upsert,
    ensureConnected, disconnect
  }
})
