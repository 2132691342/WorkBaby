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
    await loadList()
    await open(sess.id)
    return sess
  }

  async function open(id: string) {
    currentId.value = id
    const detail = await api.sessions.detail(id)
    messages.value = detail.messages
    const idx = list.value.findIndex((s) => s.id === id)
    if (idx >= 0) list.value[idx] = detail.session
  }

  async function refresh() {
    if (currentId.value) await open(currentId.value)
  }

  async function remove(id: string) {
    await api.sessions.remove(id)
    if (currentId.value === id) {
      currentId.value = null
      messages.value = []
    }
    await loadList()
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

  return { list, currentId, current, messages, loadList, create, open, refresh, remove, rename, setPermission, setModel }
})
