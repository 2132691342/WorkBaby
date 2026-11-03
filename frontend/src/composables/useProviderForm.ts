// 服务商的编辑草稿：表单内容与「打开 / 保存 / 拉模型」三个动作，
// 以及最近一次拉回的上游清单（弹窗渲染成可点选的 chip）。
// 草稿与清单必须两份数据（正在编辑的 / 已保存的）——混成一份就取消不了编辑。
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/http'
import { useToasts } from '@/composables/useToasts'
import type { EngineKind, ModelEntry, ProviderView } from '@/types/api'

/** 草稿形态：与 ProviderView 对齐，但密钥是可以改的空串而不是掩码。 */
export interface ProviderForm {
  name: string
  base_url: string
  api: string
  api_key: string
  models: ModelEntry[]
}

/**
 * 模型能力的底：留空不能等于 0。
 *
 * 0 会让水位环永远显示 0%，引擎也拿不到可用预算。
 * 这两个值与引擎侧的默认值一致。
 */
export const DEFAULT_CONTEXT_WINDOW = 128000
export const DEFAULT_MAX_TOKENS = 8192

/**
 * 服务商草稿的操作集。
 *
 * reload 是保存后的刷新回调，由调用方给——它还要同时刷新模型表与登录区，
 * 那些不在本文件里。
 */
export function useProviderForm(
  reload: () => Promise<void>,
  /** 当前作用域的引擎：新建时写入哪个引擎的配置 */
  defaultKind: () => EngineKind,
) {
  const { t } = useI18n()
  const toasts = useToasts()

  const dialogOpen = ref(false)
  const saving = ref(false)
  // 正在编辑的服务商名（空表示新建）
  const editing = ref('')
  // 正在编辑的那一行属于哪个引擎：两个引擎的配置完全独立，同名可以各有一份。
  const editingKind = ref<EngineKind>('omp')
  // 弹窗里的「拉模型」按钮单独一个标志：它与卡片上的探测不是一回事。
  const fetching = ref(false)

  const form = ref<ProviderForm>({
    name: '',
    base_url: '',
    api: 'openai-completions',
    api_key: '',
    models: [],
  })
  // 最近一次拉回来的上游清单，弹窗把它渲染成可点选的 chip。
  // 打开弹窗（新建或编辑）时清空：它是「这次编辑会话」的临时产物。
  const fetched = ref<ModelEntry[]>([])

  function reset(): void {
    editing.value = ''
    form.value = { name: '', base_url: '', api: 'openai-completions', api_key: '', models: [] }
    fetched.value = []
  }

  function openCreate(): void {
    reset()
    editingKind.value = defaultKind()
    dialogOpen.value = true
  }

  function openEdit(view: ProviderView): void {
    editing.value = view.provider_name
    // 记住这一行属于哪个引擎：编辑同名服务商时不能写到另一个引擎的文件里。
    editingKind.value = (view.engine_kind || defaultKind()) as EngineKind
    form.value = {
      name: view.provider_name,
      base_url: view.base_url,
      api: view.api || 'openai-completions',
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
      const res = await api.fetchProviderModels({
        engine_kind: editingKind.value,
        provider_name: editing.value || undefined,
        base_url: form.value.base_url.trim(),
        api: form.value.api,
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
    const name = form.value.name.trim()
    if (!name) return
    saving.value = true
    try {
      const payload: Partial<ProviderView> & { models?: ModelEntry[] } = {
        base_url: form.value.base_url.trim(),
        api: form.value.api,
        api_key: form.value.api_key.trim(),
        models: form.value.models.filter((m) => m.model_id.trim()),
      }
      // 写入必须落到一个具体引擎：编辑已有行时用它自己的 engine_kind，
      // 新建时用当前作用域（「全部」时退回 omp）。
      const target = editing.value ? editingKind.value : defaultKind()
      await api.upsertProvider(name, payload, target)
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
    fetching,
    editing,
    editingKind,
    form,
    fetched,
    openCreate,
    openEdit,
    fetchModels,
    save,
  }
}
