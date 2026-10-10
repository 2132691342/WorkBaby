import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import type { ApprovalVO, MessageVO, SessionVO } from '../types/api'

export const useSessionStore = defineStore('session', () => {
  const list = ref<SessionVO[]>([])
  const currentId = ref<string | null>(null)
  const messages = ref<MessageVO[]>([])
  // 一页会话数必须与后端 sessionPageSize 一致；还有更多时侧栏给「加载更多」。
  const PAGE = 200
  const hasMore = ref(false)

  const current = computed(() => list.value.find((s) => s.id === currentId.value) || null)

  async function loadList() {
    list.value = await api.sessions.list(0)
    hasMore.value = list.value.length >= PAGE
    return list.value
  }

  // loadMore 追加下一页：用 set 去重——翻页期间有会话更新会跳到前面，
  // 不去重会在列表里出现两条一样的对话。
  async function loadMore() {
    const more = await api.sessions.list(list.value.length)
    const seen = new Set(list.value.map((s) => s.id))
    const fresh = more.filter((s) => !seen.has(s.id))
    list.value = [...list.value, ...fresh]
    hasMore.value = more.length >= PAGE && fresh.length > 0
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

  // openingId 是「正在打开哪个会话」：侧栏行要给 loading，点了没反馈
  // 会让用户在慢机器上连点，来回切换把状态搅乱。
  const openingId = ref<string | null>(null)

  // stale 记录「上次退出时没来得及处理的确认」→ 会话 id 到条目数。
  // 这些确认在重启时被按拒绝收口，界面必须有一个地方说出这件事。
  const stale = ref<Record<string, string[]>>({})

  function setStaleApprovals(list: ApprovalVO[]) {
    const next: Record<string, string[]> = {}
    for (const a of list) {
      if (!a.session_id) continue
      ;(next[a.session_id] ||= []).push(a.label || a.tool || '一步操作')
    }
    stale.value = next
  }

  // staleText 给当前会话拼一句人话；看过后就清掉，别每次刷新都再念一遍。
  function takeStaleText(id: string): string {
    const labels = stale.value[id]
    if (!labels?.length) return ''
    const rest = { ...stale.value }
    delete rest[id]
    stale.value = rest
    const names = [...new Set(labels)].slice(0, 3).join('、')
    return labels.length === 1
      ? `上次退出时有一条确认没来得及处理（${names}），已按拒绝结束。需要的话让助手再做一次。`
      : `上次退出时有 ${labels.length} 条确认没来得及处理（${names} 等），已按拒绝结束。需要的话让助手再做一次。`
  }

  async function open(id: string) {
    openingId.value = id
    try {
      // 换会话先清空：不清的话这一瞬间标题已经是新的、消息还是上一段，
      // 观感是「两个会话叠在一起」。同一个会话刷新不清（失败时保住现有内容）。
      if (currentId.value !== id) messages.value = []
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
    } finally {
      if (openingId.value === id) openingId.value = null
    }
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
    hasMore,
    currentId,
    current,
    messages,
    openingId,
    stale,
    setStaleApprovals,
    takeStaleText,
    loadList,
    loadMore,
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
