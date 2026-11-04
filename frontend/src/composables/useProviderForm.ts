// 服务商表单：一个域一份草稿 + 一个提交动作。
// 本壳只有一个引擎，所以不存在「这一行属于哪个引擎」这个维度——
// 服务商只落在一个 models.json 里。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { ModelEntry, ProviderView } from '@/types/api'

/** 新建模型时的默认上下文窗口。 */
export const DEFAULT_CONTEXT_WINDOW = 200000

/** 新建模型时的默认单次输出上限。 */
export const DEFAULT_MAX_TOKENS = 8192

/** 表单草稿。 */
export interface ProviderForm {
  provider_name: string
  base_url: string
  api_protocol: string
  api_key: string
  models: ModelEntry[]
}

/** 服务商表单的状态与动作。 */
export function useProviderForm(reload: () => Promise<unknown>) {
  const { t } = useI18n()
  const toasts = useToasts()

  const dialogOpen = ref(false)
  const saving = ref(false)
  // 正在编辑的服务商名（空表示新建）
  const editing = ref('')
  // 弹窗里的「拉模型」按钮单独一个标志：它与卡片上的探测不是一回事。
  const fetching = ref(false)

  const form = ref<ProviderForm>({
    provider_name: '',
    base_url: '',
    api_protocol: 'openai',
    api_key: '',
    models: [],
  })
  // 最近一次拉回来的上游清单，弹窗把它渲染成可点选的 chip。
  const fetched = ref<ModelEntry[]>([])

  /** 编辑态：名字是配置里的键，改它等于新建一条。 */
  const nameLocked = computed(() => Boolean(editing.value))

  function reset(): void {
    editing.value = ''
    form.value = { provider_name: '', base_url: '', api_protocol: 'openai', api_key: '', models: [] }
    fetched.value = []
  }

  function openCreate(): void {
    reset()
    dialogOpen.value = true
  }

  function openEdit(view: ProviderView): void {
    editing.value = view.provider_name
    form.value = {
      provider_name: view.provider_name,
      base_url: view.base_url,
      api_protocol: view.api_protocol || 'openai',
      // 打码后的密钥不回填：用户没改就提交空串，后端会保留原值
      api_key: '',
      models: (view.models ?? []).map((m) => ({ ...m })),
    }
    fetched.value = []
    dialogOpen.value = true
  }

  /**
   * 向上游问模型清单，结果交给弹窗渲染成 chip，由用户点选要哪些。
   *
   * 为什么不直接填进表里：一次拉回几十个 id 时用户通常只要其中几个，
   * 全填进去等于把「问出来的」当成「要保存的」，删起来比加起来还慢。
   */
  async function fetchModels(): Promise<void> {
    if (fetching.value) return
    fetching.value = true
    try {
      const res = await api.probeProvider({
        provider_name: editing.value || undefined,
        base_url: form.value.base_url.trim(),
        api_protocol: form.value.api_protocol,
        api_key: form.value.api_key.trim(),
      })
      const rows = res.models ?? []
      if (!rows.length) {
        fetched.value = []
        toasts.push('info', t('models.fetchedEmpty'))
        return
      }
      fetched.value = rows
      toasts.push('ok', t('models.fetched', { n: rows.length }))
    } catch (err) {
      toasts.push('error', err instanceof Error ? err.message : String(err))
    } finally {
      fetching.value = false
    }
  }

  async function save(): Promise<void> {
    const name = form.value.provider_name.trim()
    if (!name) return
    saving.value = true
    try {
      await api.upsertProvider({
        provider_name: name,
        base_url: form.value.base_url.trim(),
        api_protocol: form.value.api_protocol,
        auth_kind: 'api_key',
        api_key: form.value.api_key.trim(),
        models: form.value.models.filter((m) => m.model_id.trim()),
      })
      dialogOpen.value = false
      toasts.push('ok', t('common.saved'))
      await reload()
    } catch (err) {
      toasts.push('error', err instanceof Error ? err.message : String(err))
    } finally {
      saving.value = false
    }
  }

  return {
    dialogOpen,
    saving,
    editing,
    fetching,
    form,
    fetched,
    nameLocked,
    reset,
    openCreate,
    openEdit,
    fetchModels,
    save,
  }
}
