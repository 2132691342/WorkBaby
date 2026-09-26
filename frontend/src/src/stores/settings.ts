import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiGet, apiPost } from '@/api/client'
import { UploadFile, OpenFileDialog } from '@/wailsjs/go/main/App'
import { normalizeThemeID, useTheme } from '@/composables/useTheme'
import { useToast } from '@/composables/useToast'
import { t } from '@/i18n'
import type { AiProvider, AiProviderKind, AiProviderReq, CircuitState, ProviderPreset, WebSearchConfig } from '@/types/api'

/** 可选的代码字体（精简后只保留 3 个内置字体）。 */
export const CODE_FONTS = [
  'JetBrains Mono',
  'Fira Code',
  'Cascadia Code'
] as const

/** 把选中字体写到 --font-mono CSS 变量。 */
function applyCodeFontToDom(font: string): void {
  document.documentElement.style.setProperty('--font-mono', `'${font}', ui-monospace, monospace`)
}

/**
 * 设置 store（适配后端 ai_provider 单表改造 + snake_case 契约）。
 */
export const useSettingsStore = defineStore('settings', () => {
  const toast = useToast()
  const providers = ref<AiProvider[]>([])
  /** 后端实现的 ProviderKind 列表 + 展示元数据（前端从 /api/v1/ai-provider/kinds 拉，避免错配）。 */
  const providerKinds = ref<AiProviderKind[]>([])
  /** 合法档位取值（primary / backup），从 /api/v1/ai-provider/tiers 拉。 */
  const providerTiers = ref<string[]>(['primary', 'backup'])
  /** 常见模型服务预设（/ai-provider/presets）：新增 provider 一键预填。 */
  const providerPresets = ref<ProviderPreset[]>([])
  /** 所有 provider 的熔断状态 Map（id → CircuitState）。 */
  const circuitStates = ref<Map<string, CircuitState>>(new Map())
  const general = ref<Record<string, unknown>>({})
  /** 联网搜索配置（DuckDuckGo，无需 api_key；仅 enabled 生效）。 */
  const webSearchConfig = ref<WebSearchConfig>({
    enabled: 'true',
    engine: 'duckduckgo',
    api_key: ''
  })

  /** 全局记忆开关（KV memory.enabled；缺省开启）。关闭后：不再自动召回长期记忆、不再沉淀情景记忆。 */
  const memoryEnabled = ref(true)

  /** 读取全局记忆开关（KV 未配置 = 开启）。 */
  async function loadMemoryEnabled(): Promise<void> {
    try {
      const r = await apiGet<{ v?: string } | null>('/api/v1/kv/memory.enabled')
      memoryEnabled.value = r?.v == null ? true : r.v !== 'false'
    } catch {
      memoryEnabled.value = true
    }
  }

  /** 保存全局记忆开关。 */
  async function setMemoryEnabled(v: boolean): Promise<boolean> {
    error.value = null
    try {
      await apiPost('/api/v1/kv/memory.enabled', { value: String(v) })
      memoryEnabled.value = v
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  /** 聊天默认参数（system_settings KV；未配置 = 后端内置兜底，显示为 null 即「默认」）。
   *  default_temperature：全局默认采样温度（Provider 级 > 全局）；
   *  default_thinking：全局默认思考强度 off/low/medium/high；
   *  compression_ratio：上下文占用达窗口该比例触发摘要压缩（Provider 级 > 全局）；
   *  max_input_chars：单条用户输入字符上限。 */
  interface ChatDefaults {
    default_temperature: number | null
    default_thinking: string | null
    compression_ratio: number | null
    max_input_chars: number | null
    /** 单次 run 的 token 花费上限（0/空 = 不限；与 Agent 内置预算取更严者）。 */
    max_run_tokens: number | null
  }
  const chatDefaults = ref<ChatDefaults>({
    default_temperature: null,
    default_thinking: null,
    compression_ratio: null,
    max_input_chars: null,
    max_run_tokens: null
  })

  /** 读取聊天默认参数（逐项容错：单 key 失败不影响其余）。 */
  async function loadChatDefaults(): Promise<void> {
    const read = async (key: string): Promise<string | null> => {
      try {
        const r = await apiGet<{ v?: string } | null>(`/api/v1/kv/${key}`)
        return r?.v ?? null
      } catch {
        return null
      }
    }
    const [temp, think, ratio, maxChars, maxRun] = await Promise.all([
      read('chat.defaultTemperature'),
      read('chat.defaultThinking'),
      read('chat.compressionRatio'),
      read('chat.maxInputChars'),
      read('chat.maxRunTokens')
    ])
    const num = (v: string | null): number | null => {
      if (v == null || v === '') return null
      const n = Number(v)
      return Number.isFinite(n) ? n : null
    }
    chatDefaults.value = {
      default_temperature: num(temp),
      default_thinking: think || null,
      compression_ratio: num(ratio),
      max_input_chars: num(maxChars),
      max_run_tokens: num(maxRun)
    }
  }

  /** 保存聊天默认参数（逐项 upsert；空串即「清除自定义值，回落内置默认」）。 */
  async function saveChatDefaults(): Promise<boolean> {
    error.value = null
    const items: Array<[string, string]> = [
      ['chat.defaultTemperature', chatDefaults.value.default_temperature?.toString() ?? ''],
      ['chat.defaultThinking', chatDefaults.value.default_thinking ?? ''],
      ['chat.compressionRatio', chatDefaults.value.compression_ratio?.toString() ?? ''],
      ['chat.maxInputChars', chatDefaults.value.max_input_chars?.toString() ?? ''],
      ['chat.maxRunTokens', chatDefaults.value.max_run_tokens?.toString() ?? '']
    ]
    try {
      for (const [key, value] of items) {
        await apiPost(`/api/v1/kv/${key}`, { value })
      }
      toast.success(t('common.saveSuccess'))
      return true
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
      return false
    }
  }
  /** 关闭主窗口是否隐藏到托盘（tray.close_to_tray）。 */
  const closeToTray = ref(false)
  /** 全局界面缩放（0.85~1.25；1 = 默认，经 CSS zoom 全局生效）。 */
  const fontScale = ref(1)
  const backgroundUrl = ref<string | null>(null)
  const codeFont = ref<string>('JetBrains Mono')
  const theme = ref<string>('light')
  const error = ref<string | null>(null)
  const loading = ref(false)

  const form = ref<AiProviderReq>({
    name: '',
    kind: 'openai',
    api_key: '',
    base_url: '',
    model: '',
    alias: '',
    tier: 'primary',
    enabled: true,
    context_window: null,
    max_output_tokens: null,
    compress_ratio: 0.9,
    capabilities_json: null,
    pricing_json: null,
    temperature: null,
    top_p: null,
    thinking_effort: null,
    thinking_style: null,
    supports_tool_call: null,
    supports_vision: null,
    supports_reasoning: null
  })

  /** form 的出厂值（addProvider 成功后重置用，避免两处维护字段清单）。 */
  const emptyProviderForm = (): AiProviderReq => ({ ...form.value, name: '', api_key: '', base_url: '', model: '', alias: '' })

  /** 同时加载模型配置列表、通用设置和 SMTP 配置。 */
  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const [p, g, w, c, k, trs, presets] = await Promise.all([
        apiGet<AiProvider[]>('/api/v1/ai-provider'),
        // 注意：必须取 /settings/general（返回 theme/appearance/fontScale 的 JSON map）；
        // /settings 返回的是全部 KV 数组，不是前端要读的外观对象 —— 取错会导致缩放/主题永远无法恢复。
        apiGet<Record<string, unknown>>('/api/v1/settings/general'),
        apiGet<WebSearchConfig>('/api/v1/settings/websearch'),
        apiGet<CircuitState[]>('/api/v1/ai-provider/circuit-status').catch(() => [] as CircuitState[]),
        apiGet<AiProviderKind[]>('/api/v1/ai-provider/kinds').catch(() => [] as AiProviderKind[]),
        apiGet<string[]>('/api/v1/ai-provider/tiers').catch(() => [] as string[]),
        apiGet<ProviderPreset[]>('/api/v1/ai-provider/presets').catch(() => [] as ProviderPreset[])
      ])
      providers.value = p
      general.value = g
      webSearchConfig.value = w
      providerKinds.value = Array.isArray(k) ? k : []
      providerTiers.value = Array.isArray(trs) && trs.length > 0 ? trs : ['primary', 'backup']
      providerPresets.value = Array.isArray(presets) ? presets : []
      applyTheme(String(g.theme ?? theme.value))
      // 设置页自身也要回填外观（缩放/代码字体），否则这里显示默认 100%，
      // 与后端已保存的实际值不一致。
      syncAppearanceFromGeneral(g)
      const map = new Map<string, CircuitState>()
      for (const s2 of c) map.set(s2.id, s2)
      circuitStates.value = map
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** 新增模型配置（沿用组件表单的载荷）。 */
  async function addProvider(): Promise<boolean> {
    return addProviderWith(form.value)
  }

  /** 新增模型配置（指定载荷；新增/编辑统一弹窗共用，校验与提示同 addProvider）。 */
  async function addProviderWith(data: AiProviderReq): Promise<boolean> {
    error.value = null
    if (!data.name?.trim() || !data.model?.trim()) {
      error.value = t('settings.errNameModel')
      toast.warning(t('settings.errNameModel'))
      return false
    }
    // 额外提醒：本地协议（OpenAI 兼容 / Anthropic / Gemini / Ollama）通常需要 base_url 与 api_key，
    // 没填的话能创建成功但实际无法联通；不阻断但 toast.info 提醒，避免「点了没反应」。
    if (!data.base_url?.trim()) {
      toast.info(t('settings.baseUrlMissingHint'))
    }
    if (!data.api_key?.trim() && data.kind !== 'ollama') {
      toast.info(t('settings.apiKeyMissingHint'))
    }
    try {
      await apiPost<AiProvider>('/api/v1/ai-provider', data)
      form.value = emptyProviderForm()
      await load()
      toast.success(t('settings.providerAdded'))
      return true
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
      return false
    }
  }

  /** 更新模型配置。 */
  async function updateProvider(id: string, data: AiProviderReq): Promise<boolean> {
    error.value = null
    try {
      await apiPost<AiProvider>(`/api/v1/ai-provider/${id}/update`, data)
      await load()
      toast.success(t('settings.providerUpdated'))
      return true
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      error.value = msg
      toast.error(t('common.saveFailed'), msg)
      return false
    }
  }

  /** 删除模型配置。 */
  async function removeProvider(id: string): Promise<void> {
    try {
      await apiPost(`/api/v1/ai-provider/${id}/delete`)
      await load()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    }
  }

  /** 连通测试。 */
  async function testProvider(id: string): Promise<{ ok: boolean; error?: string }> {
    try {
      return await apiPost<{ ok: boolean; error?: string }>(`/api/v1/ai-provider/${id}/test`)
    } catch (e) {
      return { ok: false, error: e instanceof Error ? e.message : String(e) }
    }
  }

  /** 清空指定 provider 的熔断。 */
  async function resetCircuit(id: string): Promise<boolean> {
    try {
      const r = await apiPost<{ reset: boolean }>(`/api/v1/ai-provider/${id}/reset-circuit`)
      if (r.reset) {
        const next = new Map(circuitStates.value)
        next.delete(id)
        circuitStates.value = next
      }
      return r.reset
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  /** 保存联网搜索配置。 */
  async function saveWebSearchConfig(): Promise<boolean> {
    error.value = null
    try {
      await apiPost('/api/v1/settings/websearch', webSearchConfig.value)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  /** 保存通用设置。 */
  async function saveGeneral(): Promise<boolean> {
    error.value = null
    try {
      general.value = await apiPost<Record<string, unknown>>('/api/v1/settings/general', general.value)
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  /** 读取 appearance（背景 + 代码字体）并应用到 DOM。 */
  async function loadBackground(): Promise<void> {
    try {
      const g = await apiGet<Record<string, any>>('/api/v1/settings/general')
      general.value = g
      const bgID = g.appearance?.backgroundImageID as string | undefined
      backgroundUrl.value = bgID
        ? (await apiGet<{ url: string }>(`/api/v1/files/${bgID}/preview-url`)).url
        : null
      const font = (g.appearance?.fontMono as string) || 'JetBrains Mono'
      codeFont.value = font
      applyCodeFontToDom(font)
      // 后端未存过主题时回退 light（浅蓝工作台为默认外观）
      applyTheme((g.theme as string) || 'light')
    } catch {
      backgroundUrl.value = null
    }
  }

  /** 切换代码字体。 */
  async function setCodeFont(font: string): Promise<void> {
    codeFont.value = font
    applyCodeFontToDom(font)
    const g = (general.value as Record<string, any>) ?? {}
    const appearance = (g.appearance as Record<string, any>) ?? {}
    appearance.fontMono = font
    g.appearance = appearance
    general.value = await apiPost<Record<string, unknown>>('/api/v1/settings/general', g)
  }

  /** 上传背景图。 */
  async function setBackground(file: File | null): Promise<void> {
    const g = (general.value as Record<string, any>) ?? {}
    const appearance = (g.appearance as Record<string, any>) ?? {}
    let bgID = ''
    if (file) {
      const selected = await OpenFileDialog(t('settings.appearance.backgroundUpload'), '*.png;*.jpg;*.jpeg;*.gif;*.webp')
      if (!selected) return
      const info = await UploadFile('', selected, '', '')
      bgID = info.id
    }
    appearance.backgroundImageID = bgID
    g.appearance = appearance
    general.value = await apiPost<Record<string, unknown>>('/api/v1/settings/general', g)
    backgroundUrl.value = bgID
      ? (await apiGet<{ url: string }>(`/api/v1/files/${bgID}/preview-url`)).url
      : null
  }

  /** 应用主题到 DOM。 */
  function applyTheme(t: string): void {
    const themeID = normalizeThemeID(t)
    theme.value = themeID
    useTheme().setTheme(themeID)
  }

  /** 把界面缩放应用到 DOM（WebView2 = Chromium，CSS zoom 全量生效）。 */
  function applyFontScaleToDom(scale: number): void {
    document.documentElement.style.zoom = String(scale)
  }

  /** 从后端 general.appearance 回填缩放/代码字体并应用到 DOM（load() / loadBackground() 共用）。 */
  function syncAppearanceFromGeneral(g: Record<string, any>): void {
    if (!g || typeof g !== 'object') return
    const appearance = (g.appearance ?? {}) as Record<string, unknown>
    const scale = Number(appearance.fontScale)
    if (Number.isFinite(scale) && scale >= 0.8 && scale <= 1.3) {
      fontScale.value = scale
      applyFontScaleToDom(scale)
    }
    const font = appearance.fontMono as string | undefined
    if (font) {
      codeFont.value = font
      applyCodeFontToDom(font)
    }
  }

  /** 读取并应用托盘/缩放设置（App 启动调用一次）。 */
  async function loadBehavior(): Promise<void> {
    try {
      const [trayRaw, g] = await Promise.all([
        apiGet<{ value?: string } | null>('/api/v1/kv/tray.close_to_tray').catch(() => null),
        apiGet<Record<string, any>>('/api/v1/settings/general')
      ])
      closeToTray.value = String(trayRaw?.value ?? '').toLowerCase() === 'true'
      const scale = Number((g.appearance as Record<string, unknown> | undefined)?.fontScale)
      if (Number.isFinite(scale) && scale >= 0.8 && scale <= 1.3) {
        fontScale.value = scale
        applyFontScaleToDom(scale)
      }
    } catch {
      // 静默：默认退出行为 + 默认缩放
    }
  }

  /** 切换关闭到托盘行为。 */
  async function setCloseToTray(v: boolean): Promise<boolean> {
    error.value = null
    try {
      await apiPost('/api/v1/kv/tray.close_to_tray', { value: String(v) })
      closeToTray.value = v
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    }
  }

  /** 切换全局界面缩放并持久化。 */
  async function setFontScale(scale: number): Promise<void> {
    fontScale.value = scale
    applyFontScaleToDom(scale)
    const g = (general.value as Record<string, any>) ?? {}
    const appearance = (g.appearance as Record<string, any>) ?? {}
    appearance.fontScale = scale
    g.appearance = appearance
    general.value = await apiPost<Record<string, unknown>>('/api/v1/settings/general', g)
  }

  /** 切换主题并持久化。 */
  async function setTheme(t: string): Promise<void> {
    applyTheme(t)
    const g = (general.value as Record<string, any>) ?? {}
    g.theme = t
    general.value = await apiPost<Record<string, unknown>>('/api/v1/settings/general', g)
  }

  return {
    providers,
    providerKinds,
    providerTiers,
    providerPresets,
    general,
    webSearchConfig,
    chatDefaults,
    memoryEnabled,
    backgroundUrl,
    codeFont,
    theme,
    closeToTray,
    fontScale,
    circuitStates,
    error,
    loading,
    form,
    load,
    addProvider,
    addProviderWith,
    updateProvider,
    removeProvider,
    testProvider,
    resetCircuit,
    saveGeneral,
    saveWebSearchConfig,
    loadChatDefaults,
    saveChatDefaults,
    loadMemoryEnabled,
    setMemoryEnabled,
    loadBackground,
    setBackground,
    setCodeFont,
    setTheme,
    loadBehavior,
    setCloseToTray,
    setFontScale
  }
})
