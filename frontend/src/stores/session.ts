// 会话列表与当前会话：会话目录即真相，列表来自 PI 的会话文件。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import { i18n } from '../i18n'
import type { MessageVO, SessionVO } from '../types/api'
import { useChatStore } from './chat'

export const useSessionStore = defineStore('session', () => {
  const list = ref<SessionVO[]>([])
  /** 当前会话编号；null 表示「新对话」草稿（还没落盘） */
  const currentId = ref<string | null>(null)
  const currentSession = ref<SessionVO | null>(null)
  const messages = ref<MessageVO[]>([])
  /** 侧栏的工作目录过滤；空串表示全部 */
  const workspaceFilter = ref('')

  const current = computed(
    () =>
      list.value.find((s) => s.id === currentId.value) ||
      (currentSession.value?.id === currentId.value ? currentSession.value : null),
  )

  async function loadList() {
    list.value = await api.sessions.list(workspaceFilter.value)
    return list.value
  }

  /** 新对话：不建会话——PI 那边要等第一条消息才肯落盘 */
  function draft() {
    currentId.value = null
    currentSession.value = null
    messages.value = []
  }

  const openingId = ref<string | null>(null)
  /** 刚发出一条消息、把界面绑到后端新建的会话上：这不算「用户切换会话」，
   *  对话页据此跳过重置，否则会把刚收到的流式内容清掉。 */
  const adopting = ref(false)

  async function open(id: string) {
    openingId.value = id
    try {
      // 换会话先清空：不清的话这一瞬间标题已经是新的、消息还是上一段。
      if (currentId.value !== id) messages.value = []
      currentId.value = id
      const detail = await api.sessions.detail(id)
      if (!detail || !Array.isArray(detail.messages)) {
        throw new Error(i18n.global.t('chat.snapshotBroken'))
      }
      messages.value = detail.messages
      const idx = list.value.findIndex((s) => s.id === id)
      if (idx >= 0) list.value[idx] = detail.session
      currentSession.value = detail.session
      // 快照带运行态：切回一条还在跑的会话时立刻回到运行中（插话 / 排队模式），
      // 不能等下一帧事件才「发现」它在跑——它可能已经跑完了整轮。
      if (detail.session.running) useChatStore().resumeRunning()
    } finally {
      if (openingId.value === id) openingId.value = null
    }
  }

  async function refresh() {
    if (currentId.value) await open(currentId.value)
  }

  /** 发送成功后绑定到后端真正建出来的那条会话。
   * 先设 currentId（SSE 订阅跟着它切，切得越早丢帧越少），再补列表。 */
  async function adopt(id: string) {
    const prev = currentId.value
    adopting.value = true
    currentId.value = id
    currentSession.value = null
    try {
      await loadList()
      const found = list.value.find((s) => s.id === id)
      if (found) currentSession.value = found
    } catch {
      // 列表没刷出来不影响这一轮：会话编号已经切过去了
      currentId.value = id || prev
    } finally {
      adopting.value = false
    }
  }

  function echoUserMessage(content: string) {
    messages.value = [
      ...messages.value,
      { id: `local-${Date.now()}`, role: 'user', type: 'text', content, created_at: Date.now() },
    ]
  }

  async function remove(id: string) {
    await api.sessions.remove(id)
    await loadList()
    if (currentId.value === id) {
      const next = list.value[0]
      if (next) await open(next.id)
      else draft()
    }
  }

  async function rename(id: string, title: string) {
    await api.sessions.rename(id, title)
    await loadList()
    if (currentId.value === id) await open(id)
  }

  async function setModel(provider: string, model: string) {
    if (!currentId.value) return
    await api.sessions.setModel(currentId.value, provider, model)
    await loadList()
  }

  async function setThinking(level: string) {
    if (!currentId.value) return
    await api.sessions.setThinking(currentId.value, level)
    await loadList()
  }

  function setWorkspaceFilter(workspace: string) {
    workspaceFilter.value = workspace
    void loadList()
  }

  return {
    list,
    currentId,
    current,
    currentSession,
    messages,
    openingId,
    adopting,
    workspaceFilter,
    loadList,
    draft,
    open,
    refresh,
    adopt,
    remove,
    rename,
    setModel,
    setThinking,
    setWorkspaceFilter,
    echoUserMessage,
  }
})
