import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { AgentProfile, AgentProfileReq } from '@/types/api'

/**
 * 自定义子智能体 store（snake_case 契约）。
 *
 * <p>后端以 name 为定位键（POST /agent-profiles 按 name upsert；/agent-profiles/:name/enabled|delete）。
 * 写操作成功后 harness 注册表即时同步，delegate_task 下一次委派即可按名命中。
 */
export const useAgentProfilesStore = defineStore('agentProfiles', () => {
  const toast = useToast()
  const dialog = useDialog()
  const profiles = ref<AgentProfile[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** 正在编辑的条目 name（null = 新建模式）。 */
  const editingName = ref<string | null>(null)

  const form = ref<AgentProfileReq>(blank())

  function blank(): AgentProfileReq {
    return {
      name: '',
      description: '',
      system_prompt: '',
      tools_allow: [],
      tools_deny: [],
      memory_enable: false,
      max_turns: 0,
      model: '',
      thinking: '',
      enabled: true
    }
  }

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      profiles.value = await apiGet<AgentProfile[]>('/api/v1/agent-profiles')
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  function startCreate(): void {
    editingName.value = null
    form.value = blank()
    error.value = null
  }

  function startEdit(p: AgentProfile): void {
    editingName.value = p.name
    form.value = {
      name: p.name,
      description: p.description ?? '',
      system_prompt: p.system_prompt ?? '',
      tools_allow: p.tools_allow ?? [],
      tools_deny: p.tools_deny ?? [],
      memory_enable: p.memory_enable,
      max_turns: p.max_turns,
      model: p.model ?? '',
      thinking: p.thinking ?? '',
      enabled: p.enabled
    }
    error.value = null
  }

  function cancel(): void {
    editingName.value = null
    form.value = blank()
  }

  async function submit(): Promise<AgentProfile | null> {
    error.value = null
    if (!form.value.name.trim()) {
      error.value = t('agents.errRequired')
      toast.warning(t('agents.errRequired'))
      return null
    }
    try {
      const saved = await apiPost<AgentProfile>('/api/v1/agent-profiles', form.value)
      cancel()
      await load()
      return saved
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
      return null
    }
  }

  async function toggleEnabled(p: AgentProfile, enabled: boolean): Promise<void> {
    try {
      await apiPost(`/api/v1/agent-profiles/${encodeURIComponent(p.name)}/enabled`, { enabled })
      await load()
    } catch (e) {
      toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
    }
  }

  async function remove(p: AgentProfile): Promise<void> {
    const ok = await dialog.confirm({ title: t('common.confirmDeleteTitle'), content: t('agents.deleteConfirm', p.name), danger: true })
    if (!ok) return
    try {
      await apiPost(`/api/v1/agent-profiles/${encodeURIComponent(p.name)}/delete`)
      await load()
    } catch (e) {
      toast.error(t('chat.operationFailed'), e instanceof Error ? e.message : String(e))
    }
  }

  return { profiles, loading, error, editingName, form, load, startCreate, startEdit, cancel, submit, toggleEnabled, remove }
})
