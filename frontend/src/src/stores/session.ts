import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import type { MessageVO, SessionVO } from '../types/api'

export const useSessionStore = defineStore('session', () => {
  const list = ref<SessionVO[]>([])
  const currentId = ref<string | null>(null)
  const messages = ref<MessageVO[]>([])

  const current = computed(() => list.value.find((s) => s.id === currentId.value) || null)

  async function loadList() {
    list.value = await api.sessions.list()
    return list.value
  }

  async function create(payload: { title?: string; workspace?: string } = {}) {
    const sess = await api.sessions.create(payload)
    // 返回体没有 id 时当场说清楚：继续往下走会读 null.id 崩在渲染层，
    // 用户只看到一条看不懂的「界面出现异常」。
    if (!sess?.id) throw new Error('新建对话没有成功，请重试')
    await loadList()
    await open(sess.id)
    return sess
  }

  async function open(id: string) {
    currentId.value = id
    const detail = await api.sessions.detail(id)
    // 快照防御：格式异常的响应绝不覆盖现有展示。
    // 注意不拦「空列表」——新建会话的快照本来就是空的，拦了会误伤新建/切换。
    if (!detail || !Array.isArray(detail.messages)) {
      throw new Error('会话快照格式异常，已保留当前内容')
    }
    messages.value = detail.messages
    const idx = list.value.findIndex((s) => s.id === id)
    if (idx >= 0) list.value[idx] = detail.session
  }

  async function refresh() {
    if (currentId.value) await open(currentId.value)
  }

  // echoUserMessage 在发送成功后立刻把这条用户消息插到列表末尾。
  // 用 entry_id 去重：随后的权威快照会把同一条再带回来，
  // 没有这道去重，屏幕上就会出现两条一模一样的消息。
  function echoUserMessage(entryId: string, content: string) {
    if (!entryId) return
    if (messages.value.some((m) => m.id === entryId)) return
    messages.value = [
      ...messages.value,
      {
        id: entryId,
        role: 'user',
        type: 'message',
        content,
        created_at: Date.now(),
      },
    ]
  }

  async function remove(id: string) {
    await api.sessions.remove(id)
    await loadList()
    if (currentId.value === id) {
      // 删的是当前会话：自动切到列表里的下一个，不留一个空白界面
      const next = list.value[0]
      if (next) {
        await open(next.id)
      } else {
        await create({})
      }
    }
  }

  async function rename(id: string, title: string) {
    await api.sessions.rename(id, title)
    await loadList()
  }

  async function setPermission(permission: string) {
    if (!currentId.value) return
    await api.sessions.setPermission(currentId.value, permission)
    await loadList()
  }

  async function setModel(providerId: string, model: string) {
    if (!currentId.value) return
    await api.sessions.setModel(currentId.value, providerId, model)
    await loadList()
  }

  return {
    list,
    currentId,
    current,
    messages,
    loadList,
    create,
    open,
    refresh,
    remove,
    rename,
    setPermission,
    setModel,
    echoUserMessage,
  }
})
